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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/manifests"
	"github.com/google/go-cmp/cmp"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigyaml "sigs.k8s.io/yaml"
)

var updateFlag = flag.Bool("update", false, "Update golden baselines in testdata/.")

type gmpSchemaValidators struct {
	podMonitoring        validation.SchemaValidator
	clusterPodMonitoring validation.SchemaValidator
}

func loadGMPSchemaValidators() (*gmpSchemaValidators, error) {
	validators := &gmpSchemaValidators{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests.CRDManifest), 4096)

	for {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			break
		}

		if len(crd.Spec.Versions) == 0 || crd.Spec.Versions[0].Schema == nil || crd.Spec.Versions[0].Schema.OpenAPIV3Schema == nil {
			continue
		}

		apiSchema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
		var internalSchema apiextensions.JSONSchemaProps
		if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(apiSchema, &internalSchema, nil); err != nil {
			return nil, fmt.Errorf("failed to convert schema for %s: %w", crd.Name, err)
		}

		validator, _, err := validation.NewSchemaValidator(&internalSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to create validator for %s: %w", crd.Name, err)
		}

		switch crd.Name {
		case "podmonitorings.monitoring.googleapis.com":
			validators.podMonitoring = validator
		case "clusterpodmonitorings.monitoring.googleapis.com":
			validators.clusterPodMonitoring = validator
		}
	}

	if validators.podMonitoring == nil {
		return nil, errors.New("podmonitorings.monitoring.googleapis.com schema not found in manifests")
	}
	if validators.clusterPodMonitoring == nil {
		return nil, errors.New("clusterpodmonitorings.monitoring.googleapis.com schema not found in manifests")
	}

	return validators, nil
}

func validateResourceAgainstSchema(t *testing.T, validators *gmpSchemaValidators, u *unstructured.Unstructured) {
	t.Helper()

	var validator validation.SchemaValidator
	switch u.GetKind() {
	case KindPodMonitoring:
		validator = validators.podMonitoring
	case KindClusterPodMonitoring:
		validator = validators.clusterPodMonitoring
	default:
		return
	}

	res := validator.Validate(u.Object)
	if res.HasErrors() {
		var errStrs []string
		for _, e := range res.Errors {
			errStrs = append(errStrs, e.Error())
		}
		t.Errorf("Generated resource %s/%s failed OpenAPI schema validation:\n%s", u.GetKind(), u.GetName(), strings.Join(errStrs, "\n"))
	}
}

func TestPrometheusOperatorOpenAPIGolden(t *testing.T) {
	validators, err := loadGMPSchemaValidators()
	if err != nil {
		t.Fatalf("failed to load GMP schema validators: %v", err)
	}

	goldenDir := filepath.Join("testdata", "golden")

	for _, tc := range prometheusOperatorPodMonitorTestCases {
		testName := tc.name
		t.Run(testName, func(t *testing.T) {
			testDir := filepath.Join(goldenDir, testName)
			inputPath := filepath.Join(testDir, "input_tmp.yaml")
			expectedOutputPath := filepath.Join(testDir, "expected_output.yaml")
			expectedStderrPath := filepath.Join(testDir, "expected_stderr.log")

			if err := os.MkdirAll(testDir, 0755); err != nil {
				t.Fatalf("failed to create test directory %s: %v", testDir, err)
			}

			tc.input.TypeMeta = metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: KindPodMonitor}
			inputBytes, err := sigyaml.Marshal(tc.input)
			if err != nil {
				t.Fatalf("failed to marshal input struct: %v", err)
			}
			if err := os.WriteFile(inputPath, inputBytes, 0644); err != nil {
				t.Fatalf("failed to write temporary input file: %v", err)
			}
			defer os.Remove(inputPath)

			migrator := NewMigrator()
			var logBuf bytes.Buffer
			migrator.Stderr = &logBuf
			migrator.RegisterConverter(&PodMonitorConverter{})

			report, err := migrator.Run(inputPath)
			if err != nil {
				t.Fatalf("migrator Run failed: %v", err)
			}

			var yamlDocs []string
			for _, output := range report.Outputs {
				validateResourceAgainstSchema(t, validators, output)
				outBytes, err := sigyaml.Marshal(output.Object)
				if err != nil {
					t.Fatalf("failed to marshal output resource: %v", err)
				}
				yamlDocs = append(yamlDocs, strings.TrimSpace(string(outBytes)))
			}

			actualYAML := strings.Join(yamlDocs, "\n---\n") + "\n"
			actualStderr := logBuf.String()

			if *updateFlag {
				if err := os.WriteFile(expectedOutputPath, []byte(actualYAML), 0644); err != nil {
					t.Fatalf("failed to update golden output file: %v", err)
				}
				if err := os.WriteFile(expectedStderrPath, []byte(actualStderr), 0644); err != nil {
					t.Fatalf("failed to update golden stderr file: %v", err)
				}
				t.Logf("updated golden files in %s", testDir)
				return
			}

			expectedBytes, err := os.ReadFile(expectedOutputPath)
			if err != nil {
				t.Fatalf("failed to read golden output file %s (run with -update to generate): %v", expectedOutputPath, err)
			}
			if diff := cmp.Diff(string(expectedBytes), actualYAML); diff != "" {
				t.Errorf("golden output mismatch for %s (-want +got):\n%s", testName, diff)
			}

			expectedStderrBytes, err := os.ReadFile(expectedStderrPath)
			if err != nil {
				t.Fatalf("failed to read golden stderr file %s (run with -update to generate): %v", expectedStderrPath, err)
			}
			if diff := cmp.Diff(string(expectedStderrBytes), actualStderr); diff != "" {
				t.Errorf("golden stderr mismatch for %s (-want +got):\n%s", testName, diff)
			}
		})
	}
}
