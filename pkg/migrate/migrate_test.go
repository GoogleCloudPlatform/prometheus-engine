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

func TestMigratorCacheAndExtensibility(t *testing.T) {
	tmpDir := t.TempDir()

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
	inputFilePath := filepath.Join(tmpDir, "input.yaml")
	if err := os.WriteFile(inputFilePath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	migrator := NewMigrator()
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	testConv := &TestPodMonitorConverter{}
	migrator.RegisterConverter(testConv)

	// Run migration
	report, err := migrator.Run(inputFilePath)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if testConv.calls != 1 {
		t.Errorf("expected TestPodMonitorConverter to be called 1 time, got %d", testConv.calls)
	}

	// Verify report stats
	if report.SuccessCount != 1 {
		t.Errorf("expected SuccessCount to be 1, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}
	if report.SkippedCount != 0 {
		t.Errorf("expected SkippedCount to be 0, got %d", report.SkippedCount)
	}

	stderrLogs := stderrBuf.String()
	if !strings.Contains(stderrLogs, "[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service") {
		t.Errorf("expected formatted INFO log in Stderr, got: %q", stderrLogs)
	}
	if !strings.Contains(stderrLogs, "[SUCCESS] [PodMonitor:default/my-monitor] Converted successfully") {
		t.Errorf("expected formatted SUCCESS log in Stderr, got: %q", stderrLogs)
	}
}

func TestResourceCacheNamespaceScoping(t *testing.T) {
	cache := NewResourceCache()

	omittedNsRes := &unstructured.Unstructured{}
	omittedNsRes.SetKind("PodMonitor")
	omittedNsRes.SetName("my-monitor-omitted")
	omittedNsRes.SetNamespace("")

	if err := cache.Add(omittedNsRes); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if _, found := cache.Get("PodMonitor", "", "my-monitor-omitted"); !found {
		t.Error("expected namespaced resource with omitted namespace to be found under empty namespace")
	}

	nsARes := &unstructured.Unstructured{}
	nsARes.SetKind("PodMonitor")
	nsARes.SetName("common-name")
	nsARes.SetNamespace("namespace-a")

	if err := cache.Add(nsARes); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if _, found := cache.Get("PodMonitor", "namespace-b", "common-name"); found {
		t.Error("expected strict namespace isolation; found resource from namespace-a when querying namespace-b")
	}

	res, found := cache.Get("PodMonitor", "namespace-a", "common-name")
	if !found {
		t.Fatal("expected to find resource in namespace-a")
	}
	if res.GetNamespace() != "namespace-a" {
		t.Errorf("expected found resource to have namespace 'namespace-a', got %q", res.GetNamespace())
	}
}

func TestMigratorMalformedInput(t *testing.T) {
	tmpDir := t.TempDir()

	// YAML resource with a Kind but completely missing metadata.name
	malformedYAML := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: default
spec:
  selector:
    matchLabels:
      app: my-app
`
	inputFilePath := filepath.Join(tmpDir, "bad_resource.yaml")
	if err := os.WriteFile(inputFilePath, []byte(malformedYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	migrator := NewMigrator()
	migrator.RegisterConverter(&TestPodMonitorConverter{})
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	// Run migration on the directory containing the malformed file
	report, err := migrator.Run(tmpDir)
	if err != nil {
		t.Fatalf("Run should not return a fatal error for directory walks, got: %v", err)
	}

	// Verify that the file parse error was caught and counted as a failure.
	if report.FailedCount != 1 {
		t.Errorf("expected FailedCount to be 1, got %d", report.FailedCount)
	}
	if report.SuccessCount != 0 {
		t.Errorf("expected SuccessCount to be 0, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}
	if report.SkippedCount != 0 {
		t.Errorf("expected SkippedCount to be 0, got %d", report.SkippedCount)
	}

	// Verify that a [ERROR] log was printed to Stderr showing the file path and exact parse error
	stderrLogs := stderrBuf.String()
	if !strings.Contains(stderrLogs, "[ERROR] ["+inputFilePath+"] Skipping file due to parse error") {
		t.Errorf("expected formatted [ERROR] log in Stderr, got: %q", stderrLogs)
	}
	if !strings.Contains(stderrLogs, "malformed resource: apiVersion, kind, and metadata.name must all be specified") {
		t.Errorf("expected underlying parse error in Stderr, got: %q", stderrLogs)
	}
}

func TestMigratorSkippedResource(t *testing.T) {
	tmpDir := t.TempDir()

	// Unsupported Prometheus Operator resource kind (Alertmanager)
	skippedYAML := `
apiVersion: monitoring.coreos.com/v1
kind: Alertmanager
metadata:
  name: my-alertmanager
spec:
  replicas: 3
`
	inputFilePath := filepath.Join(tmpDir, "skipped_resource.yaml")
	if err := os.WriteFile(inputFilePath, []byte(skippedYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	migrator := NewMigrator()
	migrator.RegisterConverter(&TestPodMonitorConverter{})
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	// Run migration on the directory containing the skipped file
	report, err := migrator.Run(tmpDir)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify report stats
	if report.SkippedCount != 1 {
		t.Errorf("expected SkippedCount to be 1, got %d", report.SkippedCount)
	}
	if report.SuccessCount != 0 {
		t.Errorf("expected SuccessCount to be 0, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}
	if report.FailedCount != 0 {
		t.Errorf("expected FailedCount to be 0, got %d", report.FailedCount)
	}

	// Verify that a [SKIPPED] log was printed to Stderr showing the resource details
	stderrLogs := stderrBuf.String()
	if !strings.Contains(stderrLogs, "[SKIPPED] [Alertmanager:my-alertmanager] Skipping unsupported Prometheus Operator resource") {
		t.Errorf("expected formatted [SKIPPED] log in Stderr, got: %q", stderrLogs)
	}
}

func TestMigratorMultipleInputs(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Write a Service manifest to a separate file
	serviceYAML := `
apiVersion: v1
kind: Service
metadata:
  name: backing-service
  namespace: default
spec:
  ports:
  - port: 80
`
	servicePath := filepath.Join(tmpDir, "service.yaml")
	if err := os.WriteFile(servicePath, []byte(serviceYAML), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	// 2. Write a PodMonitor manifest referencing that service to a separate file
	podMonitorYAML := `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: my-monitor
  namespace: default
spec:
  foo: bar
`
	podMonitorPath := filepath.Join(tmpDir, "podmonitor.yaml")
	if err := os.WriteFile(podMonitorPath, []byte(podMonitorYAML), 0644); err != nil {
		t.Fatalf("failed to write podmonitor file: %v", err)
	}

	migrator := NewMigrator()
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	testConv := &TestPodMonitorConverter{}
	migrator.RegisterConverter(testConv)

	// Run migration passing both files explicitly!
	report, err := migrator.Run(servicePath, podMonitorPath)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if testConv.calls != 1 {
		t.Errorf("expected TestPodMonitorConverter to be called 1 time, got %d", testConv.calls)
	}

	// Verify report stats
	if report.SuccessCount != 1 {
		t.Errorf("expected SuccessCount to be 1, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}
	if report.SkippedCount != 0 {
		t.Errorf("expected SkippedCount to be 0, got %d", report.SkippedCount)
	}
	if report.FailedCount != 0 {
		t.Errorf("expected FailedCount to be 0, got %d", report.FailedCount)
	}

	// Verify that the reference was successfully resolved across the separate files!
	stderrLogs := stderrBuf.String()
	if !strings.Contains(stderrLogs, "[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service") {
		t.Errorf("expected reference to be successfully resolved, got logs: %q", stderrLogs)
	}
}

func TestMigratorPipedList(t *testing.T) {
	// A standard v1.List containing a Service and a PodMonitor in its items array
	listYAML := `
apiVersion: v1
kind: List
metadata:
  resourceVersion: ""
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
	migrator := NewMigrator()
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	// Pipe the list YAML buffer directly into Stdin!
	migrator.Stdin = strings.NewReader(listYAML)

	testConv := &TestPodMonitorConverter{}
	migrator.RegisterConverter(testConv)

	// Run migration using "-" (Stdin)
	report, err := migrator.Run("-")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if testConv.calls != 1 {
		t.Errorf("expected TestPodMonitorConverter to be called 1 time, got %d", testConv.calls)
	}

	// Verify report stats
	if report.SuccessCount != 1 {
		t.Errorf("expected SuccessCount to be 1, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}
	if report.SkippedCount != 0 {
		t.Errorf("expected SkippedCount to be 0, got %d", report.SkippedCount)
	}
	if report.FailedCount != 0 {
		t.Errorf("expected FailedCount to be 0, got %d", report.FailedCount)
	}

	// Verify that the PodMonitor resolved the Service successfully inside the list!
	stderrLogs := stderrBuf.String()
	if !strings.Contains(stderrLogs, "[INFO] [PodMonitor:default/my-monitor] Successfully resolved backing-service") {
		t.Errorf("expected reference to be successfully resolved inside list, got logs: %q", stderrLogs)
	}
}
