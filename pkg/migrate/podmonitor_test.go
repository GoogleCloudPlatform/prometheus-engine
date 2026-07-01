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
	"reflect"
	"strings"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
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
						Labels: map[string]string{
							"team": "frontend",
						},
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
						},
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
								},
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

			actual, err := converter.Convert(context.Background(), logger, uInput, nil)

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
				expectedVal := reflect.ValueOf(tc.expected[i])
				if expectedVal.Kind() != reflect.Pointer {
					t.Fatalf("expected object at index %d must be a pointer, got %T", i, tc.expected[i])
				}
				gotObj := reflect.New(expectedVal.Elem().Type()).Interface().(runtime.Object)

				err := runtime.DefaultUnstructuredConverter.FromUnstructured(actual[i].Object, gotObj)
				if err != nil {
					t.Fatalf("failed to convert actual to struct: %v", err)
				}

				if diff := cmp.Diff(tc.expected[i], gotObj); diff != "" {
					t.Errorf("mismatch at index %d (-want +got):\n%s", i, diff)
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
