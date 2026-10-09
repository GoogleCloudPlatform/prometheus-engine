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

	openapiValidator, _, err := validation.NewSchemaValidator(&internalSchema)
	if err != nil {
		t.Fatalf("failed to create OpenAPI validator: %v", err)
	}

	structural, err := structuralschema.NewStructural(&internalSchema)
	if err != nil {
		t.Fatalf("failed to create structural schema: %v", err)
	}

	celValidator := cel.NewValidator(structural, false, celconfig.PerCallLimit)

	validateCRD := func(obj map[string]any) []string {
		var errs []string
		openapiResult := openapiValidator.Validate(obj)
		for _, e := range openapiResult.Errors {
			errs = append(errs, e.Error())
		}
		celErrors, _ := celValidator.Validate(t.Context(), nil, structural, obj, nil, celconfig.RuntimeCELCostBudget)
		for _, e := range celErrors {
			errs = append(errs, e.Error())
		}
		return errs
	}

	tests := []struct {
		name       string
		payload    map[string]any
		wantErrMsg string
	}{
		{
			name: "valid minimal",
			payload: map[string]any{
				"apiVersion": "monitoring.googleapis.com/v1",
				"kind":       "OperatorConfig",
				"metadata": map[string]any{
					"name":      "config",
					"namespace": "gmp-public",
				},
			},
		},
		{
			name: "TLS CA secret and configMap mutually exclusive",
			payload: map[string]any{
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
								"name":      "am",
								"namespace": "gmp-public",
								"port":      9093,
								"tls": map[string]any{
									"ca": map[string]any{
										"secret": map[string]any{
											"name": "my-secret",
											"key":  "ca.crt",
										},
										"configMap": map[string]any{
											"name": "my-configmap",
											"key":  "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErrMsg: "SecretOrConfigMap fields are mutually exclusive",
		},
		{
			name: "invalid collection compression enum",
			payload: map[string]any{
				"collection": map[string]any{
					"compression": "snappy",
				},
			},
			wantErrMsg: "collection.compression in body should be one of [none gzip]",
		},
		{
			name: "invalid features config compression enum",
			payload: map[string]any{
				"features": map[string]any{
					"config": map[string]any{
						"compression": "snappy",
					},
				},
			},
			wantErrMsg: "features.config.compression in body should be one of [none gzip]",
		},
		{
			name: "invalid kubeletScraping interval duration",
			payload: map[string]any{
				"collection": map[string]any{
					"kubeletScraping": map[string]any{
						"interval": "xyz",
					},
				},
			},
			wantErrMsg: "collection.kubeletScraping.interval in body must be of type duration",
		},
		{
			name: "omitted kubeletScraping interval required",
			payload: map[string]any{
				"collection": map[string]any{
					"kubeletScraping": map[string]any{},
				},
			},
			wantErrMsg: "collection.kubeletScraping.interval in body is required",
		},
		{
			name: "invalid alertmanager timeout duration",
			payload: map[string]any{
				"rules": map[string]any{
					"alerting": map[string]any{
						"alertmanagers": []any{
							map[string]any{
								"name":      "am",
								"namespace": "gmp-public",
								"port":      9093,
								"timeout":   "xyz",
							},
						},
					},
				},
			},
			wantErrMsg: "rules.alerting.alertmanagers[0].timeout in body must be of type duration",
		},
		{
			name: "omitted alertmanager required fields",
			payload: map[string]any{
				"rules": map[string]any{
					"alerting": map[string]any{
						"alertmanagers": []any{
							map[string]any{
								"name": "am",
							},
						},
					},
				},
			},
			wantErrMsg: "namespace in body is required",
		},
		{
			name: "omitted secret key required field",
			payload: map[string]any{
				"collection": map[string]any{
					"credentials": map[string]any{
						"name": "my-secret",
					},
				},
			},
			wantErrMsg: "collection.credentials.key in body is required",
		},
		{
			name: "invalid exports url",
			payload: map[string]any{
				"exports": []any{
					map[string]any{
						"url": "http://:::",
					},
				},
			},
			wantErrMsg: "url must be a valid URL",
		},
		{
			name: "invalid managedAlertmanager externalURL",
			payload: map[string]any{
				"managedAlertmanager": map[string]any{
					"externalURL": "http://:::",
				},
			},
			wantErrMsg: "externalURL must be a valid URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateCRD(tc.payload)
			joined := strings.Join(errs, "; ")
			if tc.wantErrMsg == "" && len(errs) > 0 {
				t.Errorf("expected no errors, got: %s", joined)
			}
			if tc.wantErrMsg != "" && !strings.Contains(joined, tc.wantErrMsg) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErrMsg, joined)
			}
		})
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

		// 3. Execute OpenAPIv3 Schema Validation and CEL Validation.
		openapiResult := openapiValidator.Validate(unstructuredObj)
		celErrors, _ := celValidator.Validate(t.Context(), nil, structural, unstructuredObj, nil, celconfig.RuntimeCELCostBudget)
		crdPassed := !openapiResult.HasErrors() && len(celErrors) == 0

		// 4. Execute Webhook Validation.
		webhookErr := oc.Validate()
		webhookPassed := webhookErr == nil

		// 5. Differential assertion.
		if crdPassed != webhookPassed {
			if !crdPassed && webhookPassed {
				if isExpectedCRDDiscrepancy(openapiResult.Errors, celErrors) {
					t.Skip("Narrowly skipping: CRD schema/CEL is intentionally stricter than Webhook")
				}
				t.Fatalf("Discrepancy (False Positive): CRD validation rejected the object, but Webhook accepted it.\nOpenAPI Errors: %v\nCEL Errors: %v\nPayload: %s", openapiResult.Errors, celErrors, string(data))
			}
			if crdPassed && !webhookPassed {
				// This is a False Negative in CRD/CEL (CRD/CEL is too lenient / missing rules).
				// We narrowly tolerate this if the webhook rejected it specifically because of generatorUrl parsing
				// or duration string parsing (see https://github.com/kubernetes/kube-openapi/pull/619).
				if strings.Contains(webhookErr.Error(), "failed to parse generator URL") {
					t.Skip("Narrowly skipping: Webhook is stricter than CEL for generatorUrl validation")
				}
				if isDurationValidationDiscrepancy(webhookErr) {
					t.Skip("Narrowly skipping: Webhook is stricter than CEL for duration string validation")
				}
				t.Fatalf("Discrepancy (False Negative): CRD validation accepted the object, but Webhook rejected it.\nWebhook Error: %v\nPayload: %s", webhookErr, string(data))
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

func isExpectedCRDDiscrepancy(openapiErrs []error, celErrs field.ErrorList) bool {
	if len(openapiErrs) == 0 && len(celErrs) == 0 {
		return false
	}
	for _, err := range openapiErrs {
		msg := err.Error()
		isExpected := strings.HasSuffix(msg, "is required") ||
			strings.Contains(msg, "should be one of [none gzip]") ||
			strings.Contains(msg, "must be of type uri") ||
			strings.Contains(msg, ".timeout in body must be of type duration") ||
			strings.Contains(msg, "collection.kubeletScraping.interval in body must be of type duration") ||
			strings.HasPrefix(msg, "metadata.")
		if !isExpected {
			return false
		}
	}
	for _, err := range celErrs {
		f := err.Field
		isURLField := f == "rules.generatorUrl" || f == "managedAlertmanager.externalURL" || (strings.HasPrefix(f, "exports[") && strings.HasSuffix(f, "].url"))
		if !isURLField {
			return false
		}
	}
	return true
}
