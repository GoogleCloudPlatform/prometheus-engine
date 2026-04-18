// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/go-kit/log"
	"github.com/thanos-io/thanos/pkg/reloader"
)

// gzipData compressed config, the same as operator
// https://github.com/GoogleCloudPlatform/prometheus-engine/blob/c2c7e38e14842aaec2110fa95b3042274ae9729e/pkg/operator/collection.go#L233
func gzipData(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

const avgPrometheusScrapeJob = `job_name: PodMonitoring/ns1/name1/web
honor_timestamps: false
track_timestamps_staleness: false
scrape_interval: 10s
scrape_timeout: 10s
metrics_path: /metrics
enable_compression: true
sample_limit: 1
label_limit: 2
label_name_length_limit: 3
label_value_length_limit: 4
follow_redirects: true
enable_http2: true
relabel_configs:
- target_label: project_id
  replacement: test_project
  action: replace
- target_label: location
  replacement: test_location
  action: replace
- target_label: cluster
  replacement: test_cluster
  action: replace
- source_labels: [__meta_kubernetes_namespace]
  regex: ns1
  action: keep
- source_labels: [__meta_kubernetes_namespace]
  target_label: namespace
  action: replace
- target_label: job
  replacement: name1
  action: replace
- source_labels: [__meta_kubernetes_pod_phase]
  regex: (Failed|Succeeded)
  action: drop
- source_labels: [__meta_kubernetes_pod_name]
  target_label: __tmp_instance
  action: replace
- source_labels: [__meta_kubernetes_pod_controller_kind, __meta_kubernetes_pod_node_name]
  regex: DaemonSet;(.*)
  target_label: __tmp_instance
  replacement: $1
  action: replace
- source_labels: [__meta_kubernetes_pod_container_port_name]
  regex: web
  action: keep
- source_labels: [__tmp_instance, __meta_kubernetes_pod_container_port_name]
  regex: (.+);(.+)
  target_label: instance
  replacement: $1:$2
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key1]
  target_label: key2
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key3]
  target_label: key3
  action: replace
metric_relabel_configs:
- source_labels: [mlabel_1, mlabel_2]
  target_label: mlabel_3
  action: replace
- source_labels: [mlabel_1]
  modulus: 3
  target_label: __tmp_mod
  action: hashmod
- source_labels: [mlabel_4]
  target_label: mlabel_5
- regex: foo_.+
  modulus: 3
  action: keep
kubernetes_sd_configs:
- role: pod
  kubeconfig_file: ""
  follow_redirects: true
  enable_http2: true
  selectors:
  - role: pod
    field: spec.nodeName=$(NODE_NAME)
`

// generateRandomString returns a pseudo-random string of length n.
// It uses the global math/rand source, which must be seeded externally.
func generateRandomString(r *rand.Rand, n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}

func generateCompressedTestPromConfig(t testing.TB, jobs int) []byte {
	t.Helper()

	r := rand.New(rand.NewSource(42)) // For deterministic input.

	ret := &promconfig.Config{
		GlobalConfig: promconfig.DefaultGlobalConfig,
	}
	ret.ScrapeConfigs = make([]*promconfig.ScrapeConfig, jobs)
	for i := range jobs {
		var avgScrapeConfig promconfig.ScrapeConfig
		require.NoError(t, yaml.Unmarshal([]byte(avgPrometheusScrapeJob), &avgScrapeConfig))

		ns := generateRandomString(r, 30)
		name := generateRandomString(r, 30)
		portName := generateRandomString(r, 30)

		// Change some obvious things to avoid trivial compression.
		avgScrapeConfig.JobName = fmt.Sprintf("PodMonitoring/%s/%s/%s", ns, name, portName)
		avgScrapeConfig.RelabelConfigs[3].Regex = relabel.MustNewRegexp(ns)
		avgScrapeConfig.RelabelConfigs[5].Regex = relabel.MustNewRegexp(name)
		avgScrapeConfig.RelabelConfigs[9].Regex = relabel.MustNewRegexp(portName)
		ret.ScrapeConfigs[i] = &avgScrapeConfig
	}

	out, err := yaml.Marshal(ret)
	require.NoError(t, err)
	return out
}

// Recommended CLI invocation:
/*
	export bench=reloader && go test ./cmd/config-reloader/... \
		-run '^$' -bench '^BenchmarkConfigReloader_ReloadingWithoutChange' \
		-benchtime 2s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkConfigReloader_ReloadingWithoutChange(b *testing.B) {
	// 8500 is roughly the maximum number of minimal scrape jobs (so scrape endpoints
	// in all PM, CPM and CNMs CRs) user could configure given the Kubernetes
	// configMap limit (1MB as per https://kubernetes.io/docs/concepts/configuration/configmap/#motivation)
	//
	// 8500 jobs means compressed ~0.96 MB config file, from the ~28.4 MB uncompressed.
	// This can obviously vary depends on compressibility and size of the extra relabelling.
	// Practically this number could be closer to ~7000.
	for _, jobs := range []int{10, 100, 1000, 8500} {
		b.Run(fmt.Sprintf("jobs=%v", jobs), func(b *testing.B) {
			dir := b.TempDir()
			input := generateCompressedTestPromConfig(b, jobs)
			compressedInput, err := gzipData(input)
			require.NoError(b, err)

			require.NoError(b, os.WriteFile(filepath.Join(dir, "config.yaml"), compressedInput, 0o644))

			r := reloader.New(
				log.NewNopLogger(),
				nil,
				&reloader.Options{
					CfgFile:       filepath.Join(dir, "config.yaml"),
					CfgOutputFile: filepath.Join(dir, "config-out.yaml"),

					WatchInterval: 0,               // Watch 0 turns makes reloader run on-demand.
					RetryInterval: 5 * time.Second, // Has to be positive due to Thanos bug.
				},
			)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				b.ReportMetric(float64(jobs), "scrape-jobs")
				b.ReportMetric(float64(len(compressedInput)), "compressed-config-size-bytes")
				b.ReportMetric(float64(len(input)), "config-size-bytes")

				// TODO: Verify full cycle has happened. This requires updating the config,
				// reloading mock etc. However the majority of the overhead happens before,
				// so low priority..
				require.NoError(b, r.Watch(b.Context()))
			}
		})
	}
}
