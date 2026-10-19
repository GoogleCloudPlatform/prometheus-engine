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

// TODO(bernot): Remove the webhook parity checks and differential fuzzers in this file when the *Rules webhooks are removed.

package v1

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GoogleCloudPlatform/prometheus-engine/manifests"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	celmodel "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/yaml"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/cel/environment"
)

func loadCRDByName(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests.CRDManifest), 4096)
	for {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			break
		}
		if crd.Name == name {
			return &crd, nil
		}
	}
	return nil, fmt.Errorf("CRD %q not found in manifests", name)
}

func loadCRDSchemaByName(name string) (*apiextensionsv1.JSONSchemaProps, error) {
	crd, err := loadCRDByName(name)
	if err != nil {
		return nil, err
	}
	for _, v := range crd.Spec.Versions {
		if v.Name == "v1" && v.Schema != nil && v.Schema.OpenAPIV3Schema != nil {
			return v.Schema.OpenAPIV3Schema, nil
		}
	}
	return nil, fmt.Errorf("v1 schema not found in CRD %q", name)
}

func compileCRDValidators(tb testing.TB, crdName string) (validation.SchemaValidator, *structuralschema.Structural, *cel.Validator) {
	tb.Helper()
	apiSchema, err := loadCRDSchemaByName(crdName)
	if err != nil {
		tb.Fatalf("failed to load %s schema: %v", crdName, err)
	}

	var internalSchema apiextensions.JSONSchemaProps
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(apiSchema, &internalSchema, nil); err != nil {
		tb.Fatalf("failed to convert %s schema: %v", crdName, err)
	}

	openapiValidator, _, err := validation.NewSchemaValidator(&internalSchema)
	if err != nil {
		tb.Fatalf("failed to create OpenAPI validator for %s: %v", crdName, err)
	}

	structural, err := structuralschema.NewStructural(&internalSchema)
	if err != nil {
		tb.Fatalf("failed to create structural schema for %s: %v", crdName, err)
	}

	celValidator := cel.NewValidator(structural, true, celconfig.PerCallLimit)
	if celValidator == nil {
		tb.Fatalf("expected non-nil CEL validator for %s", crdName)
	}
	return openapiValidator, structural, celValidator
}

const (
	staticEstimatedCostLimit    uint64 = 10_000_000
	staticEstimatedCRDCostLimit uint64 = 100_000_000
)

func hasXValidations(s *structuralschema.Structural) bool {
	if s == nil {
		return false
	}
	if len(s.XValidations) > 0 {
		return true
	}
	if hasXValidations(s.Items) {
		return true
	}
	for _, prop := range s.Properties {
		if hasXValidations(&prop) {
			return true
		}
	}
	if s.AdditionalProperties != nil && hasXValidations(s.AdditionalProperties.Structural) {
		return true
	}
	return false
}

func validateStaticCELCost(tb testing.TB, s *structuralschema.Structural, fldPath *field.Path, isRoot bool, cardinality uint64, totalCost *uint64) {
	tb.Helper()
	if s == nil {
		return
	}
	if len(s.XValidations) > 0 {
		declType := celmodel.SchemaDeclType(s, isRoot)
		if declType == nil {
			tb.Fatalf("%s: nil DeclType for schema with XValidations", fldPath)
		}
		envSet := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion())
		compResults, err := cel.Compile(s, declType, celconfig.PerCallLimit, envSet, cel.NewExpressionsEnvLoader())
		if err != nil {
			tb.Fatalf("%s: cel.Compile failed: %v", fldPath, err)
		}
		for i, cr := range compResults {
			if cr.Error != nil {
				tb.Fatalf("%s.x-kubernetes-validations[%d]: compile error: %v", fldPath, i, cr.Error)
			}
			exprCost := cr.MaxCost * cardinality
			*totalCost += exprCost
			if exprCost > staticEstimatedCostLimit {
				tb.Errorf("%s.x-kubernetes-validations[%d]: estimated rule cost %d exceeds per-rule budget %d (%.1fx)",
					fldPath, i, exprCost, staticEstimatedCostLimit, float64(exprCost)/float64(staticEstimatedCostLimit))
			}
		}
	}

	if s.Items != nil {
		itemCardinality := cardinality
		if s.ValueValidation != nil && s.ValueValidation.MaxItems != nil && *s.ValueValidation.MaxItems > 0 {
			itemCardinality *= uint64(*s.ValueValidation.MaxItems)
		} else if hasXValidations(s.Items) {
			tb.Errorf("%s: list schema with nested XValidations must set positive maxItems to bound static CEL cost", fldPath)
			itemCardinality *= staticEstimatedCostLimit
		}
		validateStaticCELCost(tb, s.Items, fldPath.Child("items"), false, itemCardinality, totalCost)
	}
	for name, prop := range s.Properties {
		validateStaticCELCost(tb, &prop, fldPath.Child("properties").Key(name), false, cardinality, totalCost)
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Structural != nil {
		valCardinality := cardinality
		if s.ValueValidation != nil && s.ValueValidation.MaxProperties != nil && *s.ValueValidation.MaxProperties > 0 {
			valCardinality *= uint64(*s.ValueValidation.MaxProperties)
		} else if len(s.XValidations) > 0 || hasXValidations(s.AdditionalProperties.Structural) {
			tb.Errorf("%s: map schema with XValidations must set positive maxProperties to bound static CEL cost", fldPath)
			valCardinality *= staticEstimatedCostLimit
		}
		validateStaticCELCost(tb, s.AdditionalProperties.Structural, fldPath.Child("additionalProperties"), false, valCardinality, totalCost)
	}
}

// TestRulesCRDStaticValidation verifies that the Rules, ClusterRules, and GlobalRules CRDs
// compile cleanly into structural schemas, OpenAPIv3 validators, and CEL validators, and pass
// Kubernetes API server static CEL cost estimation budgets.
func TestRulesCRDStaticValidation(t *testing.T) {
	for _, crdName := range []string{
		"rules.monitoring.googleapis.com",
		"clusterrules.monitoring.googleapis.com",
		"globalrules.monitoring.googleapis.com",
	} {
		t.Run(crdName, func(t *testing.T) {
			_, structural, _ := compileCRDValidators(t, crdName)
			var totalCost uint64
			validateStaticCELCost(t, structural, field.NewPath("openAPIV3Schema"), true, 1, &totalCost)
			t.Logf("%s total static CEL cost: %d / %d (%.1f%%)", crdName, totalCost, staticEstimatedCRDCostLimit, 100*float64(totalCost)/float64(staticEstimatedCRDCostLimit))
			if totalCost > staticEstimatedCRDCostLimit {
				t.Errorf("%s total static CEL cost %d exceeds CRD budget %d (%.1fx)", crdName, totalCost, staticEstimatedCRDCostLimit, float64(totalCost)/float64(staticEstimatedCRDCostLimit))
			}
		})
	}
}

func TestRulesCELAndOpenAPICases(t *testing.T) {
	rulesOpenAPI, rulesStructural, rulesCEL := compileCRDValidators(t, "rules.monitoring.googleapis.com")
	clusterRulesOpenAPI, clusterRulesStructural, clusterRulesCEL := compileCRDValidators(t, "clusterrules.monitoring.googleapis.com")
	globalRulesOpenAPI, globalRulesStructural, globalRulesCEL := compileCRDValidators(t, "globalrules.monitoring.googleapis.com")

	type testCase struct {
		name              string
		rawPayload        map[string]any
		spec              map[string]any
		wantRulesValid    bool
		wantClusterValid  bool
		wantGlobalValid   bool
		wantErrMsg        string
		wantNoCELErrors   bool
		skipWebhookParity bool
	}

	cases := []testCase{
		{
			name: "valid recording rule",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "group-1",
						"interval": "30s",
						"rules": []any{
							map[string]any{
								"record": "job:http_requests:rate5m",
								"expr":   "sum(rate(http_requests_total[5m]))",
								"labels": map[string]any{
									"env": "prod",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "valid alerting rule with for and annotations",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "group-alerts",
						"interval": "1m",
						"rules": []any{
							map[string]any{
								"alert": "HighErrorRate",
								"expr":  "job:http_errors:rate5m > 0.05",
								"for":   "5m",
								"labels": map[string]any{
									"severity": "page",
								},
								"annotations": map[string]any{
									"summary": "High error rate on {{ $labels.instance }}",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "valid alerting rule with Prometheus duration units (1d, 1w)",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "group-long-durations",
						"interval": "1d",
						"rules": []any{
							map[string]any{
								"alert": "DiskFillingSlowly",
								"expr":  "node_filesystem_avail_bytes == 0",
								"for":   "1w",
							},
						},
					},
				},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "valid recording rule with leading colon and underscore",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-identifiers",
						"rules": []any{
							map[string]any{
								"record": ":leading_colon_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"_leading_underscore": "ok",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "recording rule with empty annotations map accepted",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-empty-annotations",
						"rules": []any{
							map[string]any{
								"record":      "valid_metric",
								"expr":        "vector(1)",
								"annotations": map[string]any{},
							},
						},
					},
				},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "recording rule with non-empty annotations rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-recording-annotations",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"annotations": map[string]any{
									"summary": "not allowed on recording rule",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Annotations are only allowed for alerting rules",
		},
		{
			name: "both record and alert set rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-both-record-and-alert",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"alert":  "BothSet",
								"expr":   "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Must set exactly one of Record or Alert",
		},
		{
			name: "neither record nor alert set rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-neither-record-nor-alert",
						"rules": []any{
							map[string]any{
								"expr": "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Must set exactly one of Record or Alert",
		},
		{
			name: "empty group name rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "name in body should be at least 1 chars long",
		},
		{
			name: "duplicate group names rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "dup-group",
						"rules": []any{
							map[string]any{
								"record": "valid_metric_1",
								"expr":   "vector(1)",
							},
						},
					},
					map[string]any{
						"name": "dup-group",
						"rules": []any{
							map[string]any{
								"record": "valid_metric_2",
								"expr":   "vector(2)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Rule group names must be unique",
		},
		{
			name: "empty expr rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "expr in body should be at least 1 chars long",
		},
		{
			name: "for (5m) on recording rule rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"for":    "5m",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "For is only allowed for alerting rules",
		},
		{
			name: "for (1d) on recording rule rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"for":    "1d",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "For is only allowed for alerting rules",
		},
		{
			name: "for (0s) on recording rule rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"for":    "0s",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "For is only allowed for alerting rules",
		},
		{
			name: "__name__ label key rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"__name__": "override",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Label keys must match ^[a-zA-Z_][a-zA-Z0-9_]*$ and must not be __name__",
		},
		{
			name: "invalid label key with dash rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"invalid-label": "val",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Label keys must match ^[a-zA-Z_][a-zA-Z0-9_]*$ and must not be __name__",
		},
		{
			name: "invalid label key with colon rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"colon:not_allowed": "val",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Label keys must match ^[a-zA-Z_][a-zA-Z0-9_]*$ and must not be __name__",
		},
		{
			name: "invalid metric name with leading digit rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "0invalid_metric",
								"expr":   "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "record in body should match '^[a-zA-Z_:][a-zA-Z0-9_:]*$'",
		},
		{
			name: "invalid annotation key with colon rejected",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"alert": "MyAlert",
								"expr":  "vector(1)",
								"annotations": map[string]any{
									"colon:not_allowed": "val",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "Annotation keys must match ^[a-zA-Z_][a-zA-Z0-9_]*$",
		},
		{
			name: "reserved label project_id rejected on Rules and ClusterRules but allowed on GlobalRules",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"project_id": "other-project",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  true,
			wantErrMsg:       "labels cannot contain reserved keys",
		},
		{
			name: "reserved label location rejected on Rules and ClusterRules but allowed on GlobalRules",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"location": "us-east1",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  true,
			wantErrMsg:       "labels cannot contain reserved keys",
		},
		{
			name: "reserved label cluster rejected on Rules and ClusterRules but allowed on GlobalRules",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"cluster": "other-cluster",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  true,
			wantErrMsg:       "labels cannot contain reserved keys",
		},
		{
			name: "reserved label namespace rejected on Rules but allowed on ClusterRules and GlobalRules",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
								"labels": map[string]any{
									"namespace": "other-ns",
								},
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: true,
			wantGlobalValid:  true,
			wantErrMsg:       "Rule labels cannot contain reserved keys",
		},
		{
			name: "omitted spec rejected by OpenAPIv3 required",
			rawPayload: map[string]any{
				"apiVersion": "monitoring.googleapis.com/v1",
				"metadata": map[string]any{
					"name":      "missing-spec",
					"namespace": "default",
				},
			},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "spec in body is required",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
		{
			name:              "omitted spec.groups rejected by OpenAPIv3 required",
			spec:              map[string]any{},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "spec.groups in body is required",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
		{
			name: "empty spec.groups array accepted",
			spec: map[string]any{
				"groups": []any{},
			},
			wantRulesValid:   true,
			wantClusterValid: true,
			wantGlobalValid:  true,
		},
		{
			name: "omitted group.name rejected by OpenAPIv3 required and webhook",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
							},
						},
					},
					map[string]any{
						"name": "valid-group",
						"rules": []any{
							map[string]any{
								"record": "valid_metric_2",
								"expr":   "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "name in body is required",
			wantNoCELErrors:  true,
		},
		{
			name: "omitted group.rules rejected by OpenAPIv3 required",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
					},
				},
			},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "rules in body is required",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
		{
			name: "empty group.rules array rejected by OpenAPIv3 minItems",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name":  "group-1",
						"rules": []any{},
					},
				},
			},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "rules in body should have at least 1 items",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
		{
			name: "omitted rule.expr rejected by OpenAPIv3 required and webhook",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
							},
						},
					},
				},
			},
			wantRulesValid:   false,
			wantClusterValid: false,
			wantGlobalValid:  false,
			wantErrMsg:       "expr in body is required",
			wantNoCELErrors:  true,
		},
		{
			name: "explicit empty interval string rejected by OpenAPIv3 minLength",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "group-1",
						"interval": "",
						"rules": []any{
							map[string]any{
								"record": "valid_metric",
								"expr":   "vector(1)",
							},
						},
					},
				},
			},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "interval in body should be at least 1 chars long",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
		{
			name: "explicit empty for string on alerting rule rejected by OpenAPIv3 minLength",
			spec: map[string]any{
				"groups": []any{
					map[string]any{
						"name": "group-1",
						"rules": []any{
							map[string]any{
								"alert": "MyAlert",
								"expr":  "vector(1)",
								"for":   "",
							},
						},
					},
				},
			},
			wantRulesValid:    false,
			wantClusterValid:  false,
			wantGlobalValid:   false,
			wantErrMsg:        "for in body should be at least 1 chars long",
			wantNoCELErrors:   true,
			skipWebhookParity: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkKind := func(kind string, openapiVal validation.SchemaValidator, structural *structuralschema.Structural, celVal *cel.Validator, wantValid bool, runWebhook func([]byte) error) {
				payload := map[string]any{
					"apiVersion": "monitoring.googleapis.com/v1",
					"kind":       kind,
					"metadata": map[string]any{
						"name":      "test-rules",
						"namespace": "default",
					},
					"spec": tc.spec,
				}
				if tc.rawPayload != nil {
					payload = make(map[string]any, len(tc.rawPayload)+1)
					maps.Copy(payload, tc.rawPayload)
					payload["kind"] = kind
				}
				data, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("marshal %s: %v", kind, err)
				}
				var validationPayload map[string]any
				if err := json.Unmarshal(data, &validationPayload); err != nil {
					t.Fatalf("unmarshal %s: %v", kind, err)
				}
				defaulting.PruneNonNullableNullsWithoutDefaults(validationPayload, structural)
				defaulting.Default(validationPayload, structural)

				openapiRes := openapiVal.Validate(validationPayload)
				celErrs, _ := celVal.Validate(t.Context(), nil, structural, validationPayload, nil, celconfig.RuntimeCELCostBudget)
				crdValid := !openapiRes.HasErrors() && len(celErrs) == 0
				if crdValid != wantValid {
					t.Errorf("%s CRD validation got valid=%v, want %v (openapi=%v, cel=%v)", kind, crdValid, wantValid, openapiRes.Errors, celErrs)
				}
				if tc.wantNoCELErrors && len(celErrs) > 0 {
					t.Errorf("%s expected no CEL errors on OpenAPIv3 schema rejection, got %v", kind, celErrs)
				}
				if !wantValid && tc.wantErrMsg != "" {
					var combined strings.Builder
					for _, e := range openapiRes.Errors {
						combined.WriteString(e.Error())
						combined.WriteByte('\n')
					}
					for _, e := range celErrs {
						combined.WriteString(e.Error())
						combined.WriteByte('\n')
					}
					if !strings.Contains(combined.String(), tc.wantErrMsg) {
						t.Errorf("%s CRD rejection errors %q do not contain wantErrMsg %q", kind, combined.String(), tc.wantErrMsg)
					}
				}

				if !tc.skipWebhookParity {
					webhookErr := runWebhook(data)
					webhookValid := webhookErr == nil
					if webhookValid != wantValid {
						t.Errorf("%s webhook validation got valid=%v, want %v (err=%v)", kind, webhookValid, wantValid, webhookErr)
					}
				}
			}

			checkKind("Rules", rulesOpenAPI, rulesStructural, rulesCEL, tc.wantRulesValid, func(b []byte) error {
				var r Rules
				if err := json.Unmarshal(b, &r, json.RejectUnknownMembers(true)); err != nil {
					return err
				}
				_, err := r.ValidateCreate()
				return err
			})
			checkKind("ClusterRules", clusterRulesOpenAPI, clusterRulesStructural, clusterRulesCEL, tc.wantClusterValid, func(b []byte) error {
				var r ClusterRules
				if err := json.Unmarshal(b, &r, json.RejectUnknownMembers(true)); err != nil {
					return err
				}
				_, err := r.ValidateCreate()
				return err
			})
			checkKind("GlobalRules", globalRulesOpenAPI, globalRulesStructural, globalRulesCEL, tc.wantGlobalValid, func(b []byte) error {
				var r GlobalRules
				if err := json.Unmarshal(b, &r, json.RejectUnknownMembers(true)); err != nil {
					return err
				}
				_, err := r.ValidateCreate()
				return err
			})
		})
	}
}

func seedRulesCorpus(f *testing.F, kind string) {
	f.Helper()
	seeds := []map[string]any{
		{
			"apiVersion": "monitoring.googleapis.com/v1",
			"kind":       kind,
			"metadata": map[string]any{
				"name":      "recording-rules",
				"namespace": "default",
			},
			"spec": map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "recording-group",
						"interval": "30s",
						"rules": []any{
							map[string]any{
								"record": "instance:node_cpu:rate5m",
								"expr":   "rate(node_cpu_seconds_total[5m])",
								"labels": map[string]any{
									"team": "infra",
								},
							},
						},
					},
				},
			},
		},
		{
			"apiVersion": "monitoring.googleapis.com/v1",
			"kind":       kind,
			"metadata": map[string]any{
				"name":      "alerting-rules",
				"namespace": "default",
			},
			"spec": map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "alerting-group",
						"interval": "1m",
						"rules": []any{
							map[string]any{
								"alert": "InstanceDown",
								"expr":  "up == 0",
								"for":   "5m",
								"labels": map[string]any{
									"severity": "critical",
								},
								"annotations": map[string]any{
									"summary": "Instance {{ $labels.instance }} is down",
								},
							},
						},
					},
				},
			},
		},
		{
			"apiVersion": "monitoring.googleapis.com/v1",
			"kind":       kind,
			"metadata": map[string]any{
				"name":      "prometheus-durations",
				"namespace": "default",
			},
			"spec": map[string]any{
				"groups": []any{
					map[string]any{
						"name":     "daily-alerts",
						"interval": "1d",
						"rules": []any{
							map[string]any{
								"alert": "SlowBurn",
								"expr":  "vector(1)",
								"for":   "1w",
							},
						},
					},
				},
			},
		},
	}

	for _, s := range seeds {
		b, err := json.Marshal(s)
		if err == nil {
			f.Add(b)
		}
	}
}

func runRulesDifferentialFuzz(
	f *testing.F,
	crdName string,
	kind string,
	validateWebhook func(data []byte) error,
) {
	openapiValidator, structural, celValidator := compileCRDValidators(f, crdName)
	seedRulesCorpus(f, kind)

	f.Fuzz(func(t *testing.T, data []byte) {
		if !utf8.Valid(data) {
			t.Skip()
		}

		webhookErr := validateWebhook(data)
		if errors.Is(webhookErr, errSkipUnmarshal) {
			t.Skip()
		}
		webhookPassed := webhookErr == nil

		var unstructuredObj map[string]any
		if err := json.Unmarshal(data, &unstructuredObj); err != nil {
			t.Skip()
		}
		if unstructuredObj == nil {
			t.Skip()
		}
		// Status is a separate subresource and is not validated by ValidateCreate().
		delete(unstructuredObj, "status")
		defaulting.PruneNonNullableNullsWithoutDefaults(unstructuredObj, structural)
		defaulting.Default(unstructuredObj, structural)

		openapiResult := openapiValidator.Validate(unstructuredObj)
		celErrors, _ := celValidator.Validate(t.Context(), nil, structural, unstructuredObj, nil, celconfig.RuntimeCELCostBudget)
		crdPassed := !openapiResult.HasErrors() && len(celErrors) == 0

		if crdPassed != webhookPassed {
			if !crdPassed && webhookPassed {
				if isIntentionalOpenAPISchemaTightening(openapiResult.Errors, celErrors) {
					t.Skip("Narrowly skipping: intentional OpenAPIv3 schema tightening over Go struct unmarshaling")
				}
				t.Fatalf("Discrepancy (False Positive): CRD validation rejected %s, but Webhook accepted it.\nOpenAPI Errors: %v\nCEL Errors: %v\nPayload: %s", kind, openapiResult.Errors, celErrors, string(data))
			}
			if crdPassed && !webhookPassed {
				if isIntentionalPostAdmissionRulesError(webhookErr) {
					t.Skip("Narrowly skipping: intentional post-admission operator validation check")
				}
				t.Fatalf("Discrepancy (False Negative): CRD validation accepted %s, but Webhook rejected it.\nWebhook Error: %v\nPayload: %s", kind, webhookErr, string(data))
			}
		}
	})
}

var errSkipUnmarshal = errors.New("skip unmarshal")

// isIntentionalOpenAPISchemaTightening returns true when CRD OpenAPIv3 validation rejects a raw JSON
// payload that Go struct json.Unmarshal + ValidateCreate() accepts due to intentional schema constraints:
//   - Missing required envelope fields ("spec", "spec.groups") or null "spec.groups" (which unmarshal as nil in Go).
//   - Missing or empty "rules" in a RuleGroup (OpenAPIv3 enforces required + minItems: 1, while rulefmt allows 0 rules).
//   - Explicit "" on omitempty string/duration fields ("record": "", "alert": "", "interval": "", or "for": ""),
//     which OpenAPIv3 rejects via minLength: 1 while Go json.Unmarshal cannot distinguish "" from an omitted field.
//   - Prometheus year duration unit ("y") on "interval" or "for", which OpenAPIv3 format: duration rejects
//     (matching upstream Prometheus v3's removal of "y" from model.ParseDuration) while the v2.53.5 Go pin still parses it.
//   - Upper-bound limits (maxItems / maxProperties) required for Kubernetes CEL static cost estimation.
//   - TypeMeta/ObjectMeta schema checks on "apiVersion", "kind", or "metadata" not checked by ValidateCreate().
func isIntentionalOpenAPISchemaTightening(openapiErrs []error, celErrs field.ErrorList) bool {
	if len(openapiErrs) == 0 {
		return false
	}
	hasEmptyRecordOrAlert := false
	hasEmptyFor := false
	for _, err := range openapiErrs {
		msg := err.Error()
		if strings.Contains(msg, ".record in body should be at least 1 chars long") ||
			strings.Contains(msg, ".alert in body should be at least 1 chars long") {
			hasEmptyRecordOrAlert = true
		}
		if strings.Contains(msg, ".for in body should be at least 1 chars long") {
			hasEmptyFor = true
		}
		allowed := strings.Contains(msg, "spec in body is required") ||
			strings.Contains(msg, "spec.groups in body is required") ||
			strings.Contains(msg, "spec.groups in body must be of type array") ||
			strings.Contains(msg, ".rules in body is required") ||
			strings.Contains(msg, ".rules in body should have at least 1 items") ||
			strings.Contains(msg, ".rules in body must be of type array") ||
			strings.Contains(msg, ".record in body should be at least 1 chars long") ||
			strings.Contains(msg, ".alert in body should be at least 1 chars long") ||
			strings.Contains(msg, ".interval in body should be at least 1 chars long") ||
			strings.Contains(msg, ".for in body should be at least 1 chars long") ||
			strings.Contains(msg, ".interval in body must be of type duration") ||
			strings.Contains(msg, ".for in body must be of type duration") ||
			strings.Contains(msg, "should have at most") ||
			strings.Contains(msg, "should be at most 1024 chars long") ||
			strings.HasPrefix(msg, "apiVersion ") ||
			strings.HasPrefix(msg, "kind ") ||
			strings.HasPrefix(msg, "metadata.")
		if !allowed {
			return false
		}
	}
	for _, cerr := range celErrs {
		msg := cerr.Error()
		allowedCEL := (hasEmptyRecordOrAlert && strings.Contains(msg, "Must set exactly one of Record or Alert")) ||
			(hasEmptyFor && strings.Contains(msg, "For is only allowed for alerting rules"))
		if !allowedCEL {
			return false
		}
	}
	return true
}

// isIntentionalPostAdmissionRulesError returns true only for errors that are intentionally
// deferred to the operator reconciler (PromQL expression parsing, PromQL selector scope
// matcher conflicts, Go template parsing in alert labels/annotations, YAML control characters)
// or known kube-openapi duration format discrepancies.
func isIntentionalPostAdmissionRulesError(err error) bool {
	if err == nil {
		return false
	}
	if isDurationValidationDiscrepancy(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "could not parse expression") ||
		strings.Contains(msg, "parse PromQL expression") ||
		strings.Contains(msg, "conflicting label matcher") ||
		strings.Contains(msg, "template: __alert_") ||
		strings.Contains(msg, "executing template __alert_") ||
		strings.Contains(msg, "control characters are not allowed")
}

// FuzzRules runs differential fuzzing between the OpenAPIv3/CEL validations in the Rules CRD
// and the Go-based validation in Rules.ValidateCreate().
func FuzzRules(f *testing.F) {
	runRulesDifferentialFuzz(f, "rules.monitoring.googleapis.com", "Rules", func(data []byte) error {
		var r Rules
		if err := json.Unmarshal(data, &r, json.RejectUnknownMembers(true)); err != nil {
			return errSkipUnmarshal
		}
		_, err := r.ValidateCreate()
		return err
	})
}

// FuzzClusterRules runs differential fuzzing between the OpenAPIv3/CEL validations in the ClusterRules CRD
// and the Go-based validation in ClusterRules.ValidateCreate().
func FuzzClusterRules(f *testing.F) {
	runRulesDifferentialFuzz(f, "clusterrules.monitoring.googleapis.com", "ClusterRules", func(data []byte) error {
		var r ClusterRules
		if err := json.Unmarshal(data, &r, json.RejectUnknownMembers(true)); err != nil {
			return errSkipUnmarshal
		}
		_, err := r.ValidateCreate()
		return err
	})
}

// FuzzGlobalRules runs differential fuzzing between the OpenAPIv3/CEL validations in the GlobalRules CRD
// and the Go-based validation in GlobalRules.ValidateCreate().
func FuzzGlobalRules(f *testing.F) {
	runRulesDifferentialFuzz(f, "globalrules.monitoring.googleapis.com", "GlobalRules", func(data []byte) error {
		var r GlobalRules
		if err := json.Unmarshal(data, &r, json.RejectUnknownMembers(true)); err != nil {
			return errSkipUnmarshal
		}
		_, err := r.ValidateCreate()
		return err
	})
}

// FuzzRulesStructured mutates RuleGroup and Rule fields directly rather than raw JSON bytes,
// exercising OpenAPIv3 and CEL validation rules against ValidateCreate() at high throughput
// across Rules, ClusterRules, and GlobalRules.
func FuzzRulesStructured(f *testing.F) {
	rulesOpenAPI, rulesStructural, rulesCEL := compileCRDValidators(f, "rules.monitoring.googleapis.com")
	clusterOpenAPI, clusterStructural, clusterCEL := compileCRDValidators(f, "clusterrules.monitoring.googleapis.com")
	globalOpenAPI, globalStructural, globalCEL := compileCRDValidators(f, "globalrules.monitoring.googleapis.com")

	f.Add(uint8(0), "group-1", "30s", "group-2", "job:http_requests:rate5m", "", "vector(1)", "", "env", "prod", "", "", false, false)
	f.Add(uint8(1), "alerts", "1m", "alerts-2", "", "HighLatency", "up == 0", "5m", "severity", "page", "summary", "Instance down", true, true)
	f.Add(uint8(2), "daily", "1d", "dup", "", "DiskFull", "vector(1)", "1w", "project_id", "other", "description", "Disk full", false, false)
	f.Add(uint8(0), "dup", "10s", "dup", "valid_metric", "", "vector(1)", "0s", "__name__", "bad", "bad-key", "val", true, false)

	f.Fuzz(func(
		t *testing.T,
		kindIdx uint8,
		g1Name, g1Interval, g2Name string,
		r1Record, r1Alert, r1Expr, r1For string,
		labelKey, labelVal, annotKey, annotVal string,
		includeGroup2, includeRule2 bool,
	) {
		for _, s := range []string{g1Name, g1Interval, g2Name, r1Record, r1Alert, r1Expr, r1For, labelKey, labelVal, annotKey, annotVal} {
			if !utf8.ValidString(s) {
				t.Skip()
			}
		}

		r1 := Rule{
			Record: r1Record,
			Alert:  r1Alert,
			Expr:   r1Expr,
			For:    r1For,
		}
		if labelKey != "" || labelVal != "" {
			r1.Labels = map[string]string{labelKey: labelVal}
		}
		if annotKey != "" || annotVal != "" {
			r1.Annotations = map[string]string{annotKey: annotVal}
		}

		g1Rules := []Rule{r1}
		if includeRule2 {
			g1Rules = append(g1Rules, Rule{
				Record: "second_rule",
				Expr:   "vector(1)",
			})
		}
		groups := []RuleGroup{
			{
				Name:     g1Name,
				Interval: g1Interval,
				Rules:    g1Rules,
			},
		}
		if includeGroup2 {
			groups = append(groups, RuleGroup{
				Name: g2Name,
				Rules: []Rule{
					{
						Record: "group2_rule",
						Expr:   "vector(2)",
					},
				},
			})
		}

		spec := RulesSpec{Groups: groups}

		var (
			kind             string
			openapiValidator validation.SchemaValidator
			structural       *structuralschema.Structural
			celValidator     *cel.Validator
			data             []byte
			err              error
			webhookErr       error
		)

		switch kindIdx % 3 {
		case 0:
			kind = "Rules"
			openapiValidator, structural, celValidator = rulesOpenAPI, rulesStructural, rulesCEL
			obj := Rules{
				APIVersion: "monitoring.googleapis.com/v1",
				Kind:       kind,
				Name:       "fuzz-rules",
				Namespace:  "default",
				Spec:       spec,
			}
			data, err = json.Marshal(obj)
			if err != nil {
				t.Skip()
			}
			_, webhookErr = obj.ValidateCreate()
		case 1:
			kind = "ClusterRules"
			openapiValidator, structural, celValidator = clusterOpenAPI, clusterStructural, clusterCEL
			obj := ClusterRules{
				APIVersion: "monitoring.googleapis.com/v1",
				Kind:       kind,
				Name:       "fuzz-clusterrules",
				Spec:       spec,
			}
			data, err = json.Marshal(obj)
			if err != nil {
				t.Skip()
			}
			_, webhookErr = obj.ValidateCreate()
		default:
			kind = "GlobalRules"
			openapiValidator, structural, celValidator = globalOpenAPI, globalStructural, globalCEL
			obj := GlobalRules{
				APIVersion: "monitoring.googleapis.com/v1",
				Kind:       kind,
				Name:       "fuzz-globalrules",
				Spec:       spec,
			}
			data, err = json.Marshal(obj)
			if err != nil {
				t.Skip()
			}
			_, webhookErr = obj.ValidateCreate()
		}

		var unstructuredObj map[string]any
		if err := json.Unmarshal(data, &unstructuredObj); err != nil {
			t.Skip()
		}
		delete(unstructuredObj, "status")
		defaulting.PruneNonNullableNullsWithoutDefaults(unstructuredObj, structural)
		defaulting.Default(unstructuredObj, structural)

		openapiResult := openapiValidator.Validate(unstructuredObj)
		celErrors, _ := celValidator.Validate(t.Context(), nil, structural, unstructuredObj, nil, celconfig.RuntimeCELCostBudget)
		crdPassed := !openapiResult.HasErrors() && len(celErrors) == 0
		webhookPassed := webhookErr == nil

		if crdPassed != webhookPassed {
			if !crdPassed && webhookPassed {
				if isIntentionalOpenAPISchemaTightening(openapiResult.Errors, celErrors) {
					t.Skip("Narrowly skipping: intentional OpenAPIv3 schema tightening over Go struct unmarshaling")
				}
				t.Fatalf("Discrepancy (False Positive) in %s: CRD rejected, Webhook accepted.\nOpenAPI Errors: %v\nCEL Errors: %v\nPayload: %s", kind, openapiResult.Errors, celErrors, string(data))
			}
			if crdPassed && !webhookPassed {
				if isIntentionalPostAdmissionRulesError(webhookErr) {
					t.Skip("Narrowly skipping: intentional post-admission operator validation check")
				}
				t.Fatalf("Discrepancy (False Negative) in %s: CRD accepted, Webhook rejected.\nWebhook Error: %v\nPayload: %s", kind, webhookErr, string(data))
			}
		}
	})
}
