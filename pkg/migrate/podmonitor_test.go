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
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestPodMonitorNamespaceHarness(t *testing.T) {
	converter := &PodMonitorConverter{}

	// Helper to build unstructured logger that writes to go test logs
	logger := slog.New(slog.NewTextHandler(&testingWriter{t}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	t.Run("Case A: Cluster-Scoped (Any Namespace)", func(t *testing.T) {
		pm := &pomonitoringv1.PodMonitor{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "monitoring.coreos.com/v1",
				Kind:       KindPodMonitor,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global-monitor",
				Namespace: "default",
			},
			Spec: pomonitoringv1.PodMonitorSpec{
				NamespaceSelector: pomonitoringv1.NamespaceSelector{
					Any: true,
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "global-app"},
				},
			},
		}

		uInput := toUnstructured(t, pm)

		outputs, err := converter.Convert(context.Background(), logger, uInput, nil)
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		if len(outputs) != 1 {
			t.Fatalf("expected exactly 1 output, got %d", len(outputs))
		}

		out := outputs[0]
		if out.GetKind() != KindClusterPodMonitoring {
			t.Errorf("expected Kind %q, got %q", KindClusterPodMonitoring, out.GetKind())
		}
		if out.GetAPIVersion() != GMPAPIVersion {
			t.Errorf("expected APIVersion %q, got %q", GMPAPIVersion, out.GetAPIVersion())
		}
		if out.GetName() != "global-monitor" {
			t.Errorf("expected Name 'global-monitor', got %q", out.GetName())
		}
		if out.GetNamespace() != "" {
			t.Errorf("expected empty Namespace for cluster-scoped resource, got %q", out.GetNamespace())
		}

		// Verify spec selector mapped
		var gmpCPM monitoringv1.ClusterPodMonitoring
		fromUnstructured(t, out, &gmpCPM)
		if gmpCPM.Spec.Selector.MatchLabels["app"] != "global-app" {
			t.Errorf("expected selector app='global-app', got %v", gmpCPM.Spec.Selector.MatchLabels)
		}
	})

	t.Run("Case B: Multi-Namespace Split (Unstructured Cloning)", func(t *testing.T) {
		pm := &pomonitoringv1.PodMonitor{
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
		}

		uInput := toUnstructured(t, pm)

		outputs, err := converter.Convert(context.Background(), logger, uInput, nil)
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		if len(outputs) != 2 {
			t.Fatalf("expected exactly 2 outputs, got %d", len(outputs))
		}

		// Verify each output resource
		namespacesSeen := make(map[string]bool)
		for _, out := range outputs {
			if out.GetKind() != KindPodMonitoring {
				t.Errorf("expected Kind %q, got %q", KindPodMonitoring, out.GetKind())
			}
			if out.GetName() != "multi-monitor" {
				t.Errorf("expected Name 'multi-monitor', got %q", out.GetName())
			}
			namespacesSeen[out.GetNamespace()] = true

			var gmpPM monitoringv1.PodMonitoring
			fromUnstructured(t, out, &gmpPM)
			if gmpPM.Spec.Selector.MatchLabels["app"] != "multi-app" {
				t.Errorf("expected selector app='multi-app', got %v", gmpPM.Spec.Selector.MatchLabels)
			}
		}

		if !namespacesSeen["ns-a"] || !namespacesSeen["ns-b"] {
			t.Errorf("expected outputs in namespaces ns-a and ns-b, got %v", namespacesSeen)
		}
	})

	t.Run("Case B.2: Namespace Deduplication & Trimming", func(t *testing.T) {
		pm := &pomonitoringv1.PodMonitor{
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
		}

		uInput := toUnstructured(t, pm)

		outputs, err := converter.Convert(context.Background(), logger, uInput, nil)
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		// Should deduplicate "ns-a" and filter out the empty/whitespace entries, yielding EXACTLY 1 resource
		if len(outputs) != 1 {
			t.Fatalf("expected exactly 1 output after deduplication, got %d", len(outputs))
		}

		out := outputs[0]
		if out.GetNamespace() != "ns-a" {
			t.Errorf("expected trimmed namespace 'ns-a', got %q", out.GetNamespace())
		}
	})

	t.Run("Case B.3: Broken Config (Skip and Warn)", func(t *testing.T) {
		pm := &pomonitoringv1.PodMonitor{
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
		}

		uInput := toUnstructured(t, pm)
		_, err := converter.Convert(context.Background(), logger, uInput, nil)
		if err == nil {
			t.Fatal("expected Convert to fail on empty matchNames, but got no error")
		}
		if !strings.Contains(err.Error(), "contains only empty or invalid values") {
			t.Errorf("expected error message to contain 'contains only empty or invalid values', got %v", err)
		}
	})

	t.Run("Case C: Local Scoping (Omitted Selector)", func(t *testing.T) {
		pm := &pomonitoringv1.PodMonitor{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "monitoring.coreos.com/v1",
				Kind:       KindPodMonitor,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-monitor",
				Namespace: "my-local-namespace",
			},
			Spec: pomonitoringv1.PodMonitorSpec{
				// NamespaceSelector is completely omitted
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "local-app"},
				},
			},
		}

		uInput := toUnstructured(t, pm)

		outputs, err := converter.Convert(context.Background(), logger, uInput, nil)
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		if len(outputs) != 1 {
			t.Fatalf("expected exactly 1 output, got %d", len(outputs))
		}

		out := outputs[0]
		if out.GetKind() != KindPodMonitoring {
			t.Errorf("expected Kind %q, got %q", KindPodMonitoring, out.GetKind())
		}
		if out.GetNamespace() != "my-local-namespace" {
			t.Errorf("expected Namespace 'my-local-namespace', got %q", out.GetNamespace())
		}

		var gmpPM monitoringv1.PodMonitoring
		fromUnstructured(t, out, &gmpPM)
		if gmpPM.Spec.Selector.MatchLabels["app"] != "local-app" {
			t.Errorf("expected selector app='local-app', got %v", gmpPM.Spec.Selector.MatchLabels)
		}
	})
}

// Helpers for test conversions.
func toUnstructured(t *testing.T, obj any) *unstructured.Unstructured {
	t.Helper()
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("failed to convert object to unstructured: %v", err)
	}
	return &unstructured.Unstructured{Object: m}
}

func fromUnstructured(t *testing.T, u *unstructured.Unstructured, obj any) {
	t.Helper()
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		t.Fatalf("failed to convert unstructured to object: %v", err)
	}
}

// testingWriter redirects slog output to go test logging.
type testingWriter struct {
	t *testing.T
}

func (w *testingWriter) Write(p []byte) (n int, err error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
