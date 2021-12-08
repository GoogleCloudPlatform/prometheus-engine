// Copyright 2021 Google LLC
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

package v1alpha1

import (
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/relabel"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestValidatePodMonitoring(t *testing.T) {
	cases := []struct {
		desc        string
		pm          PodMonitoringSpec
		fail        bool
		errContains string
	}{
		{
			desc: "OK",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
					},
					{
						Port:     intstr.FromInt(8080),
						Interval: "10000ms",
						Timeout:  "5s",
					},
				},
				TargetLabels: TargetLabels{
					FromPod: []LabelMapping{
						{From: "key1", To: "key2"},
						{From: "key3"},
					},
				},
			},
		}, {
			desc: "port missing",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{Interval: "10s"},
				},
			},
			fail:        true,
			errContains: "port must be set",
		}, {
			desc: "scrape interval missing",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{Port: intstr.FromString("web")},
				},
			},
			fail:        true,
			errContains: "invalid scrape interval: empty duration string",
		}, {
			desc: "scrape interval malformed",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "foo",
					},
				},
			},
			fail:        true,
			errContains: "invalid scrape interval: not a valid duration string",
		}, {
			desc: "scrape timeout malformed",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "1s",
						Timeout:  "_",
					},
				},
			},
			fail:        true,
			errContains: "invalid scrape timeout: not a valid duration string",
		}, {
			desc: "scrape timeout greater than interval",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "1s",
						Timeout:  "2s",
					},
				},
			},
			fail:        true,
			errContains: "scrape timeout 2s must not be greater than scrape interval 1s",
		}, {
			desc: "remapping onto prometheus_target label",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
					},
				},
				TargetLabels: TargetLabels{
					FromPod: []LabelMapping{
						{From: "key1", To: "cluster"},
					},
				},
			},
			fail:        true,
			errContains: `invalid PodMonitoring target labels: cannot relabel with action "replace" onto protected label "cluster"`,
		}, {
			// A simple error that should be caught by invoking the upstream validation. We don't
			// have to cover everything it covers.
			desc: "remapping onto bad label name",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
					},
				},
				TargetLabels: TargetLabels{
					FromPod: []LabelMapping{
						{From: "key1", To: "foo-bar"},
					},
				},
			},
			fail:        true,
			errContains: `"foo-bar" is invalid 'target_label' for replace action`,
		}, {
			desc: "metric relabeling: labelmap forbidden",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						MetricRelabeling: []RelabelingRule{
							{
								SourceLabels: []string{"foo", "bar"},
								Action:       "labelmap",
							},
						},
					},
				},
			},
			fail:        true,
			errContains: `relabeling with action "labelmap" not allowed`,
		}, {
			desc: "metric relabeling: protected replace label",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						MetricRelabeling: []RelabelingRule{
							{
								Action:      "replace",
								TargetLabel: "project_id",
							},
						},
					},
				},
			},
			fail:        true,
			errContains: `cannot relabel with action "replace" onto protected label "project_id"`,
		}, {
			desc: "metric relabeling: protected labelkeep",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						MetricRelabeling: []RelabelingRule{
							{
								Action: "labelkeep",
								// project_id label is not kept.
								Regex: "(cluster|location|cluster|namespace|job|instance|__address__)",
							},
						},
					},
				},
			},
			fail:        true,
			errContains: `regex (cluster|location|cluster|namespace|job|instance|__address__) would drop at least one of the protected labels project_id, location, cluster, namespace, job, instance, __address__`,
		}, {
			desc: "metric relabeling: protected labeldrop",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						MetricRelabeling: []RelabelingRule{
							{
								Action: "labeldrop",
								Regex:  "n?amespace",
							},
						},
					},
				},
			},
			fail:        true,
			errContains: `regex n?amespace would drop at least one of the protected labels project_id, location, cluster, namespace, job, instance, __address__`,
		}, {
			desc: "invalid URL",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						ProxyURL: "_:_",
					},
				},
			},
			fail:        true,
			errContains: `invalid proxy URL`,
		}, {
			desc: "proxy URL with password",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
						ProxyURL: "http://user:password@foo.bar/",
					},
				},
			},
			fail:        true,
			errContains: `passwords encoded in URLs are not supported`,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			pm := &PodMonitoring{
				Spec: c.pm,
			}
			err := pm.ValidateCreate()
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
					ProxyURL: "http://foo.bar/test",
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
	scrapeCfgs, err := pmon.ScrapeConfigs()
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
proxy_url: ""
follow_redirects: true
relabel_configs:
- source_labels: [__meta_kubernetes_namespace]
  regex: ns1
  action: keep
- source_labels: [__meta_kubernetes_namespace]
  target_label: namespace
  action: replace
- target_label: job
  replacement: name1
  action: replace
- source_labels: [__meta_kubernetes_pod_container_port_name]
  regex: web
  action: keep
- source_labels: [__meta_kubernetes_pod_name, __meta_kubernetes_pod_container_port_name]
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
  follow_redirects: true
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
proxy_url: http://foo.bar/test
follow_redirects: true
relabel_configs:
- source_labels: [__meta_kubernetes_namespace]
  regex: ns1
  action: keep
- source_labels: [__meta_kubernetes_namespace]
  target_label: namespace
  action: replace
- target_label: job
  replacement: name1
  action: replace
- source_labels: [__meta_kubernetes_pod_name]
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
  follow_redirects: true
  selectors:
  - role: pod
    field: spec.nodeName=$(NODE_NAME)
`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected scrape config YAML (-want, +got): %s", diff)
	}
}
