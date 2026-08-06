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
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestPodMonitorConverter implements ResourceConverter for testing.
type TestPodMonitorConverter struct {
	calls int
}

func (t *TestPodMonitorConverter) ImportKey() string {
	return "PodMonitor"
}

func (t *TestPodMonitorConverter) Convert(_ context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, error) {
	t.calls++

	_, found := cache.Get("Service", unstruct.GetNamespace(), "backing-service")

	if !found {
		logger.Warn("backing-service not found in cache")
	} else {
		logger.Info("Successfully resolved backing-service")
	}

	out := &unstructured.Unstructured{}
	out.SetGroupVersionKind(unstruct.GroupVersionKind())
	out.SetKind("TranslatedDummy")
	out.SetName("translated-" + unstruct.GetName())
	out.SetNamespace(unstruct.GetNamespace())

	return []*unstructured.Unstructured{out}, nil
}

func TestMigratorRun(t *testing.T) {
	tests := []struct {
		name            string
		setupInputs     func(t *testing.T, tmpDir string) (inputPaths []string, stdinReader *strings.Reader)
		expectedSuccess int
		expectedWarning int
		expectedSkipped int
		expectedFailed  int
		expectedOutputs int
		wantStderrLogs  []string
		expectRunErr    bool
	}{
		{
			name: "Single file input & converter extensibility",
			setupInputs: func(t *testing.T, tmpDir string) ([]string, *strings.Reader) {
				yamlContent := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: my-monitor
  namespace: default
spec:
  foo: bar
---
apiVersion: v1
kind: Service
metadata:
  name: backing-service
  namespace: default
spec:
  ports:
  - port: 80
`
				p := filepath.Join(tmpDir, "input.yaml")
				if err := os.WriteFile(p, []byte(yamlContent), 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				return []string{p}, nil
			},
			expectedSuccess: 1,
			expectedOutputs: 1,
			wantStderrLogs: []string{
				"[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service",
				"[SUCCESS] [PodMonitor:default/my-monitor] Converted successfully",
			},
		},
		{
			name: "Malformed YAML input",
			setupInputs: func(t *testing.T, tmpDir string) ([]string, *strings.Reader) {
				malformedYAML := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: default
`
				p := filepath.Join(tmpDir, "bad_resource.yaml")
				if err := os.WriteFile(p, []byte(malformedYAML), 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				return []string{tmpDir}, nil
			},
			expectedFailed: 1,
			wantStderrLogs: []string{
				"Skipping file due to parse error",
				"malformed resource: apiVersion, kind, and metadata.name must all be specified",
			},
		},
		{
			name: "Skipped unsupported resource",
			setupInputs: func(t *testing.T, tmpDir string) ([]string, *strings.Reader) {
				skippedYAML := `
apiVersion: monitoring.coreos.com/v1
kind: Alertmanager
metadata:
  name: my-alertmanager
spec:
  replicas: 3
`
				p := filepath.Join(tmpDir, "skipped.yaml")
				if err := os.WriteFile(p, []byte(skippedYAML), 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				return []string{tmpDir}, nil
			},
			expectedSkipped: 1,
			wantStderrLogs: []string{
				"[SKIPPED] [Alertmanager:my-alertmanager] Skipping unsupported Prometheus Operator resource",
			},
		},
		{
			name: "Multiple input files",
			setupInputs: func(t *testing.T, tmpDir string) ([]string, *strings.Reader) {
				svcYAML := `
apiVersion: v1
kind: Service
metadata:
  name: backing-service
  namespace: default
spec:
  ports:
  - port: 80
`
				pmYAML := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: my-monitor
  namespace: default
spec:
  foo: bar
`
				p1 := filepath.Join(tmpDir, "svc.yaml")
				p2 := filepath.Join(tmpDir, "pm.yaml")
				_ = os.WriteFile(p1, []byte(svcYAML), 0644)
				_ = os.WriteFile(p2, []byte(pmYAML), 0644)
				return []string{p1, p2}, nil
			},
			expectedSuccess: 1,
			expectedOutputs: 1,
			wantStderrLogs: []string{
				"[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service",
			},
		},
		{
			name: "Piped v1.List from Stdin",
			setupInputs: func(_ *testing.T, _ string) ([]string, *strings.Reader) {
				listYAML := `
apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: Service
  metadata:
    name: backing-service
    namespace: default
  spec:
    ports:
    - port: 80
- apiVersion: monitoring.coreos.com/v1
  kind: PodMonitor
  metadata:
    name: my-monitor
    namespace: default
  spec:
    foo: bar
`
				return []string{"-"}, strings.NewReader(listYAML)
			},
			expectedSuccess: 1,
			expectedOutputs: 1,
			wantStderrLogs: []string{
				"[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service",
			},
		},
		{
			name: "Directory walk skips hidden files and hidden subdirectories",
			setupInputs: func(t *testing.T, tmpDir string) ([]string, *strings.Reader) {
				validYAML := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: my-monitor
  namespace: default
spec:
  foo: bar
---
apiVersion: v1
kind: Service
metadata:
  name: backing-service
  namespace: default
spec:
  ports:
  - port: 80
`
				hiddenDir := filepath.Join(tmpDir, ".hidden")
				if err := os.MkdirAll(hiddenDir, 0755); err != nil {
					t.Fatalf("failed to create hidden dir: %v", err)
				}
				_ = os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(validYAML), 0644)
				_ = os.WriteFile(filepath.Join(hiddenDir, "bad.yaml"), []byte("invalid-yaml"), 0644)
				_ = os.WriteFile(filepath.Join(tmpDir, ".yamllint.yaml"), []byte("invalid-yaml"), 0644)
				return []string{tmpDir}, nil
			},
			expectedSuccess: 1,
			expectedFailed:  0,
			expectedOutputs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			inputPaths, stdinReader := tc.setupInputs(t, tmpDir)

			migrator := NewMigrator()
			var stdoutBuf, stderrBuf bytes.Buffer
			migrator.Stdout = &stdoutBuf
			migrator.Stderr = &stderrBuf
			if stdinReader != nil {
				migrator.Stdin = stdinReader
			}

			testConv := &TestPodMonitorConverter{}
			migrator.RegisterConverter(testConv)

			report, err := migrator.Run(inputPaths...)
			if (err != nil) != tc.expectRunErr {
				t.Fatalf("Run() error = %v, expectRunErr = %v", err, tc.expectRunErr)
			}
			if tc.expectRunErr {
				return
			}

			if report.SuccessCount != tc.expectedSuccess {
				t.Errorf("expected SuccessCount %d, got %d", tc.expectedSuccess, report.SuccessCount)
			}
			if report.WarningCount != tc.expectedWarning {
				t.Errorf("expected WarningCount %d, got %d", tc.expectedWarning, report.WarningCount)
			}
			if report.SkippedCount != tc.expectedSkipped {
				t.Errorf("expected SkippedCount %d, got %d", tc.expectedSkipped, report.SkippedCount)
			}
			if report.FailedCount != tc.expectedFailed {
				t.Errorf("expected FailedCount %d, got %d", tc.expectedFailed, report.FailedCount)
			}
			if len(report.Outputs) != tc.expectedOutputs {
				t.Errorf("expected Outputs count %d, got %d", tc.expectedOutputs, len(report.Outputs))
			}

			stderrStr := stderrBuf.String()
			for _, wantLog := range tc.wantStderrLogs {
				if !strings.Contains(stderrStr, wantLog) {
					t.Errorf("expected log containing %q in Stderr, got:\n%s", wantLog, stderrStr)
				}
			}
		})
	}
}

func TestResourceCacheNamespaceScoping(t *testing.T) {
	tests := []struct {
		name        string
		resource    *unstructured.Unstructured
		queryKind   string
		queryNS     string
		queryName   string
		expectFound bool
		expectedNS  string
	}{
		{
			name: "Resource with empty namespace is stored under empty namespace",
			resource: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "monitoring.coreos.com/v1",
					"kind":       "PodMonitor",
					"metadata": map[string]any{
						"name":      "my-monitor-omitted",
						"namespace": "",
					},
				},
			},
			queryKind:   "PodMonitor",
			queryNS:     "",
			queryName:   "my-monitor-omitted",
			expectFound: true,
			expectedNS:  "",
		},
		{
			name: "Strict namespace isolation prevents query match across namespaces",
			resource: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "monitoring.coreos.com/v1",
					"kind":       "PodMonitor",
					"metadata": map[string]any{
						"name":      "common-name",
						"namespace": "namespace-a",
					},
				},
			},
			queryKind:   "PodMonitor",
			queryNS:     "namespace-b",
			queryName:   "common-name",
			expectFound: false,
		},
		{
			name: "Exact namespace query returns stored resource",
			resource: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "monitoring.coreos.com/v1",
					"kind":       "PodMonitor",
					"metadata": map[string]any{
						"name":      "common-name",
						"namespace": "namespace-a",
					},
				},
			},
			queryKind:   "PodMonitor",
			queryNS:     "namespace-a",
			queryName:   "common-name",
			expectFound: true,
			expectedNS:  "namespace-a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewResourceCache()
			if err := cache.Add(tc.resource); err != nil {
				t.Fatalf("Add failed: %v", err)
			}

			res, found := cache.Get(tc.queryKind, tc.queryNS, tc.queryName)
			if found != tc.expectFound {
				t.Fatalf("Get() found = %v, want %v", found, tc.expectFound)
			}
			if tc.expectFound && res.GetNamespace() != tc.expectedNS {
				t.Errorf("expected found resource namespace %q, got %q", tc.expectedNS, res.GetNamespace())
			}
		})
	}
}

func TestMigratorWriteOutputs(t *testing.T) {
	tests := []struct {
		name       string
		outputs    []*unstructured.Unstructured
		wantErr    bool
		wantOutput string
	}{
		{
			name: "Single document output",
			outputs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "PodMonitoring",
						"metadata": map[string]any{
							"name":      "my-pm",
							"namespace": "default",
						},
					},
				},
			},
			wantErr: false,
			wantOutput: `apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: my-pm
  namespace: default
`,
		},
		{
			name: "Multi-document output separated by document boundary",
			outputs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "PodMonitoring",
						"metadata": map[string]any{
							"name":      "pm-1",
							"namespace": "default",
						},
					},
				},
				{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "PodMonitoring",
						"metadata": map[string]any{
							"name":      "pm-2",
							"namespace": "default",
						},
					},
				},
			},
			wantErr: false,
			wantOutput: `apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: pm-1
  namespace: default
---
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: pm-2
  namespace: default
`,
		},
		{
			name: "Nil resource object in outputs returns error",
			outputs: []*unstructured.Unstructured{
				nil,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			migrator := NewMigrator()
			var stdoutBuf bytes.Buffer
			migrator.Stdout = &stdoutBuf

			err := migrator.WriteOutputs(tc.outputs)
			if (err != nil) != tc.wantErr {
				t.Fatalf("WriteOutputs() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && stdoutBuf.String() != tc.wantOutput {
				t.Errorf("WriteOutputs() output mismatch:\nwant:\n%s\ngot:\n%s", tc.wantOutput, stdoutBuf.String())
			}
		})
	}
}
