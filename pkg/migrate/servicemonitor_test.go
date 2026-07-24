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

	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceMonitorConverter_Convert(t *testing.T) {
	tests := []struct {
		name         string
		setupCache   func(cache *ResourceCache) error
		inputSM      *pomonitoringv1.ServiceMonitor
		expectedGVK  string
		expectedNS   string
		expectedName string
		verify       func(t *testing.T, outputs []*unstructured.Unstructured)
		wantErr      bool
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
			verify: func(t *testing.T, outputs []*unstructured.Unstructured) {
				if len(outputs) != 1 {
					t.Fatalf("expected 1 output, got %d", len(outputs))
				}
				pm := outputs[0]

				sel, found, _ := unstructured.NestedMap(pm.Object, "spec", "selector", "matchLabels")
				if !found || sel["app"] != "foo-pod" {
					t.Errorf("expected selector app=foo-pod, got %v", sel)
				}

				ports, found, _ := unstructured.NestedSlice(pm.Object, "spec", "endpoints")
				if !found || len(ports) != 1 {
					t.Fatalf("expected 1 endpoint, got %v", ports)
				}
				ep, ok := ports[0].(map[string]any)
				if !ok {
					t.Fatal("failed to cast endpoint")
				}
				port, found, _ := unstructured.NestedFieldNoCopy(ep, "port")
				if !found || port != float64(8080) {
					t.Errorf("expected port 8080, got %v", port)
				}
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
			verify: func(t *testing.T, outputs []*unstructured.Unstructured) {
				// We expect 2 PodMonitorings because selectors conflict.
				if len(outputs) != 2 {
					t.Fatalf("expected 2 outputs due to split, got %d", len(outputs))
				}

				// Check first output (should suffix with service-a).
				pmA := outputs[0]
				if pmA.GetName() != "my-monitor-service-a" {
					t.Errorf("expected name my-monitor-service-a, got %s", pmA.GetName())
				}
				selA, _, _ := unstructured.NestedMap(pmA.Object, "spec", "selector", "matchLabels")
				if selA["app"] != "foo-pod" {
					t.Errorf("expected selector app=foo-pod, got %v", selA)
				}

				// Check second output (should suffix with service-b).
				pmB := outputs[1]
				if pmB.GetName() != "my-monitor-service-b" {
					t.Errorf("expected name my-monitor-service-b, got %s", pmB.GetName())
				}
				selB, _, _ := unstructured.NestedMap(pmB.Object, "spec", "selector", "matchLabels")
				if selB["app"] != "bar-pod" {
					t.Errorf("expected selector app=bar-pod, got %v", selB)
				}
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
			verify: func(t *testing.T, outputs []*unstructured.Unstructured) {
				// We expect 2 PodMonitorings because port mappings conflict.
				if len(outputs) != 2 {
					t.Fatalf("expected 2 outputs due to split, got %d", len(outputs))
				}
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
			verify: func(t *testing.T, outputs []*unstructured.Unstructured) {
				// We expect 2 PodMonitorings because target labels conflict.
				if len(outputs) != 2 {
					t.Fatalf("expected 2 outputs due to split, got %d", len(outputs))
				}
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
			verify: func(t *testing.T, outputs []*unstructured.Unstructured) {
				// We expect only 1 output because the Services are compatible.
				if len(outputs) != 1 {
					t.Fatalf("expected 1 output (merged), got %d", len(outputs))
				}
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

			if tc.verify != nil {
				tc.verify(t, outputs)
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
