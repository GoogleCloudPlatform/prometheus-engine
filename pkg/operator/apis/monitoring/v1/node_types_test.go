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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateClusterNodeMonitoring(t *testing.T) {
	cases := []struct {
		desc        string
		pm          ClusterNodeMonitoringSpec
		eps         []ScrapeNodeEndpoint
		fail        bool
		errContains string
	}{
		{
			desc: "OK metadata labels",
			eps: []ScrapeNodeEndpoint{
				{
					Interval: "10s",
				},
			},
		},
		{
			desc: "Scrape interval missing",
			eps: []ScrapeNodeEndpoint{
				{},
			},
			fail:        true,
			errContains: "empty duration string",
		},
		{
			desc: "scrape interval malformed",
			eps: []ScrapeNodeEndpoint{
				{
					Interval: "foo",
				},
			},
			fail:        true,
			errContains: "invalid scrape interval: not a valid duration string",
		},
		{
			desc: "scrape timeout greater than interval",
			eps: []ScrapeNodeEndpoint{
				{
					Interval: "1s",
					Timeout:  "2s",
				},
			},
			fail:        true,
			errContains: "scrape timeout 2s must not be greater than scrape interval 1s",
		},
		{
			// Regression test for https://github.com/GoogleCloudPlatform/prometheus-engine/issues/479
			desc: "Duplicated job name",
			eps: []ScrapeNodeEndpoint{
				{
					Interval: "10s",
				},
				{
					Interval: "10000ms",
				},
			},
			fail:        true,
			errContains: "/r1/metrics for endpoints with index 0 and 1;consider creating a separate custom resource (PodMonitoring, etc.) for endpoints that share the same resource name, namespace and port name",
		},
	}

	for _, c := range cases {
		t.Run(c.desc+"", func(t *testing.T) {
			nm := &ClusterNodeMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name: "r1",
				},
				Spec: ClusterNodeMonitoringSpec{
					Endpoints: c.eps,
				},
			}
			_, err := nm.ValidateCreate()
			t.Log(err)

			if err == nil && c.fail {
				t.Fatalf("expected failure but passed")
			}
			if err != nil && !c.fail {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err != nil && c.fail && !strings.Contains(err.Error(), c.errContains) {
				t.Fatalf("expected error to contain %q but got %q", c.errContains, err)
			}
		})
	}
}

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
	scrapeCfgs, err := pmon.ScrapeConfigs("test_project", "test_location", "test_cluster")
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
scrape_interval: 10s
scrape_timeout: 10s
metrics_path: /cadvisor/metrics
scheme: https
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
scrape_interval: 10s
scrape_timeout: 5s
metrics_path: /metrics
scheme: https
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
