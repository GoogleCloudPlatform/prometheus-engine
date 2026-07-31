// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// gen-jsonschemas extracts openAPIV3Schema from CRD manifests and writes JSON
// schema files for kubeconform and similar validators.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gen-jsonschemas: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	crdDir := filepath.Join("charts", "operator", "crds")
	outDir := "schemas"
	if len(args) > 0 {
		crdDir = args[0]
	}
	if len(args) > 1 {
		outDir = args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	entries, err := os.ReadDir(crdDir)
	if err != nil {
		return fmt.Errorf("read crd dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(crdDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &crd); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		groupShort := strings.SplitN(crd.Spec.Group, ".", 2)[0]
		singular := crd.Spec.Names.Singular

		for _, version := range crd.Spec.Versions {
			if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
				continue
			}

			name := fmt.Sprintf("%s-%s-%s.json", singular, groupShort, version.Name)
			outPath := filepath.Join(outDir, name)

			encoded, err := json.MarshalIndent(version.Schema.OpenAPIV3Schema, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal schema for %s: %w", name, err)
			}
			encoded = append(encoded, '\n')

			if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}
		}
	}

	return nil
}
