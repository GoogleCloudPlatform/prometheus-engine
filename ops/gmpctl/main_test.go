package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGoMinorVersion(t *testing.T) {
	for _, tt := range []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name: "google-go.pkg.dev image",
			files: map[string]string{
				"Dockerfile": `
FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.2@sha256:1bee769a7a50eea7730ac31f75182ae2614f50a70902407312db390a7c7cb2ff AS buildbase
ARG TARGETOS
`,
			},
			expected: "1.26",
		},
		{
			name: "standard golang image",
			files: map[string]string{
				"Dockerfile": `
FROM golang:1.23.5 AS build
`,
			},
			expected: "1.23",
		},
		{
			name: "skip directories",
			files: map[string]string{
				"third_party/Dockerfile":  "FROM golang:1.20.0",
				"hack/Dockerfile":         "FROM golang:1.20.0",
				"ui/Dockerfile":           "FROM golang:1.20.0",
				"vendor/Dockerfile":       "FROM golang:1.20.0",
				"node_modules/Dockerfile": "FROM golang:1.20.0",
				"Dockerfile":              "FROM golang:1.24.1",
			},
			expected: "1.24",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "gmpctl-test")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
			for path, content := range tt.files {
				fullPath := filepath.Join(tempDir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			version, err := detectGoMinorVersion(tempDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if version != tt.expected {
				t.Errorf("expected version %s, got %s", tt.expected, version)
			}
		})
	}
}

func TestReplaceOtelImports(t *testing.T) {
	for _, tt := range []struct {
		name          string
		files         map[string]string
		targetVersion string
		expected      map[string]string
	}{
		{
			name: "replace import when SchemaURL is used",
			files: map[string]string{
				"tracing.go": `package tracing
import (
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)
func init() {
	_ = semconv.SchemaURL
}
`,
			},
			targetVersion: "v1.40.0",
			expected: map[string]string{
				"tracing.go": `package tracing
import (
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)
func init() {
	_ = semconv.SchemaURL
}
`,
			},
		},
		{
			name: "do not replace import when SchemaURL is NOT used",
			files: map[string]string{
				"queue.go": `package queue
import (
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)
func init() {
	_ = semconv.HTTPResendCount
}
`,
			},
			targetVersion: "v1.40.0",
			expected: map[string]string{
				"queue.go": `package queue
import (
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)
func init() {
	_ = semconv.HTTPResendCount
}
`,
			},
		},
		{
			name: "replace with alias",
			files: map[string]string{
				"tracing.go": `package tracing
import (
	conventions "go.opentelemetry.io/otel/semconv/v1.39.0"
)
func init() {
	_ = conventions.SchemaURL
}
`,
			},
			targetVersion: "v1.40.0",
			expected: map[string]string{
				"tracing.go": `package tracing
import (
	conventions "go.opentelemetry.io/otel/semconv/v1.40.0"
)
func init() {
	_ = conventions.SchemaURL
}
`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "gmpctl-otel-test")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
			for path, content := range tt.files {
				fullPath := filepath.Join(tempDir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := runCommand(&cmdOpts{Dir: tempDir, HideOutputs: true}, "git", "init", "-b", "main"); err != nil {
				t.Fatal(err)
			}

			err = replaceOtelImports(tempDir, tt.targetVersion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for path, expectedContent := range tt.expected {
				fullPath := filepath.Join(tempDir, path)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != expectedContent {
					t.Errorf("file %s: expected content:\n%s\ngot:\n%s", path, expectedContent, string(content))
				}
			}
		})
	}
}
