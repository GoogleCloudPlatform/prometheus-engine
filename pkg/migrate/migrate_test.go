package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DummyConverter implements ResourceConverter for testing.
type DummyConverter struct {
	calls int
}

func (d *DummyConverter) ImportKey() string {
	return "DummyResource"
}

func (d *DummyConverter) Convert(unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, []LogMessage, error) {
	d.calls++

	_, found := cache.Get("Service", unstruct.GetNamespace(), "backing-service")

	logs := []LogMessage{}
	if !found {
		logs = append(logs, LogMessage{
			Level:     LevelWarning,
			Kind:      unstruct.GetKind(),
			Namespace: unstruct.GetNamespace(),
			Name:      unstruct.GetName(),
			Message:   "backing-service not found in cache",
		})
	} else {
		logs = append(logs, LogMessage{
			Level:     LevelInfo,
			Kind:      unstruct.GetKind(),
			Namespace: unstruct.GetNamespace(),
			Name:      unstruct.GetName(),
			Message:   "Successfully resolved backing-service",
		})
	}

	out := &unstructured.Unstructured{}
	out.SetGroupVersionKind(unstruct.GroupVersionKind())
	out.SetKind("TranslatedDummy")
	out.SetName("translated-" + unstruct.GetName())
	out.SetNamespace(unstruct.GetNamespace())

	return []*unstructured.Unstructured{out}, logs, nil
}

func TestMigratorCacheAndExtensibility(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlContent := `
apiVersion: monitoring.coreos.com/v1
kind: DummyResource
metadata:
  name: my-dummy
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
	// Redirect output to buffers to avoid cluttering test logs and verify output
	var stdoutBuf, stderrBuf bytes.Buffer
	migrator.Stdout = &stdoutBuf
	migrator.Stderr = &stderrBuf

	dummyConv := &DummyConverter{}
	migrator.RegisterConverter(dummyConv)

	// Run migration
	report, err := migrator.Run(inputFilePath)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify converter was called
	if dummyConv.calls != 1 {
		t.Errorf("expected DummyConverter to be called 1 time, got %d", dummyConv.calls)
	}

	// Verify report stats (should be success, since backing-service was found)
	if report.SuccessCount != 1 {
		t.Errorf("expected SuccessCount to be 1, got %d", report.SuccessCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("expected WarningCount to be 0, got %d", report.WarningCount)
	}

	// Verify logs contains the INFO log
	foundInfo := false
	for _, log := range report.Logs {
		if log.Level == LevelInfo && strings.Contains(log.Message, "Successfully resolved backing-service") {
			foundInfo = true
		}
	}
	if !foundInfo {
		t.Error("expected INFO log about resolving backing-service was not found in report")
	}
}

func TestResourceCacheNamespaceScoping(t *testing.T) {
	cache := NewResourceCache()

	// 1. Test defaulting of omitted namespace for namespaced resources
	omittedNsRes := &unstructured.Unstructured{}
	omittedNsRes.SetKind("PodMonitor")
	omittedNsRes.SetName("my-monitor-omitted")
	omittedNsRes.SetNamespace("") // Omitted in YAML

	cache.Add(omittedNsRes)

	// It should be stored under "default"
	if _, found := cache.Get("PodMonitor", "default", "my-monitor-omitted"); !found {
		t.Error("expected namespaced resource with omitted namespace to be defaulted to 'default'")
	}
	// It should NOT be found under empty namespace
	if _, found := cache.Get("PodMonitor", "", "my-monitor-omitted"); found {
		t.Error("expected namespaced resource with omitted namespace NOT to be found under empty namespace")
	}

	// 2. Test strict namespace isolation
	nsARes := &unstructured.Unstructured{}
	nsARes.SetKind("PodMonitor")
	nsARes.SetName("common-name")
	nsARes.SetNamespace("namespace-a")

	cache.Add(nsARes)

	// Querying in namespace-b should NOT return the resource from namespace-a
	if _, found := cache.Get("PodMonitor", "namespace-b", "common-name"); found {
		t.Error("expected strict namespace isolation; found resource from namespace-a when querying namespace-b")
	}

	// Querying in namespace-a should find it
	res, found := cache.Get("PodMonitor", "namespace-a", "common-name")
	if !found {
		t.Fatal("expected to find resource in namespace-a")
	}
	if res.GetNamespace() != "namespace-a" {
		t.Errorf("expected found resource to have namespace 'namespace-a', got %q", res.GetNamespace())
	}
}
