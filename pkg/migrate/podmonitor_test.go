package migrate

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestPodMonitorConversion(t *testing.T) {
	tests := []struct {
		name    string
		input   *pomonitoringv1.PodMonitor
		wantErr string
	}{
		{
			name: "Broken Config (Empty Namespaces)",
			input: &pomonitoringv1.PodMonitor{
				TypeMeta:   metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: KindPodMonitor},
				ObjectMeta: metav1.ObjectMeta{Name: "broken-monitor", Namespace: "default"},
				Spec: pomonitoringv1.PodMonitorSpec{
					NamespaceSelector: pomonitoringv1.NamespaceSelector{MatchNames: []string{"", "   "}},
				},
			},
			wantErr: "namespaceSelector.matchNames contains only empty or invalid values",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			converter := &PodMonitorConverter{}
			logger := slog.New(slog.NewTextHandler(&testingWriter{t}, nil))
			uInput := toUnstructured(t, tc.input)

			_, err := converter.Convert(context.Background(), logger, uInput, NewResourceCache())

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Convert failed unexpectedly: %v", err)
			}
		})
	}
}

func TestPodMonitorConversion_NilInput(t *testing.T) {
	converter := &PodMonitorConverter{}
	logger := slog.New(slog.NewTextHandler(&testingWriter{t}, nil))

	tests := []struct {
		name    string
		input   *unstructured.Unstructured
		wantErr string
	}{
		{
			name:    "Totally nil unstructured",
			input:   nil,
			wantErr: "cannot convert nil or uninitialized unstructured resource",
		},
		{
			name:    "Empty uninitialized map",
			input:   &unstructured.Unstructured{},
			wantErr: "cannot convert nil or uninitialized unstructured resource",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := converter.Convert(context.Background(), logger, tc.input, NewResourceCache())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestPodMonitorEndpoints(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       pomonitoringv1.PodMetricsEndpoint
		expectErr      string
		expectInterval string
		expectTimeout  string
	}{
		{
			name:      "Missing Port Error",
			endpoint:  pomonitoringv1.PodMetricsEndpoint{Interval: "15s"},
			expectErr: "port or targetPort must be set",
		},
		{
			name:      "Proxy Credentials Error",
			endpoint:  pomonitoringv1.PodMetricsEndpoint{Port: "web", ProxyURL: ptrTo("http://user:pass@proxy.com")},
			expectErr: "proxyUrl contains credentials",
		},
		{
			name:      "Invalid Interval",
			endpoint:  pomonitoringv1.PodMetricsEndpoint{Port: "web", Interval: "15abc"},
			expectErr: "invalid interval",
		},
		{
			name:      "Invalid ScrapeTimeout",
			endpoint:  pomonitoringv1.PodMetricsEndpoint{Port: "web", ScrapeTimeout: "1h30x"},
			expectErr: "invalid scrapeTimeout",
		},
		{
			name:           "Defaulting Logic Empty Interval",
			endpoint:       pomonitoringv1.PodMetricsEndpoint{Port: "web", Interval: "", ScrapeTimeout: ""},
			expectInterval: "30s",
			expectTimeout:  "",
		},
		{
			name:           "Timeout Capped to Interval",
			endpoint:       pomonitoringv1.PodMetricsEndpoint{Port: "api", Interval: "10s", ScrapeTimeout: "30s"},
			expectInterval: "10s",
			expectTimeout:  "10s",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			converter := &PodMonitorConverter{}
			logger := slog.New(slog.NewTextHandler(&testingWriter{t}, nil))
			convCtx := &conversionContext{logger: logger, cache: NewResourceCache(), namespace: "default"}

			epResults := make([]PreScrapeRelabelingResult, 1) // Dummy.
			got, err := converter.convertEndpoints(convCtx, []pomonitoringv1.PodMetricsEndpoint{tc.endpoint}, epResults)

			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("expected exactly 1 endpoint returned, got %d", len(got))
			}

			if tc.expectInterval != "" && got[0].Interval != tc.expectInterval {
				t.Errorf("expected interval %q, got %q", tc.expectInterval, got[0].Interval)
			}
			if tc.expectTimeout != "" && got[0].Timeout != tc.expectTimeout {
				t.Errorf("expected timeout %q, got %q", tc.expectTimeout, got[0].Timeout)
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
