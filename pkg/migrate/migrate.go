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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"maps"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// MigrationReport accumulates the results and statistics of the migration run.
type MigrationReport struct {
	Logs         []LogMessage
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
}

// NewMigrator creates a new Migrator.
func NewMigrator() *Migrator {
	return &Migrator{
		converters: make(map[string]ResourceConverter),
		cache:      NewResourceCache(),
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}
}

// RegisterConverter registers a converter for a specific resource Kind.
func (m *Migrator) RegisterConverter(c ResourceConverter) {
	m.converters[c.ImportKey()] = c
}

// Run executes the migration flow and returns the summary report.
func (m *Migrator) Run(inputPath string) (*MigrationReport, error) {
	report := &MigrationReport{}

	// 1. Parse all inputs and populate the cache
	if err := m.parseInputs(inputPath, report); err != nil {
		return nil, fmt.Errorf("failed to parse inputs: %w", err)
	}

	// 2. Run converters on cached resources
	outputs, err := m.convertResources(report)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resources: %w", err)
	}

	// 3. Write outputs (always to Stdout)
	if err := m.writeOutputs(outputs); err != nil {
		return nil, fmt.Errorf("failed to write outputs: %w", err)
	}

	// 4. Print Summary Stats
	m.printSummary(report)

	return report, nil
}

// parseInputs reads files, directories, or stdin and loads them into the cache.
func (m *Migrator) parseInputs(path string, report *MigrationReport) error {
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
		if strings.HasSuffix(fp, ".yaml") || strings.HasSuffix(fp, ".yml") {
			if err := m.parseFile(fp); err != nil {
				log := LogMessage{
					Level:   LevelError,
					Name:    fp, // Use file path as name since we don't have Kind/Namespace yet
					Message: fmt.Sprintf("Skipping file due to parse error: %v", err),
				}
				report.Logs = append(report.Logs, log)
				fmt.Fprintln(m.Stderr, log.String())
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
		if u.Object == nil || u.GetKind() == "" {
			continue
		}
		m.cache.Add(&u)
	}
	return nil
}

// convertResources runs registered converters and populates the MigrationReport.
func (m *Migrator) convertResources(report *MigrationReport) ([]*unstructured.Unstructured, error) {
	var allOutputs []*unstructured.Unstructured

	// Sort Kinds alphabetically for deterministic execution order
	kinds := slices.AppendSeq(make([]string, 0, len(m.cache.resources)), maps.Keys(m.cache.resources))
	slices.Sort(kinds)

	for _, kind := range kinds {
		converter, registered := m.converters[kind]
		if !registered {
			continue
		}

		nsMap := m.cache.resources[kind]
		// Sort Namespace/Name keys alphabetically for determinism
		keys := slices.AppendSeq(make([]string, 0, len(nsMap)), maps.Keys(nsMap))
		slices.Sort(keys)

		for _, key := range keys {
			res := nsMap[key]
			outputs, logs, err := converter.Convert(res, m.cache)
			report.Logs = append(report.Logs, logs...)

			// Emitting operational logs (INFO, WARNING, SUCCESS)
			for _, log := range logs {
				fmt.Fprintln(m.Stderr, log.String())
			}

			// If conversion fails, increment fail count
			if err != nil {
				report.FailedCount++
				errLog := LogMessage{
					Level:     LevelError,
					Kind:      kind,
					Namespace: res.GetNamespace(),
					Name:      res.GetName(),
					Message:   err.Error(),
				}
				report.Logs = append(report.Logs, errLog)
				fmt.Fprintln(m.Stderr, errLog.String())
				continue
			}

			allOutputs = append(allOutputs, outputs...)

			if len(outputs) > 0 {
				// Distinguish migrated with warnings vs complete success
				hasWarnings := false
				for _, l := range logs {
					if l.Level == LevelWarning {
						hasWarnings = true
						break
					}
				}

				if hasWarnings {
					report.WarningCount++
				} else {
					report.SuccessCount++
				}

				successLog := LogMessage{
					Level:     LevelSuccess,
					Kind:      kind,
					Namespace: res.GetNamespace(),
					Name:      res.GetName(),
					Message:   "Translated successfully",
				}
				report.Logs = append(report.Logs, successLog)
				fmt.Fprintln(m.Stderr, successLog.String())
			}
		}
	}
	return allOutputs, nil
}

// writeOutputs writes the translated resources to Stdout separated by ---.
func (m *Migrator) writeOutputs(outputs []*unstructured.Unstructured) error {
	for i, out := range outputs {
		yamlOut, err := yaml.Marshal(out.Object)
		if err != nil {
			return err
		}
		if i > 0 {
			fmt.Fprintln(m.Stdout, "---")
		}
		fmt.Fprint(m.Stdout, string(yamlOut))
	}
	return nil
}

// printSummary outputs the final execution statistics.
func (m *Migrator) printSummary(r *MigrationReport) {
	// Summary goes to Stderr so it doesn't pollute the redirected YAML payload
	fmt.Fprintln(m.Stderr, "\n=========================================")
	fmt.Fprintln(m.Stderr, "Migration Complete Summary:")
	fmt.Fprintf(m.Stderr, "  Successfully Migrated: %d\n", r.SuccessCount)
	fmt.Fprintf(m.Stderr, "  Migrated with Warnings: %d\n", r.WarningCount)
	fmt.Fprintf(m.Stderr, "  Failed (Skipped):        %d\n", r.FailedCount)
	fmt.Fprintln(m.Stderr, "=========================================")
}
