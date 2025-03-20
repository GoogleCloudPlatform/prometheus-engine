// Copyright 2024 Google LLC
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
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func applyDefaultsToRelabelConfig(rules []*relabel.Config) {
	for i := range rules {
		if rules[i].Action == relabel.Action("") {
			rules[i].Action = relabel.DefaultRelabelConfig.Action
		}
		if rules[i].Separator == "" {
			rules[i].Separator = relabel.DefaultRelabelConfig.Separator
		}
		emptyRegexp := relabel.Regexp{}
		if rules[i].Regex == emptyRegexp {
			rules[i].Regex = relabel.DefaultRelabelConfig.Regex
		}
		if rules[i].Replacement == "" {
			rules[i].Replacement = relabel.DefaultRelabelConfig.Replacement
		}
	}
}

func TestTopLevelControllerRelabel(t *testing.T) {
	rules := make([]*relabel.Config, 0, len(topLevelControllerNameRules)+len(topLevelControllerTypeRules))
	rules = append(rules, topLevelControllerNameRules...)
	rules = append(rules, topLevelControllerTypeRules...)
	applyDefaultsToRelabelConfig(rules)

	type test struct {
		input    labels.Labels
		want     labels.Labels
		wantKeep bool
	}
	tests := map[string]test{
		// Base cases
		"Empty": {
			input:    labels.Labels{},
			want:     labels.Labels{},
			wantKeep: true,
		},
		"Pod": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: ""},
				{Name: "__meta_kubernetes_pod_controller_name", Value: ""},
				{Name: "__meta_kubernetes_pod_name", Value: "pod_name"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_name", Value: "pod_name"},
			},
			wantKeep: true,
		},

		// Controller types
		"CronJob": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "Job"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-cronjob-12345678"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-cronjob-12345678-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "Job"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-cronjob-12345678"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-cronjob-12345678-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-cronjob"},
				{Name: labelTopLevelControllerType, Value: "CronJob"},
			},
			wantKeep: true,
		},
		"DaemonSet": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "DaemonSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-daemonset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-daemonset-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "DaemonSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-daemonset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-daemonset-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-daemonset"},
				{Name: labelTopLevelControllerType, Value: "DaemonSet"},
			},
			wantKeep: true,
		},
		"Deployment": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicaSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-deployment-1234567890"},
				{Name: "__meta_kubernetes_pod_labelpresent_pod_template_hash", Value: "true"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-deployment-012345789-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicaSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-deployment-1234567890"},
				{Name: "__meta_kubernetes_pod_labelpresent_pod_template_hash", Value: "true"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-deployment-012345789-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-deployment"},
				{Name: labelTopLevelControllerType, Value: "Deployment"},
			},
			wantKeep: true,
		},
		"Job": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "Job"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-job"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-job-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "Job"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-job"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-job-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-job"},
				{Name: labelTopLevelControllerType, Value: "Job"},
			},
			wantKeep: true,
		},
		"ReplicaSet": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicaSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-replicaset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-replicaset-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicaSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-replicaset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-replicaset-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-replicaset"},
				{Name: labelTopLevelControllerType, Value: "ReplicaSet"},
			},
			wantKeep: true,
		},
		"ReplicationController": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicationController"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-replicationcontroller"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-replicationcontroller-abcde"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "ReplicationController"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-replicationcontroller"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-replicationcontroller-abcde"},
				{Name: labelTopLevelControllerName, Value: "test-replicationcontroller"},
				{Name: labelTopLevelControllerType, Value: "ReplicationController"},
			},
			wantKeep: true,
		},
		"StatefulSet": {
			input: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "StatefulSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-statefulset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-statefulset-1234567890"},
			},
			want: labels.Labels{
				{Name: "__meta_kubernetes_pod_controller_kind", Value: "StatefulSet"},
				{Name: "__meta_kubernetes_pod_controller_name", Value: "test-statefulset"},
				{Name: "__meta_kubernetes_pod_name", Value: "test-statefulset-1234567890"},
				{Name: labelTopLevelControllerName, Value: "test-statefulset"},
				{Name: labelTopLevelControllerType, Value: "StatefulSet"},
			},
			wantKeep: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_ = tc
			ret, keep := relabel.Process(tc.input, rules...)
			if diff := cmp.Diff(tc.want, ret); diff != "" {
				t.Errorf("Relabeling does not produce expected result (-want, +got).\n%s\n", diff)
			}
			if tc.wantKeep != keep {
				t.Errorf("Mismatch on keep labels. Want: %t, Got: %t", tc.wantKeep, keep)
			}
		})
	}
}

func TestLabelMappingRelabelConfigs(t *testing.T) {
	cases := []struct {
		doc      string
		mappings []LabelMapping
		expected []*relabel.Config
		expErr   bool
	}{
		{
			doc:      "good podmonitoring relabel",
			mappings: []LabelMapping{{From: "from", To: "to"}},
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding podmonitoring relabel",
			mappings: []LabelMapping{{From: "from-instance", To: "instance"}},
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding podmonitoring relabel",
			mappings: []LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			expected: nil,
			expErr:   true,
		},
		{
			doc:      "empty to podmonitoring relabel",
			mappings: []LabelMapping{{From: "from"}},
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_from"},
				TargetLabel:  "from",
			}},
			expErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			// If we get an error when we don't expect, fail test.
			actual, err := labelMappingRelabelConfigs(c.mappings, "__meta_kubernetes_pod_label_")
			if err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", err)
			}
			if err == nil && c.expErr {
				t.Errorf("should have returned an error")
			}
			if diff := cmp.Diff(c.expected, actual, cmpopts.IgnoreUnexported(relabel.Regexp{}, regexp.Regexp{})); diff != "" {
				t.Errorf("returned unexpected config (-want, +got): %s", diff)
			}
		})
	}
}

func TestPodMonitoring_ScrapeConfig(t *testing.T) {
	// Generate YAML for one complex scrape config and make sure everything
	// adds up. This primarily verifies that everything is included and marshalling
	// the generated config to YAML does not produce any bad configurations due to
	// defaulting as the Prometheus structs are misconfigured in this regard in
	// several places.
	pmon := &PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "name1",
		},
		Spec: PodMonitoringSpec{
			Endpoints: []ScrapeEndpoint{
				{
					Port:     intstr.FromString("web"),
					Interval: "10s",
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
							SourceLabels: []string{"mlabel_4"},
							TargetLabel:  "mlabel_5",
						}, {
							Action:  "keep",
							Regex:   "foo_.+",
							Modulus: 3,
						},
					},
				},
				{
					Port:     intstr.FromInt(8080),
					Interval: "10000ms",
					Timeout:  "5s",
					Path:     "/prometheus",
					HTTPClientConfig: HTTPClientConfig{
						ProxyConfig: ProxyConfig{
							ProxyURL: "http://foo.bar/test",
						},
					},
				},
			},
			TargetLabels: TargetLabels{
				FromPod: []LabelMapping{
					{From: "key1", To: "key2"},
					{From: "key3"},
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
	scrapeCfgs, err := pmon.ScrapeConfigs("test_project", "test_location", "test_cluster", nil)
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
		`job_name: PodMonitoring/ns1/name1/web
honor_timestamps: false
scrape_interval: 10s
scrape_timeout: 10s
metrics_path: /metrics
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
`,
		`job_name: PodMonitoring/ns1/name1/8080
honor_timestamps: false
scrape_interval: 10s
scrape_timeout: 5s
metrics_path: /prometheus
sample_limit: 1
label_limit: 2
label_name_length_limit: 3
label_value_length_limit: 4
follow_redirects: true
enable_http2: true
proxy_url: http://foo.bar/test
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
- regex: container
  action: labeldrop
- source_labels: [__tmp_instance]
  target_label: instance
  replacement: $1:8080
  action: replace
- source_labels: [__meta_kubernetes_pod_ip]
  target_label: __address__
  replacement: $1:8080
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key1]
  target_label: key2
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key3]
  target_label: key3
  action: replace
kubernetes_sd_configs:
- role: pod
  kubeconfig_file: ""
  follow_redirects: true
  enable_http2: true
  selectors:
  - role: pod
    field: spec.nodeName=$(NODE_NAME)
`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected scrape config YAML (-want, +got): %s", diff)
	}
}

func TestClusterPodMonitoring_ScrapeConfig(t *testing.T) {
	// Generate YAML for one complex scrape config and make sure everything
	// adds up. This primarily verifies that everything is included and marshalling
	// the generated config to YAML does not produce any bad configurations due to
	// defaulting as the Prometheus structs are misconfigured in this regard in
	// several places.
	pmon := &ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: "name1",
		},
		Spec: ClusterPodMonitoringSpec{
			Endpoints: []ScrapeEndpoint{
				{
					Port:     intstr.FromString("web"),
					Interval: "10s",
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
					Port:     intstr.FromInt(8080),
					Interval: "10000ms",
					Timeout:  "5s",
					Path:     "/prometheus",
					HTTPClientConfig: HTTPClientConfig{
						ProxyConfig: ProxyConfig{
							ProxyURL: "http://foo.bar/test",
						},
					},
				},
			},
			TargetLabels: ClusterTargetLabels{
				FromPod: []LabelMapping{
					{From: "key1", To: "key2"},
					{From: "key3"},
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
	scrapeCfgs, err := pmon.ScrapeConfigs("test_project", "test_location", "test_cluster", nil)
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
		`job_name: ClusterPodMonitoring/name1/web
honor_timestamps: false
scrape_interval: 10s
scrape_timeout: 10s
metrics_path: /metrics
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
`,
		`job_name: ClusterPodMonitoring/name1/8080
honor_timestamps: false
scrape_interval: 10s
scrape_timeout: 5s
metrics_path: /prometheus
sample_limit: 1
label_limit: 2
label_name_length_limit: 3
label_value_length_limit: 4
follow_redirects: true
enable_http2: true
proxy_url: http://foo.bar/test
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
- regex: container
  action: labeldrop
- source_labels: [__tmp_instance]
  target_label: instance
  replacement: $1:8080
  action: replace
- source_labels: [__meta_kubernetes_pod_ip]
  target_label: __address__
  replacement: $1:8080
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key1]
  target_label: key2
  action: replace
- source_labels: [__meta_kubernetes_pod_label_key3]
  target_label: key3
  action: replace
kubernetes_sd_configs:
- role: pod
  kubeconfig_file: ""
  follow_redirects: true
  enable_http2: true
  selectors:
  - role: pod
    field: spec.nodeName=$(NODE_NAME)
`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected scrape config YAML (-want, +got): %s", diff)
	}
}
