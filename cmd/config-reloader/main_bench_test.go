package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

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

	b, err := gzipData(out)
	require.NoError(t, err)

	// Expected numbers of bytes before compression.
	require.Len(t, out, 29809714, len(out))

	// Expected number of bytes post compression (~0.96 MB out of ~28.4 MB uncompressed).
	// This number can't be larger than Kubernetes configMap limit (1MB)
	// https://kubernetes.io/docs/concepts/configuration/configmap/#motivation
	require.Len(t, b, 984774, len(b))
	return b
}

// Recommended CLI invocation:
/*
	export bench=reloader && go test ./cmd/config-reloader/... \
		-run '^$' -bench '^BenchmarkConfigReloader_LargeCfgFile_Reloading' \
		-benchtime 2s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkConfigReloader_LargeCfgFile_ReloadingWithoutChange(b *testing.B) {
	dir := b.TempDir()
	input := generateCompressedTestPromConfig(b, 8500)

	require.NoError(b, os.WriteFile(filepath.Join(dir, "config.yaml"), input, 0o644))

	var reloads atomic.Int64
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		reloads.Add(1)
	}))
	b.Cleanup(rec.Close)

	recURL, err := url.Parse(rec.URL)
	require.NoError(b, err)

	r := reloader.New(
		log.NewNopLogger(),
		nil,
		&reloader.Options{
			ReloadURL:     recURL,
			CfgFile:       filepath.Join(dir, "config.yaml"),
			CfgOutputFile: filepath.Join(dir, "config-out.yaml"),

			WatchInterval: 0, // Watch 0 turns makes reloader run on-demand.
		},
	)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		// TODO: Verify full cycle has happened. This requires updating the config.
		// Majority of the overhead happens before
		require.NoError(b, r.Watch(b.Context()))

		// require.Equal(b, lastReloads+1, reloads.Load())
	}
}
