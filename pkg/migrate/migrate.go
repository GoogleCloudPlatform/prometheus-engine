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
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"maps"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// MigrationReport accumulates the statistics of the migration run.
type MigrationReport struct {
	SuccessCount int // Successfully migrated with no warnings
	WarningCount int // Successfully migrated but had warnings
	FailedCount  int // Fatal failure, resource skipped
}

// Migrator orchestrates the migration process.
type Migrator struct {
	converters map[string]ResourceConverter
	cache      *ResourceCache

	// Decoupled streams (defaults to os.Stdin/os.Stdout/os.Stderr)
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	logger *slog.Logger
}

// NewMigrator creates a new Migrator.
func NewMigrator() *Migrator {
	return &Migrator{
		converters: make(map[string]ResourceConverter),
		cache:      NewResourceCache(),
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		logger:     slog.Default(),
	}
}

// RegisterConverter registers a converter for a specific resource Kind.
func (m *Migrator) RegisterConverter(c ResourceConverter) {
	m.converters[c.ImportKey()] = c
}

// Run executes the migration flow and returns the summary report.
func (m *Migrator) Run(inputPath string) (*MigrationReport, error) {
	if m.Stdin == nil {
		m.Stdin = os.Stdin
	}
	if m.Stdout == nil {
		m.Stdout = os.Stdout
	}
	if m.Stderr == nil {
		m.Stderr = os.Stderr
	}

	report := &MigrationReport{}

	// Instantiate our custom ConsoleHandler
	handler := NewConsoleHandler(m.Stderr)
	m.logger = slog.New(handler)

	// 1. Parse all inputs
	if err := m.parseInputs(inputPath); err != nil {
		return nil, fmt.Errorf("failed to parse inputs: %w", err)
	}

	// 2. Run converters
	outputs := m.convertResources()

	// 3. Write GMP manifests
	if err := m.writeOutputs(outputs); err != nil {
		return nil, fmt.Errorf("failed to write outputs: %w", err)
	}

	// 4. Calculate final statistics from the handler's tracked levels
	for _, level := range handler.ResourceLevels() {
		switch level {
		case slog.LevelError:
			report.FailedCount++
		case slog.LevelWarn:
			report.WarningCount++
		default:
			// Info or Success levels are counted as perfect successes
			report.SuccessCount++
		}
	}

	// 5. Print Summary Stats
	m.printSummary(report)

	return report, nil
}

// isRelevantKind returns true if the given resource Kind is either a target
// resource with a registered converter, or a known cached dependency.
func (m *Migrator) isRelevantKind(kind string) bool {
	switch kind {
	case "Service", "ConfigMap", "Secret":
		return true
	}
	_, registered := m.converters[kind]
	return registered
}

// parseInputs reads files, directories, or stdin and loads them into the cache.
func (m *Migrator) parseInputs(path string) error {
	if path == "-" {
		return m.parseYAMLStream(m.Stdin)
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return m.parseFile(path)
	}

	return filepath.WalkDir(path, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fp))
		if ext == ".yaml" || ext == ".yml" {
			if err := m.parseFile(fp); err != nil {
				// A file failed to parse completely. Count as a failed run step.
				m.logger.Error("Skipping file due to parse error",
					slog.String("file", fp),
					slog.Any("error", err),
				)
			}
		}
		return nil
	})
}

func (m *Migrator) parseFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return m.parseYAMLStream(f)
}

func (m *Migrator) parseYAMLStream(r io.Reader) error {
	decoder := k8syaml.NewYAMLOrJSONDecoder(bufio.NewReader(r), 4096)
	for {
		var u unstructured.Unstructured
		if err := decoder.Decode(&u); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		apiVersion := u.GetAPIVersion()
		kind := u.GetKind()
		name := u.GetName()

		// 1. If it's not a resource we care about, skip it.
		if !m.isRelevantKind(kind) {
			// If it's a Prometheus Operator resource, log an ERROR first.
			if strings.HasPrefix(apiVersion, "monitoring.coreos.com") {
				m.logger.Error("Skipping unsupported Prometheus Operator resource",
					slog.String("apiVersion", apiVersion),
					slog.String("kind", kind),
					slog.String("namespace", u.GetNamespace()),
					slog.String("name", name),
				)
			}
			continue
		}

		// 2. Since this IS a resource we care about, it must be well-formed.
		if apiVersion == "" || kind == "" || name == "" {
			return fmt.Errorf("malformed resource: apiVersion, kind, and metadata.name must all be specified (got apiVersion=%q, kind=%q, name=%q)", apiVersion, kind, name)
		}

		if err := m.cache.Add(&u); err != nil {
			return fmt.Errorf("failed to cache resource: %w", err)
		}
	}
	return nil
}

func (m *Migrator) convertResources() []*unstructured.Unstructured {
	var allOutputs []*unstructured.Unstructured
	ctx := context.Background()

	kinds := slices.AppendSeq(make([]string, 0, len(m.cache.resources)), maps.Keys(m.cache.resources))
	slices.Sort(kinds)

	for _, kind := range kinds {
		nsMap := m.cache.resources[kind]
		converter, registered := m.converters[kind]
		if !registered {
			continue
		}

		keys := slices.AppendSeq(make([]string, 0, len(nsMap)), maps.Keys(nsMap))
		slices.Sort(keys)

		for _, key := range keys {
			res := nsMap[key].DeepCopy()

			// Create the pre-scoped resource logger
			resourceLogger := m.logger.With(
				slog.String("kind", kind),
				slog.String("namespace", res.GetNamespace()),
				slog.String("name", res.GetName()),
			)

			outputs, err := converter.Convert(ctx, resourceLogger, res, m.cache)

			if err != nil {
				resourceLogger.Error(err.Error())
				continue
			}

			allOutputs = append(allOutputs, outputs...)

			resourceLogger.Log(ctx, LevelSuccess, "Converted successfully")
		}
	}
	return allOutputs
}

func (m *Migrator) writeOutputs(outputs []*unstructured.Unstructured) error {
	for i, out := range outputs {
		if out == nil || out.Object == nil {
			return fmt.Errorf("internal error: found nil resource or uninitialized object in outputs at index %d", i)
		}

		yamlOut, err := yaml.Marshal(out)
		if err != nil {
			return err
		}
		if i > 0 {
			if _, err := fmt.Fprintln(m.Stdout, "---"); err != nil {
				return fmt.Errorf("failed to write document separator: %w", err)
			}
		}
		if _, err := m.Stdout.Write(yamlOut); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}
	return nil
}

func (m *Migrator) printSummary(r *MigrationReport) {
	fmt.Fprintln(m.Stderr, "\n=========================================")
	fmt.Fprintln(m.Stderr, "Migration Complete Summary:")
	fmt.Fprintf(m.Stderr, "  Successfully Migrated: %d\n", r.SuccessCount)
	fmt.Fprintf(m.Stderr, "  Migrated with Warnings: %d\n", r.WarningCount)
	fmt.Fprintf(m.Stderr, "  Failed (Skipped):        %d\n", r.FailedCount)
	fmt.Fprintln(m.Stderr, "=========================================")
}
