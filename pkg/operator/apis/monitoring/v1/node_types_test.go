// Copyright 2022 Google LLC
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

package v1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterNodeMonitoring_ScrapeConfig(t *testing.T) {
	// Generate YAML for one complex scrape config and make sure everything
	// adds up. This primarily verifies that everything is included and marshalling
	// the generated config to YAML does not produce any bad configurations due to
	// defaulting as the Prometheus structs are misconfigured in this regard in
	// several places.
	pmon := &ClusterNodeMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubelet",
		},
		Spec: ClusterNodeMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/os": "linux"},
			},
			Endpoints: []ScrapeNodeEndpoint{
				{
					Interval: "10s",
					Path:     "/cadvisor/metrics",
					Scheme:   "https",
					MetricRelabeling: []RelabelingRule{
						{
							Action:       "replace",
							SourceLabels: []string{"mlabel_1", "mlabel_2"},
							TargetLabel:  "mlabel_3",
						}, {
							Action:       "hashmod",
							SourceLabels: []string{"mlabel_1"},
							Modulus:      3,
							TargetLabel:  "__tmp_mod",
						}, {
							Action:  "keep",
							Regex:   "foo_.+",
							Modulus: 3,
						},
					},
				},
				{
					Scheme:   "https",
					Interval: "10000ms",
					Timeout:  "5s",
				},
			},
			Limits: &ScrapeLimits{
				Samples:          1,
				Labels:           2,
				LabelNameLength:  3,
				LabelValueLength: 4,
			},
		},
	}

	globalMetricRelabelCfg := []*relabel.Config{
		{
			Action:       relabel.Drop,
			SourceLabels: prommodel.LabelNames{"__name__"},
			Regex:        relabel.MustNewRegexp("my_expensive_metric1"),
		},
	}
	scrapeCfgs, err := pmon.ScrapeConfigs("test_project", "test_location", "test_cluster", globalMetricRelabelCfg)
	if err != nil {
		t.Fatal(err)
	}
	var got []string

	for _, sc := range scrapeCfgs {
		b, err := yaml.Marshal(sc)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(b))
	}
	want := []string{
		`job_name: ClusterNodeMonitoring/kubelet/cadvisor/metrics
honor_timestamps: false
track_timestamps_staleness: false
scrape_interval: 10s
scrape_timeout: 10s
metrics_path: /cadvisor/metrics
scheme: https
enable_compression: true
sample_limit: 1
label_limit: 2
label_name_length_limit: 3
label_value_length_limit: 4
authorization:
  credentials_file: /var/run/secrets/kubernetes.io/serviceaccount/token
tls_config:
  ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
  insecure_skip_verify: false
follow_redirects: false
enable_http2: false
relabel_configs:
- source_labels: [__meta_kubernetes_node_label_kubernetes_io_os]
  regex: linux
  action: keep
- target_label: job
  replacement: kubelet
  action: replace
- source_labels: [__meta_kubernetes_node_name]
  target_label: node
  action: replace
- source_labels: [__meta_kubernetes_node_name]
  target_label: instance
  replacement: $1:cadvisor/metrics
  action: replace
- target_label: project_id
  replacement: test_project
  action: replace
- target_label: location
  replacement: test_location
  action: replace
- target_label: cluster
  replacement: test_cluster
  action: replace
metric_relabel_configs:
- source_labels: [__name__]
  regex: my_expensive_metric1
  action: drop
- source_labels: [mlabel_1, mlabel_2]
  target_label: mlabel_3
  action: replace
- source_labels: [mlabel_1]
  modulus: 3
  target_label: __tmp_mod
  action: hashmod
- regex: foo_.+
  modulus: 3
  action: keep
kubernetes_sd_configs:
- role: node
  kubeconfig_file: ""
  follow_redirects: true
  enable_http2: true
  selectors:
  - role: node
    field: metadata.name=$(NODE_NAME)
`,
		`job_name: ClusterNodeMonitoring/kubelet/metrics
honor_timestamps: false
track_timestamps_staleness: false
scrape_interval: 10s
scrape_timeout: 5s
metrics_path: /metrics
scheme: https
enable_compression: true
sample_limit: 1
label_limit: 2
label_name_length_limit: 3
label_value_length_limit: 4
authorization:
  credentials_file: /var/run/secrets/kubernetes.io/serviceaccount/token
tls_config:
  ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
  insecure_skip_verify: false
follow_redirects: false
enable_http2: false
relabel_configs:
- source_labels: [__meta_kubernetes_node_label_kubernetes_io_os]
  regex: linux
  action: keep
- target_label: job
  replacement: kubelet
  action: replace
- source_labels: [__meta_kubernetes_node_name]
  target_label: node
  action: replace
- source_labels: [__meta_kubernetes_node_name]
  target_label: instance
  replacement: $1:metrics
  action: replace
- target_label: project_id
  replacement: test_project
  action: replace
- target_label: location
  replacement: test_location
  action: replace
- target_label: cluster
  replacement: test_cluster
  action: replace
metric_relabel_configs:
- source_labels: [__name__]
  regex: my_expensive_metric1
  action: drop
kubernetes_sd_configs:
- role: node
  kubeconfig_file: ""
  follow_redirects: true
  enable_http2: true
  selectors:
  - role: node
    field: metadata.name=$(NODE_NAME)
`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected scrape config YAML (-want, +got): %s", diff)
	}
}
