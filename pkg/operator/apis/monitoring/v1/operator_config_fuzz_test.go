// Copyright 2026 Google LLC
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

//go:build goexperiment.jsonv2

// TODO(bernot): Remove this file when the webhook is removed.

package v1

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/manifests"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/yaml"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

func loadOperatorConfigSchema() (*apiextensionsv1.JSONSchemaProps, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests.CRDManifest), 4096)
	for {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			break
		}
		if crd.Name == "operatorconfigs.monitoring.googleapis.com" {
			if len(crd.Spec.Versions) == 0 {
				return nil, errors.New("no versions found in OperatorConfig CRD")
			}
			return crd.Spec.Versions[0].Schema.OpenAPIV3Schema, nil
		}
	}
	return nil, errors.New("OperatorConfig CRD not found in manifests")
}

func TestInspectNewSchemaValidator(t *testing.T) {
	apiSchema, err := loadOperatorConfigSchema()
	if err != nil {
		t.Fatalf("failed to load OperatorConfig schema: %v", err)
	}

	var internalSchema apiextensions.JSONSchemaProps
	err = apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(apiSchema, &internalSchema, nil)
	if err != nil {
		t.Fatalf("failed to convert schema: %v", err)
	}

	structural, err := structuralschema.NewStructural(&internalSchema)
	if err != nil {
		t.Fatalf("failed to create structural schema: %v", err)
	}

	celValidator := cel.NewValidator(structural, false, celconfig.PerCallLimit)

	invalidPayload := map[string]any{
		"apiVersion": "monitoring.googleapis.com/v1",
		"kind":       "OperatorConfig",
		"metadata": map[string]any{
			"name":      "config",
			"namespace": "gmp-public",
		},
		"rules": map[string]any{
			"alerting": map[string]any{
				"alertmanagers": []any{
					map[string]any{
						"tls": map[string]any{
							"ca": map[string]any{
								"secret": map[string]any{
									"name": "my-secret",
								},
								"configMap": map[string]any{
									"name": "my-configmap",
								},
							},
						},
					},
				},
			},
		},
	}

	errs, _ := celValidator.Validate(t.Context(), nil, structural, invalidPayload, nil, celconfig.RuntimeCELCostBudget)
	t.Logf("CEL Validation Errors: %v (len=%d)", errs, len(errs))
	if len(errs) == 0 {
		t.Error("expected CEL validation to fail, but it passed.")
	}
}

// FuzzOperatorConfig runs differential fuzzing between the OpenAPIv3/CEL validations
// defined in the OperatorConfig CRD and the Go-based Webhook validation in OperatorConfig.Validate().
func FuzzOperatorConfig(f *testing.F) {
	apiSchema, err := loadOperatorConfigSchema()
	if err != nil {
		f.Fatalf("failed to load OperatorConfig schema: %v", err)
	}

	var internalSchema apiextensions.JSONSchemaProps
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(apiSchema, &internalSchema, nil); err != nil {
		f.Fatalf("failed to convert schema: %v", err)
	}

	// Compile the OpenAPI v3 schema validator.
	openapiValidator, _, err := validation.NewSchemaValidator(&internalSchema)
	if err != nil {
		f.Fatalf("failed to create OpenAPI validator: %v", err)
	}

	// Compile the structural and CEL validator.
	structural, err := structuralschema.NewStructural(&internalSchema)
	if err != nil {
		f.Fatalf("failed to create structural schema: %v", err)
	}
	celValidator := cel.NewValidator(structural, false, celconfig.PerCallLimit)

	// Add seed corpus 1: A minimal valid OperatorConfig (no optional fields).
	minimalSeed := map[string]any{
		"apiVersion": "monitoring.googleapis.com/v1",
		"kind":       "OperatorConfig",
		"metadata": map[string]any{
			"name":      "config",
			"namespace": "gmp-public",
		},
	}
	minimalSeedBytes, _ := json.Marshal(minimalSeed)
	f.Add(minimalSeedBytes)

	// Add seed corpus 2: A fully populated valid OperatorConfig covering every possible field and nested subfield.
	fullyPopulatedSeed := map[string]any{
		"apiVersion": "monitoring.googleapis.com/v1",
		"kind":       "OperatorConfig",
		"metadata": map[string]any{
			"name":      "config",
			"namespace": "gmp-public",
		},
		"rules": map[string]any{
			"queryProjectID": "my-gcp-project",
			"generatorUrl":   "https://prometheus.example.com",
			"externalLabels": map[string]any{
				"label_key": "label_val",
			},
			"credentials": map[string]any{
				"name": "rules-credentials",
				"key":  "key.json",
			},
			"alerting": map[string]any{
				"alertmanagers": []any{
					map[string]any{
						"namespace":  "alertmanager-namespace",
						"name":       "alertmanager-name",
						"port":       9093,
						"scheme":     "https",
						"pathPrefix": "/api/v1",
						"timeout":    "10s",
						"apiVersion": "v2",
						"authorization": map[string]any{
							"type": "Bearer",
							"credentials": map[string]any{
								"name": "auth-token-secret",
								"key":  "token",
							},
						},
						"tls": map[string]any{
							"ca": map[string]any{
								"secret": map[string]any{
									"name": "ca-secret",
									"key":  "ca.crt",
								},
							},
							"cert": map[string]any{
								"secret": map[string]any{
									"name": "cert-secret",
									"key":  "tls.crt",
								},
							},
							"keySecret": map[string]any{
								"name": "key-secret",
								"key":  "tls.key",
							},
							"serverName":         "alertmanager.example.com",
							"insecureSkipVerify": false,
						},
					},
				},
			},
		},
		"collection": map[string]any{
			"externalLabels": map[string]any{
				"collection_label_key": "collection_label_val",
			},
			"filter": map[string]any{
				"matchOneOf": []any{
					`{__name__=~"job:.*"}`,
				},
				"enableMatchOneOf": true,
			},
			"credentials": map[string]any{
				"name": "collection-credentials",
				"key":  "key.json",
			},
			"kubeletScraping": map[string]any{
				"interval": "30s",
			},
			"compression": "gzip",
		},
		"exports": []any{
			map[string]any{
				"url": "https://remote-write-endpoint.example.com",
			},
		},
		"managedAlertmanager": map[string]any{
			"configSecret": map[string]any{
				"name": "alertmanager",
				"key":  "alertmanager.yaml",
			},
			"externalURL": "https://alertmanager-external.example.com",
		},
		"features": map[string]any{
			"targetStatus": map[string]any{
				"enabled": true,
			},
		},
		"scaling": map[string]any{
			"vpa": map[string]any{
				"enabled": true,
			},
		},
	}
	seedBytes, _ := json.Marshal(fullyPopulatedSeed)
	f.Add(seedBytes)

	f.Fuzz(func(t *testing.T, data []byte) {
		// 1. Structural check: Must unmarshal strictly into structured OperatorConfig (case-sensitive + no unknown fields).
		var oc OperatorConfig
		if err := json.Unmarshal(data, &oc, json.RejectUnknownMembers(true)); err != nil {
			// Skip inputs that do not strictly match the OperatorConfig schema.
			t.Skip()
		}

		// 2. Must unmarshal into unstructured map for schema/CEL validators.
		var unstructuredObj map[string]any
		if err := json.Unmarshal(data, &unstructuredObj); err != nil {
			t.Skip()
		}

		// 3. Execute OpenAPIv3 Schema Validation.
		openapiResult := openapiValidator.Validate(unstructuredObj)
		if openapiResult.HasErrors() {
			// If the object is structurally invalid according to the OpenAPI schema,
			// the API server would reject it before it ever reaches the validating webhook or CEL rules.
			// Therefore, we skip differential validation for this input.
			t.Skip()
		}

		// 4. Execute CEL Validation.
		celErrors, _ := celValidator.Validate(t.Context(), nil, structural, unstructuredObj, nil, celconfig.RuntimeCELCostBudget)
		celPassed := len(celErrors) == 0

		// 5. Execute Webhook Validation.
		webhookErr := oc.Validate()
		webhookPassed := webhookErr == nil

		// 6. Differential assertion.
		if celPassed != webhookPassed {
			if !celPassed && webhookPassed {
				if isURLValidationDiscrepancy(celErrors) {
					t.Skip("Narrowly skipping: CEL is stricter than Webhook for URL validation")
				}
				t.Fatalf("Discrepancy (False Positive): CEL validation rejected the object, but Webhook accepted it.\nCEL Errors: %v\nPayload: %s", celErrors, string(data))
			}
			if celPassed && !webhookPassed {
				// This is a False Negative in CEL (CEL is too lenient / missing rules).
				// We narrowly tolerate this if the webhook rejected it specifically because of generatorUrl parsing
				// or duration string parsing (see https://github.com/kubernetes/kube-openapi/pull/619).
				if strings.Contains(webhookErr.Error(), "failed to parse generator URL") {
					t.Skip("Narrowly skipping: Webhook is stricter than CEL for generatorUrl validation")
				}
				if isDurationValidationDiscrepancy(webhookErr) {
					t.Skip("Narrowly skipping: Webhook is stricter than CEL for duration string validation")
				}
				t.Fatalf("Discrepancy (False Negative): CEL validation accepted the object, but Webhook rejected it.\nWebhook Error: %v\nPayload: %s", webhookErr, string(data))
			}
		}
	})
}

func isDurationValidationDiscrepancy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not a valid duration string") ||
		strings.Contains(msg, "empty duration string") ||
		strings.Contains(msg, "unknown unit") ||
		strings.Contains(msg, "duration out of range")
}

func isURLValidationDiscrepancy(errs field.ErrorList) bool {
	if len(errs) == 0 {
		return false
	}
	for _, err := range errs {
		f := err.Field
		isURLField := f == "rules.generatorUrl" || f == "managedAlertmanager.externalURL" || (strings.HasPrefix(f, "exports[") && strings.HasSuffix(f, "].url"))
		if !isURLField {
			return false
		}
	}
	return true
}
