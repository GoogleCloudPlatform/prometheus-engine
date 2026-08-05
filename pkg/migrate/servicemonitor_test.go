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
	"os"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/relabel"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceMonitorConverter_Convert(t *testing.T) {
	tests := []struct {
		name       string
		setupCache func(cache *ResourceCache) error
		inputSM    *pomonitoringv1.ServiceMonitor
		expected   []runtime.Object
		wantErr    bool
	}{
		{
			name: "Basic ServiceMonitor conversion",
			setupCache: func(cache *ResourceCache) error {
				return addServiceWithSelectorToCache(cache, "default", "my-service",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Split on selector conflict",
			setupCache: func(cache *ResourceCache) error {
				// Service A targets foo-pod.
				err := addServiceWithSelectorToCache(cache, "default", "service-a",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
				if err != nil {
					return err
				}
				// Service B targets bar-pod.
				return addServiceWithSelectorToCache(cache, "default", "service-b",
					map[string]string{"app": "foo"},
					map[string]string{"app": "bar-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-a",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-b",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "bar-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Split on port conflict",
			setupCache: func(cache *ResourceCache) error {
				// Both target the same pods (foo-pod) but map 'web' to different container ports.
				err := addServiceWithSelectorToCache(cache, "default", "service-a",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
				if err != nil {
					return err
				}
				return addServiceWithSelectorToCache(cache, "default", "service-b",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(9090)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-a",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-b",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(9090),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Split on label conflict",
			setupCache: func(cache *ResourceCache) error {
				// Same selector, same ports, but different team labels.
				err := addServiceWithLabelsAndSelectorToCache(cache, "default", "service-a",
					map[string]string{"app": "foo", "team": "alpha"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
				if err != nil {
					return err
				}
				return addServiceWithLabelsAndSelectorToCache(cache, "default", "service-b",
					map[string]string{"app": "foo", "team": "beta"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector:     metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					TargetLabels: []string{"team"},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-a",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										TargetLabel: "team",
										Replacement: "alpha",
										Action:      "replace",
									},
								},
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-b",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										TargetLabel: "team",
										Replacement: "beta",
										Action:      "replace",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Split when labeled and unlabeled Services match selector",
			setupCache: func(cache *ResourceCache) error {
				if err := addServiceWithSelectorToCache(cache, "default", "service-labeled",
					map[string]string{"app": "foo", "team": "alpha"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				); err != nil {
					return err
				}
				return addServiceWithSelectorToCache(cache, "default", "service-unlabeled",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector:     metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					TargetLabels: []string{"team"},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-labeled",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										TargetLabel: "team",
										Replacement: "alpha",
										Action:      string(relabel.Replace),
									},
								},
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-unlabeled",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Merge compatible services",
			setupCache: func(cache *ResourceCache) error {
				// Headless and ClusterIP targeting same pods with same port mapping.
				err := addServiceWithSelectorToCache(cache, "default", "redis",
					map[string]string{"app": "redis"},
					map[string]string{"app": "redis-pod"},
					[]corev1.ServicePort{
						{Name: "redis", Port: 6379, TargetPort: intstr.FromInt32(6379)},
					},
				)
				if err != nil {
					return err
				}
				return addServiceWithSelectorToCache(cache, "default", "redis-headless",
					map[string]string{"app": "redis"},
					map[string]string{"app": "redis-pod"},
					[]corev1.ServicePort{
						{Name: "redis", Port: 6379, TargetPort: intstr.FromInt32(6379)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "redis"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "redis"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "redis-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "redis-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(6379),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Split across multiple namespaces",
			setupCache: func(cache *ResourceCache) error {
				err := addServiceWithSelectorToCache(cache, "ns-1", "service-a",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod-1"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
				if err != nil {
					return err
				}
				return addServiceWithSelectorToCache(cache, "ns-2", "service-b",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod-2"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{
						MatchNames: []string{"ns-1", "ns-2"},
					},
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-a",
						Namespace: "ns-1",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod-1"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor-service-b",
						Namespace: "ns-2",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod-2"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:       "Missing backing Service",
			setupCache: func(_ *ResourceCache) error { return nil },
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Backing Service has no selector",
			setupCache: func(cache *ResourceCache) error {
				return addServiceWithSelectorToCache(cache, "default", "my-service",
					map[string]string{"app": "foo"},
					nil, // No selector.
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Endpoint missing port and targetPort",
			setupCache: func(cache *ResourceCache) error {
				return addServiceWithSelectorToCache(cache, "default", "my-service",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Path: "/metrics"}, // Neither Port nor TargetPort is specified.
					},
				},
			},
			wantErr: true,
		},
		{
			name: "JobLabel conversion from Service",
			setupCache: func(cache *ResourceCache) error {
				return addServiceWithSelectorToCache(cache, "default", "my-service",
					map[string]string{"app": "foo", "job-key": "my-custom-job"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					JobLabel: "job-key",
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{Port: "web"},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										TargetLabel: "exported_job",
										Replacement: "my-custom-job",
										Action:      string(relabel.Replace),
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "ServiceMonitor with TargetPort fallback",
			setupCache: func(cache *ResourceCache) error {
				return addServiceWithSelectorToCache(cache, "default", "my-service",
					map[string]string{"app": "foo"},
					map[string]string{"app": "foo-pod"},
					[]corev1.ServicePort{
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				)
			},
			inputSM: &pomonitoringv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-monitor",
					Namespace: "default",
				},
				Spec: pomonitoringv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Endpoints: []pomonitoringv1.Endpoint{
						{TargetPort: &intstr.IntOrString{Type: intstr.Int, IntVal: 8080}},
					},
				},
			},
			expected: []runtime.Object{
				&monitoringv1.PodMonitoring{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "monitoring.googleapis.com/v1",
						Kind:       "PodMonitoring",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-monitor",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo-pod"},
						},
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port:     intstr.FromInt32(8080),
								Interval: "30s",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewResourceCache()
			if err := tc.setupCache(cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			smUnstruct, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.inputSM)
			if err != nil {
				t.Fatalf("failed to convert ServiceMonitor to unstructured: %v", err)
			}

			converter := &ServiceMonitorConverter{}
			outputs, err := converter.Convert(context.Background(), logger, &unstructured.Unstructured{Object: smUnstruct}, cache)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Convert() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if tc.expected != nil {
				if len(outputs) != len(tc.expected) {
					t.Fatalf("expected %d outputs, got %d", len(tc.expected), len(outputs))
				}
				for i := range tc.expected {
					var gotObj runtime.Object
					switch tc.expected[i].(type) {
					case *monitoringv1.PodMonitoring:
						gotObj = &monitoringv1.PodMonitoring{}
					case *monitoringv1.ClusterPodMonitoring:
						gotObj = &monitoringv1.ClusterPodMonitoring{}
					default:
						t.Fatalf("expected object at index %d must be a pointer to a recognized monitoring type, got %T", i, tc.expected[i])
					}

					err := runtime.DefaultUnstructuredConverter.FromUnstructured(outputs[i].Object, gotObj)
					if err != nil {
						t.Fatalf("failed to convert actual to struct: %v", err)
					}

					if diff := cmp.Diff(tc.expected[i], gotObj); diff != "" {
						t.Errorf("mismatch at index %d (-want +got):\n%s", i, diff)
					}
				}
			}
		})
	}
}

func addServiceWithSelectorToCache(cache *ResourceCache, namespace, name string, labels map[string]string, selector map[string]string, ports []corev1.ServicePort) error {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    ports,
		},
	}
	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	return cache.Add(&unstructured.Unstructured{Object: u})
}

func addServiceWithLabelsAndSelectorToCache(cache *ResourceCache, namespace, name string, labels map[string]string, selector map[string]string, ports []corev1.ServicePort) error {
	return addServiceWithSelectorToCache(cache, namespace, name, labels, selector, ports)
}

// TestConvertStaticTargetLabels tests that label keys are sanitized and protected labels are renamed to exported_<label>.
func TestConvertStaticTargetLabels(t *testing.T) {
	logger := slog.Default()
	labels := map[string]string{
		"app":                    "my-app",
		"app.kubernetes.io/name": "my-k8s-app",
		"job":                    "my-job",
		"namespace":              "my-ns",
		"project.id":             "my-project",
	}

	rules := convertStaticTargetLabels(logger, labels)
	expected := []monitoringv1.RelabelingRule{
		{
			TargetLabel: "app",
			Replacement: "my-app",
			Action:      "replace",
		},
		{
			TargetLabel: "app_kubernetes_io_name",
			Replacement: "my-k8s-app",
			Action:      "replace",
		},
		{
			TargetLabel: "exported_job",
			Replacement: "my-job",
			Action:      "replace",
		},
		{
			TargetLabel: "exported_namespace",
			Replacement: "my-ns",
			Action:      "replace",
		},
		{
			TargetLabel: "exported_project_id",
			Replacement: "my-project",
			Action:      "replace",
		},
	}

	if diff := cmp.Diff(expected, rules); diff != "" {
		t.Errorf("convertStaticTargetLabels() mismatch (-want +got):\n%s", diff)
	}
}
