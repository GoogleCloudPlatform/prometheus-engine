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

package migrate

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPodMonitorConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    *pomonitoringv1.PodMonitor
		expected []runtime.Object
		wantErr  string
	}{
		{
			name: "Case A: Cluster-Scoped (Any Namespace)",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "global-monitor",
					Namespace:   "default",
					Labels:      map[string]string{"team": "frontend"},
					Annotations: map[string]string{"prometheus.io/scrape": "true", "kubectl.kubernetes.io/last-applied-configuration": "{}"},
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						Any: true,
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "global-app"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.ClusterPodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindClusterPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-monitor",
					},
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "global-app",
							},
						},
					},
				},
			},
		},
		{
			name: "Case B: Multi-Namespace Split",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						MatchNames: []string{"ns-a", "ns-b"},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "multi-app"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-monitor",
						Namespace: "ns-a",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "multi-app",
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-monitor",
						Namespace: "ns-b",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "multi-app",
							},
						},
					},
				},
			},
		},
		{
			name: "Case B.2: Namespace Deduplication & Trimming",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dirty-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						MatchNames: []string{"ns-a", " ns-a ", "  ns-a", "", "   "},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "dirty-app"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dirty-monitor",
						Namespace: "ns-a",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "dirty-app",
							},
						},
					},
				},
			},
		},
		{
			name: "Case B.3: Broken Config",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						MatchNames: []string{"", "   "},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "broken-app"},
					},
				},
			},
			wantErr: "namespaceSelector.matchNames contains only empty or invalid values",
		},
		{
			name: "Case C: Local Scoping (Omitted Selector)",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-monitor",
					Namespace: "my-local-namespace",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "local-app"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-monitor",
						Namespace: "my-local-namespace",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "local-app",
							},
						},
					},
				},
			},
		},
		{
			name: "Valid Basic Mapping & Capping & Defaulting",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "frontend-monitor",
					Namespace: "frontend",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port:          "web",
							Path:          "/telemetry",
							Scheme:        "HTTPS",
							Interval:      "15s",
							ScrapeTimeout: "10s",
							Params:        map[string][]string{"debug": {"true"}},
						},
						{
							TargetPort:    &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
							Interval:      "10s",
							ScrapeTimeout: "15s",
						},
						{
							Port: "metrics",
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "frontend-monitor",
						Namespace: "frontend",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "frontend",
							},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("web"),
								Path:     "/telemetry",
								Scheme:   "https",
								Interval: "15s",
								Timeout:  "10s",
								Params:   map[string][]string{"debug": {"true"}},
							},
							{
								Port:     intstr.FromInt(8080),
								Interval: "10s",
								Timeout:  "10s",
							},
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
					},
				},
			},
		},
		{
			name: "Target Labels Mapping",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "label-monitor",
					Namespace: "frontend",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					JobLabel:        "app-name",
					PodTargetLabels: []string{"env", "instance", "version"},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label-monitor",
						Namespace: "frontend",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "env"},
								{From: "instance", To: "exported_instance"},
								{From: "version"},
								{From: "app-name", To: "exported_job"},
							},
						},
					},
				},
			},
		},
		{
			name: "Relabeling and Unsupported Warnings Mapping",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "relabel-monitor",
					Namespace: "frontend",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "relabel-app"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port:            "metrics",
							HonorLabels:     true,
							HonorTimestamps: ptrTo(true),
							MetricRelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__name__"},
									TargetLabel:  "instance",
									Action:       "Replace",
								},
								{
									SourceLabels: []pomonitoringv1.LabelName{"container"},
									TargetLabel:  "container_name",
									Action:       "replace",
								},
								{
									SourceLabels: []pomonitoringv1.LabelName{"temp"},
									Action:       "LabelMap",
								},
								{
									SourceLabels: []pomonitoringv1.LabelName{"namespace"},
									Regex:        "default",
									Action:       "keep",
								},
								{
									SourceLabels: []pomonitoringv1.LabelName{"job"},
									Regex:        "api-.*",
									Action:       "drop",
								},
								{
									// Supported action labeldrop (should be kept).
									Action: "labeldrop",
									Regex:  "temp_(.*)",
								},
								{
									// Supported action labelkeep (should be kept).
									Action: "labelkeep",
									Regex:  "(project_id|location|cluster|namespace|job|instance|__address__|must_keep_.*)",
								},
								{
									// Unsupported action lowercase (should be dropped).
									SourceLabels: []pomonitoringv1.LabelName{"__name__"},
									TargetLabel:  "instance",
									Action:       "lowercase",
								},
								{
									// Unsupported action keepequal (should be dropped).
									SourceLabels: []pomonitoringv1.LabelName{"namespace", "job"},
									Action:       "keepequal",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "relabel-monitor",
						Namespace: "frontend",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "relabel-app",
							},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"__name__"},
										TargetLabel:  "exported_instance",
										Action:       "replace",
									},
									{
										SourceLabels: []string{"container"},
										TargetLabel:  "container_name",
										Action:       "replace",
									},
									{
										SourceLabels: []string{"namespace"},
										Regex:        "default",
										Action:       "keep",
									},
									{
										SourceLabels: []string{"job"},
										Regex:        "api-.*",
										Action:       "drop",
									},
									{
										Action: "labeldrop",
										Regex:  "temp_(.*)",
									},
									{
										Action: "labelkeep",
										Regex:  "(project_id|location|cluster|namespace|job|instance|__address__|must_keep_.*)",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Authorization and TLS Mapping",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auth-monitor",
					Namespace: "frontend",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "auth-app"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics-basic",
							BasicAuth: &pomonitoringv1.BasicAuth{
								Username: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "user"},
								Password: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "pass"},
							},
							TLSConfig: &pomonitoringv1.SafeTLSConfig{
								CA: pomonitoringv1.SecretOrConfigMap{
									ConfigMap: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "ca-cm"}, Key: "ca.crt"},
								},
							},
						},
						{
							Port:              "metrics-bearer",
							BearerTokenSecret: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "token-secret"}, Key: "token"},
						},
						{
							Port: "metrics-oauth",
							OAuth2: &pomonitoringv1.OAuth2{
								ClientID: pomonitoringv1.SecretOrConfigMap{
									ConfigMap: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-cm"}, Key: "id"},
								},
								ClientSecret: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-secret"}, Key: "secret"},
								TokenURL:     "https://auth.example.com/token",
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       KindPodMonitoring,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auth-monitor",
						Namespace: "frontend",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "auth-app",
							},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics-basic"),
								Interval: "30s",
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									BasicAuth: &monitoringv1.BasicAuth{
										Username: "<MISSING_SECRET_auth-secret_KEY_user>",
										Password: &monitoringv1.SecretSelector{Secret: &monitoringv1.SecretKeySelector{Name: "auth-secret", Key: "pass", Namespace: "frontend"}},
									},
									TLS: &monitoringv1.TLS{
										CA: &monitoringv1.SecretSelector{Secret: &monitoringv1.SecretKeySelector{Name: "secret-ca-cm", Key: "ca.crt", Namespace: "frontend"}},
									},
								},
							},
							{
								Port:     intstr.FromString("metrics-bearer"),
								Interval: "30s",
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									Authorization: &monitoringv1.Auth{
										Credentials: &monitoringv1.SecretSelector{Secret: &monitoringv1.SecretKeySelector{Name: "token-secret", Key: "token", Namespace: "frontend"}},
									},
								},
							},
							{
								Port:     intstr.FromString("metrics-oauth"),
								Interval: "30s",
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									OAuth2: &monitoringv1.OAuth2{
										ClientID:     "<MISSING_CONFIGMAP_oauth-cm_KEY_id>",
										ClientSecret: &monitoringv1.SecretSelector{Secret: &monitoringv1.SecretKeySelector{Name: "oauth-secret", Key: "secret", Namespace: "frontend"}},
										TokenURL:     "https://auth.example.com/token",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: drop unsupported actions and pod annotations with warning",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "warn-relabel-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									// Pod annotation reference (should be dropped with warning).
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_annotation_commit"},
									TargetLabel:  "commit",
									Action:       "replace",
								},
								{
									// Protected target label rename (should warn and rename to exported_instance).
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									TargetLabel:  "instance",
									Action:       "replace",
								},
								{
									// Unsupported action labelmap (should be dropped with warning).
									Action: "labelmap",
									Regex:  "app_(.*)",
								},
								{
									// Supported action labeldrop (should be kept and promoted).
									Action: "labeldrop",
									Regex:  "temp_(.*)",
								},
								{
									// Supported action labelkeep (should be kept and promoted).
									Action: "labelkeep",
									Regex:  "(project_id|location|cluster|namespace|job|instance|__address__|must_keep_.*)",
								},
								{
									// Supported action hashmod (should be kept and promoted).
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app"},
									TargetLabel:  "shard",
									Modulus:      4,
									Action:       "hashmod",
								},
								{
									// Unsupported action lowercase (should be dropped).
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app"},
									TargetLabel:  "app",
									Action:       "lowercase",
								},
								{
									// Unsupported action keepequal (should be dropped).
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app", "__meta_kubernetes_pod_label_env"},
									Action:       "keepequal",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "warn-relabel-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action: "labeldrop",
										Regex:  "temp_(.*)",
									},
									{
										Action: "labelkeep",
										Regex:  "(project_id|location|cluster|namespace|job|instance|__address__|must_keep_.*)",
									},
									{
										SourceLabels: []string{"app"},
										TargetLabel:  "shard",
										Modulus:      4,
										Action:       "hashmod",
									},
								},
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "env", To: "exported_instance"},
								{From: "app"},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: target filtering keep and drop rules translated to Pod Selector",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "filter-relabel-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									// Exact keep -> matchLabels.
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									Regex:        "^production$",
									Action:       "keep",
								},
								{
									// Set drop -> matchExpressions NotIn.
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_tier"},
									Regex:        "(test|staging)",
									Action:       "drop",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "filter-relabel-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test", "env": "production"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"tier"},
										Regex:        "(test|staging)",
										Action:       "drop",
									},
								},
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "tier"},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: sanitized label names in keep rules not converted to selectors",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sanitized-relabel-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									// Contains underscore (potentially sanitized app.kubernetes.io/name) -> should not become selector.
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app_kubernetes_io_name"},
									Regex:        "^frontend$",
									Action:       "keep",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "sanitized-relabel-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"}, // app_kubernetes_io_name is NOT here.
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"app_kubernetes_io_name"},
										Regex:        "^frontend$",
										Action:       "keep",
									},
								},
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "app_kubernetes_io_name"},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: complex regex substring extraction promoted to post-scrape metricRelabeling",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-relabel-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app_versioned"},
									Regex:        "(.*)-v[0-9]+",
									Replacement:  ptrTo("$1"),
									TargetLabel:  "app_name",
									Action:       "replace",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "complex-relabel-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{{From: "app_versioned"}},
						},

						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{SourceLabels: []string{"app_versioned"}, Regex: "(.*)-v[0-9]+", Replacement: "$1", TargetLabel: "app_name", Action: "replace"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: target filtering rules fall through to metricRelabeling in multi-endpoint resource to prevent cross-endpoint restriction",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-endpoint-filter-relabel",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics-alpha",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									Regex:        "^production$",
									Action:       "keep",
								},
							},
						},
						{
							Port: "metrics-beta",
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "multi-endpoint-filter-relabel", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}, // Remains unchanged.
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{{From: "env"}},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics-alpha"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{SourceLabels: []string{"env"}, Regex: "^production$", TargetLabel: "", Action: "keep"},
								},
							},
							{
								Port:     intstr.FromString("metrics-beta"),
								Interval: "30s",
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: metadata label copy with renamed target falls through to metricRelabeling",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metadata-rename-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_name"},
									TargetLabel:  "custom_pod_name",
									Action:       "replace",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "metadata-rename-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
						TargetLabels: monitoringv1.TargetLabels{
							Metadata: &[]string{"container", "pod", "top_level_controller_name", "top_level_controller_type"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{SourceLabels: []string{"pod"}, TargetLabel: "custom_pod_name", Action: "replace"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: target filtering rules with invalid k8s label values fall through to metricRelabeling",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-selector-value-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									Regex:        "^prod v1$", // Contains space - invalid for k8s label value.
									Action:       "keep",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "invalid-selector-value-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{{From: "env"}},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{SourceLabels: []string{"env"}, Regex: "^prod v1$", TargetLabel: "", Action: "keep"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Unsupported annotation keep rule is dropped, selector remains empty (selecting all pods)",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotation-keep-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{}, // Empty selector selects all pods.
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_annotation_prometheus_io_scrape"},
									Regex:        "true",
									Action:       "keep",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "annotation-keep-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{}, // Remains empty, selecting all pods.
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: custom relabel rule overrides static podTargetLabels",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "override-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodTargetLabels: []string{"env"}, // Static copy env -> env.
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_custom_env"},
									TargetLabel:  "env", // Custom rule maps custom_env -> env.
									Action:       "replace",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "override-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
						TargetLabels: monitoringv1.TargetLabels{
							// Only the custom one should survive because it overrides the static 'env' mapping.
							FromPod: []monitoringv1.LabelMapping{
								{From: "custom_env", To: "env"},
							},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
					},
				},
			},
		},
		{
			name: "Scope-aware Metadata: namespace metadata mapping dropped in namespaced PodMonitoring",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespaced-metadata-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_namespace"},
									TargetLabel:  "namespace",
									Action:       "replace",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta:   BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "namespaced-metadata-monitor", Namespace: "default"},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							Metadata: &[]string{"container", "pod", "top_level_controller_name", "top_level_controller_type"},
						},
					},
				},
			},
		},
		{
			name: "Scope-aware Metadata: namespace metadata mapping kept in ClusterPodMonitoring",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-metadata-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						Any: true,
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_namespace"},
									TargetLabel:  "namespace",
									Action:       "replace",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.ClusterPodMonitoring{
					TypeMeta:   BuildTypeMeta(KindClusterPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-metadata-monitor"},
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							Metadata: ptrTo([]string{"container", "namespace", "pod", "top_level_controller_name", "top_level_controller_type"}),
						},
					},
				},
			},
		},
		{
			name: "Selector conflict: keep rule value conflicts with base matchLabels value",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conflict-selector-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_app"},
									Regex:        "backend",
									Action:       "keep",
								},
							},
						},
					},
				},
			},
			wantErr: "selector conflict: label \"app\" has conflicting values \"frontend\" (base selector) and \"backend\" (relabeling rule)",
		},
		{
			name: "Pre-Scrape Relabelings: drop action rule on pod label falls through to metricRelabelings",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drop-rule-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									Regex:        "dev",
									Action:       "drop",
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{
						Name:      "drop-rule-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"env"},
										Regex:        "dev",
										Action:       "drop",
									},
								},
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "env"},
							},
						},
					},
				},
			},
		},
		{
			name: "Pre-Scrape Relabelings: hashmod action rule promoted to metricRelabelings",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       KindPodMonitor,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "action-promotion-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.PodMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					PodMetricsEndpoints: []pomonitoringv1.PodMetricsEndpoint{
						{
							Port: "metrics",
							RelabelConfigs: []pomonitoringv1.RelabelConfig{
								{
									SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
									TargetLabel:  "env",
									Action:       "hashmod",
									Modulus:      1000,
								},
							},
						},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: BuildTypeMeta(KindPodMonitoring),
					ObjectMeta: metav1.ObjectMeta{
						Name:      "action-promotion-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromString("metrics"),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"env"},
										TargetLabel:  "env",
										Action:       "hashmod",
										Modulus:      1000,
									},
								},
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "env"},
							},
						},
					},
				},
			},
		},
	}

	converter := &PodMonitorConverter{}
	logger := slog.New(slog.NewTextHandler(&testingWriter{t}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uInput := toUnstructured(t, tc.input)

			actual, err := converter.Convert(context.Background(), logger, uInput, NewResourceCache())

			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Convert failed: %v", err)
			}

			if len(actual) != len(tc.expected) {
				t.Fatalf("expected %d output resources, got %d", len(tc.expected), len(actual))
			}

			for i := range actual {
				var gotObj runtime.Object
				switch tc.expected[i].(type) {
				case *monitoringv1.PodMonitoring:
					gotObj = &monitoringv1.PodMonitoring{}
				case *monitoringv1.ClusterPodMonitoring:
					gotObj = &monitoringv1.ClusterPodMonitoring{}
				default:
					t.Fatalf("expected object at index %d must be a pointer to a recognized monitoring type, got %T", i, tc.expected[i])
				}

				err := runtime.DefaultUnstructuredConverter.FromUnstructured(actual[i].Object, gotObj)
				if err != nil {
					t.Fatalf("failed to convert actual to struct: %v", err)
				}

				if diff := cmp.Diff(tc.expected[i], gotObj); diff != "" {
					t.Errorf("mismatch at index %d (-want +got):\n%s", i, diff)
				}

				// Verify that the generated resource compiles successfully
				// inside the GMP Operator's own config generator.
				switch obj := gotObj.(type) {
				case *monitoringv1.PodMonitoring:
					_, err = obj.ScrapeConfigs("test-project", "test-location", "test-cluster", nil)
					if err != nil {
						t.Errorf("Generated PodMonitoring failed operator compilation check: %v", err)
					}
				case *monitoringv1.ClusterPodMonitoring:
					_, err = obj.ScrapeConfigs("test-project", "test-location", "test-cluster", nil)
					if err != nil {
						t.Errorf("Generated ClusterPodMonitoring failed operator compilation check: %v", err)
					}
				}
			}
		})
	}
}

func toUnstructured(t *testing.T, obj any) *unstructured.Unstructured {
	t.Helper()
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("failed to convert object to unstructured: %v", err)
	}
	return &unstructured.Unstructured{Object: m}
}

func ptrTo[T any](v T) *T {
	return &v
}

type testingWriter struct {
	t *testing.T
}

func (w *testingWriter) Write(p []byte) (n int, err error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
