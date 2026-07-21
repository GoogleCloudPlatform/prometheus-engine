// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package golden_test

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// ============================================================================
// Section A: Upstream Prometheus Operator Test Cases (v0.75.0)
// ============================================================================

// upstreamPodMonitorTestCases maps exact upstream test scenarios from pkg/prometheus/promcfg_test.go.
var upstreamPodMonitorTestCases = []podMonitorTestCase{
	// Upstream Function: TestNamespaceSetCorrectlyForPodMonitor
	// Tests: NamespaceSelector scoping and AttachMetadata node configuration.
	{
		name: "podmonitor-namespace-selection",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				NamespaceSelector: monitoringv1.NamespaceSelector{MatchNames: []string{"test"}},
				AttachMetadata:    &monitoringv1.AttachMetadata{Node: ptr.To(true)},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{Port: "web", Interval: "30s"},
				},
			},
		},
	},

	// Upstream Function: TestSettingHonorTimestampsInPodMonitor
	// Tests: HonorTimestamps configuration flag mapping.
	{
		name: "podmonitor-setting-honor-timestamps",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						HonorTimestamps: ptr.To(false),
						Port:            "web",
						Interval:        "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestSettingTrackTimestampsStalenessInPodMonitor
	// Tests: TrackTimestampsStaleness configuration flag handling.
	{
		name: "podmonitor-track-timestamps-staleness",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TrackTimestampsStaleness: ptr.To(false),
						Port:                     "web",
						Interval:                 "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestSettingScrapeProtocolsInPodMonitor
	// Tests: ScrapeProtocols configuration options handling.
	{
		name: "podmonitor-scrape-protocols",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				ScrapeProtocols: []monitoringv1.ScrapeProtocol{
					monitoringv1.ScrapeProtocol("OpenMetricsText1.0.0"),
					monitoringv1.ScrapeProtocol("OpenMetricsText0.0.1"),
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TrackTimestampsStaleness: ptr.To(false),
						Port:                     "web",
						Interval:                 "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodTargetLabelsFromPodMonitor
	// Tests: Custom PodTargetLabels extraction.
	{
		name: "podmonitor-pod-target-labels",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{Port: "web", Interval: "30s"},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPageRedirects
	// Tests: FollowRedirects configuration settings.
	{
		name: "podmonitor-follow-redirects",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "pod-monitor-ns",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{MatchLabels: map[string]string{"group": "group1"}},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:            "web",
						Interval:        "30s",
						FollowRedirects: ptr.To(false),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPhaseFilter
	// Tests: FilterRunning endpoint-level warning configuration.
	{
		name: "podmonitor-phase-filter",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						FilterRunning: ptr.To(false),
						Port:          "test",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorEndpointEnableHttp2
	// Tests: EnableHttp2 TLS configuration options.
	{
		name: "podmonitor-http2-enablement",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "pod-monitor-ns",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{MatchLabels: map[string]string{"group": "group1"}},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:        "web",
						Interval:    "30s",
						EnableHttp2: ptr.To(true),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with Pod Name
	{
		name: "podmonitor-port-name",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-port-name",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:       "podname",
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with Pod Port Number
	{
		name: "podmonitor-port-number-with-name",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-port-num-name",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with TargetPort Int
	{
		name: "podmonitor-targetport-int",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-targetport-int",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromInt(10240)),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with TargetPort string
	{
		name: "podmonitor-targetport-string",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-targetport-string",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with Match Label Selector
	{
		name: "podmonitor-selector-match-labels",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "group1",
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with Match Expression Selector
	{
		name: "podmonitor-selector-match-expressions",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "group",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"group1"},
						},
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with selector and match expression selector (multiple criteria)
	{
		name: "podmonitor-selector-match-labels-and-expressions",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "group1",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "group",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"group2"},
						},
						{
							Key:      "groupb",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},
}

// ============================================================================
// Section B: Custom GMP Migration Test Cases (Limits, Auth, TLS, HTTP)
// ============================================================================

// customPodMonitorTestCases maps Google Managed Service for Prometheus (GMP) specific mapping gaps
// and edge cases that are not validated by the upstream Prometheus Operator test suite.
var customPodMonitorTestCases = []podMonitorTestCase{
	// Custom Case: Test mapping of scrape limits.
	{
		name: "podmonitor-scrape-limits",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-limits",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				SampleLimit:           ptr.To(uint64(1000)),
				LabelLimit:            ptr.To(uint64(50)),
				LabelNameLengthLimit:  ptr.To(uint64(64)),
				LabelValueLengthLimit: ptr.To(uint64(128)),
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{Port: "web"},
				},
			},
		},
	},

	// Custom Case: Test mapping of Authorization, BasicAuth, OAuth2, and TLS Config.
	{
		name: "podmonitor-auth-and-tls",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-auth",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port: "web",
						BasicAuth: &monitoringv1.BasicAuth{
							Username: corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-basic-secret"},
								Key:                  "user",
							},
							Password: corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-basic-secret"},
								Key:                  "pass",
							},
						},
						TLSConfig: &monitoringv1.SafeTLSConfig{
							CA: monitoringv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-tls-secret"},
									Key:                  "ca.crt",
								},
							},
							Cert: monitoringv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-tls-secret"},
									Key:                  "cert.crt",
								},
							},
							KeySecret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-tls-secret"},
								Key:                  "cert.key",
							},
							ServerName:         ptr.To("my-server"),
							InsecureSkipVerify: ptr.To(true),
						},
						OAuth2: &monitoringv1.OAuth2{
							ClientID: monitoringv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-secret"},
									Key:                  "client-id",
								},
							},
							ClientSecret: corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-secret"},
								Key:                  "client-secret",
							},
							TokenURL: "https://auth.example.com/token",
							Scopes:   []string{"read", "write"},
							EndpointParams: map[string]string{
								"audience": "my-audience",
							},
						},
					},
				},
			},
		},
		extraInputs: []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "my-basic-secret",
						"namespace": "default",
					},
					"stringData": map[string]interface{}{
						"user": "my-username",
						"pass": "my-password",
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "oauth-secret",
						"namespace": "default",
					},
					"stringData": map[string]interface{}{
						"client-id":     "my-client-id",
						"client-secret": "my-client-secret",
					},
				},
			},
		},
	},

	// Custom Case: Test mapping of Scheme, Path, GET parameters, and MetricRelabeling.
	{
		name: "podmonitor-http-customizations",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-http",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:   "web",
						Scheme: "https",
						Path:   "/metrics/v2",
						Params: map[string][]string{
							"collect[]": {"foo", "bar"},
						},
						MetricRelabelConfigs: []monitoringv1.RelabelConfig{
							{
								SourceLabels: []monitoringv1.LabelName{"__name__"},
								TargetLabel:  "__name__",
								Regex:        "up",
								Action:       "keep",
							},
						},
					},
				},
			},
		},
	},

	// Custom Case: Test mapping of pre-scrape relabel configs to selectors, target labels, and metric relabelings.
	{
		name: "podmonitor-relabel-promotions",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-relabel-ok",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port: "web",
						RelabelConfigs: []monitoringv1.RelabelConfig{
							// A. Should translate to selector.matchLabels
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
								TargetLabel:  "env",
								Regex:        "^production$",
								Action:       "keep",
							},
							// B. Should translate to targetLabels.fromPod
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_label_app"},
								TargetLabel:  "app",
								Action:       "replace",
							},
							// C. Should translate to targetLabels.metadata
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_node_name"},
								TargetLabel:  "node",
								Action:       "replace",
							},
							// D. Complex replacement -> should promote to metricRelabeling
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_label_version"},
								TargetLabel:  "version",
								Regex:        "v1\\.(.*)",
								Replacement:  ptr.To("version-1-$1"),
								Action:       "replace",
							},
						},
					},
				},
			},
		},
	},

	// Custom Case: Test warnings, label renames, and dropped configurations in pre-scrape relabel configs.
	{
		name: "podmonitor-relabel-warnings",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-relabel-warn",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port: "web",
						RelabelConfigs: []monitoringv1.RelabelConfig{
							// A. Protected target rename: job -> exported_job
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_label_project"},
								TargetLabel:  "job",
								Action:       "replace",
							},
							// B. Unsupported annotation source: dropped
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_annotation_owner"},
								TargetLabel:  "owner",
								Action:       "replace",
							},
							// C. Unsupported action: dropped
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_label_tier"},
								Action:       "labelmap",
							},
						},
					},
				},
			},
		},
	},

	// Custom Case: Test the warning emitted when multiple endpoints have conflicting phase filters.
	{
		name: "podmonitor-filter-running-conflict",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-conflict",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:          "web",
						FilterRunning: ptr.To(false),
					},
					{
						Port:          "metrics",
						FilterRunning: ptr.To(true),
					},
				},
			},
		},
	},
}
