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

package v1

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/manifests"
	"github.com/google/go-cmp/cmp"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
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
		defaulting.PruneNonNullableNullsWithoutDefaults(obj, structural)
		defaulting.Default(obj, structural)

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
		name                    string
		payload                 map[string]any
		wantErrMsg              string
		wantManagedAlertmanager map[string]any
	}{
		{
			name: "valid minimal defaults managedAlertmanager",
			payload: map[string]any{
				"apiVersion": "monitoring.googleapis.com/v1",
				"kind":       "OperatorConfig",
				"metadata": map[string]any{
					"name":      "config",
					"namespace": "gmp-public",
				},
			},
			wantManagedAlertmanager: map[string]any{
				"configSecret": map[string]any{
					"name": "alertmanager",
					"key":  "alertmanager.yaml",
				},
			},
		},
		{
			name: "partial managedAlertmanager defaults configSecret",
			payload: map[string]any{
				"apiVersion": "monitoring.googleapis.com/v1",
				"kind":       "OperatorConfig",
				"metadata": map[string]any{
					"name":      "config",
					"namespace": "gmp-public",
				},
				"managedAlertmanager": map[string]any{
					"externalURL": "https://alertmanager.example.com",
				},
			},
			wantManagedAlertmanager: map[string]any{
				"configSecret": map[string]any{
					"name": "alertmanager",
					"key":  "alertmanager.yaml",
				},
				"externalURL": "https://alertmanager.example.com",
			},
		},
		{
			name: "non-nullable null fields are pruned before validation",
			payload: map[string]any{
				"apiVersion": "monitoring.googleapis.com/v1",
				"kind":       "OperatorConfig",
				"metadata": map[string]any{
					"name":      "config",
					"namespace": "gmp-public",
				},
				"exports": nil,
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
			if tc.wantManagedAlertmanager != nil {
				if diff := cmp.Diff(tc.wantManagedAlertmanager, tc.payload["managedAlertmanager"]); diff != "" {
					t.Errorf("managedAlertmanager diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
