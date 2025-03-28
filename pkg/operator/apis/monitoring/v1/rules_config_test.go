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

package v1

import (
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/model/rulefmt"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateRules(t *testing.T) {
	projectID := "123"
	location := "us-central1"
	clusterName := "test-cluster"

	tests := []struct {
		name     string
		apiRules *Rules
		want     string
		wantErr  bool
	}{
		{
			name: "good",
			apiRules: &Rules{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr{cluster="test-cluster",location="us-central1",namespace="test-namespace",project_id="123"}
          labels:
            cluster: test-cluster
            location: us-central1
            namespace: test-namespace
            project_id: "123"
`,
			wantErr: false,
		},
		{
			name: "invalid expression",
			apiRules: &Rules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr{",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "project label",
			apiRules: &Rules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyProjectID: "other",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "namespace label",
			apiRules: &Rules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyNamespace: "other",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&RulesCustomValidator{}).ValidateCreate(t.Context(), test.apiRules)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}

			got, err := test.apiRules.RuleGroupsConfig(projectID, location, clusterName)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("expected rule groups config (-want, +got): %s", diff)
			}
		})
	}
}

func TestGenerateClusterRules(t *testing.T) {
	projectID := "123"
	location := "us-central1"
	clusterName := "test-cluster"

	tests := []struct {
		name     string
		apiRules *ClusterRules
		want     string
		wantErr  bool
	}{
		{
			name: "good",
			apiRules: &ClusterRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr{cluster="test-cluster",location="us-central1",project_id="123"}
          labels:
            cluster: test-cluster
            location: us-central1
            project_id: "123"
`,
			wantErr: false,
		},
		{
			name: "invalid expression",
			apiRules: &ClusterRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr{",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "project label",
			apiRules: &ClusterRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyProjectID: "other",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "namespace label",
			apiRules: &ClusterRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyNamespace: "other",
									},
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr{cluster="test-cluster",location="us-central1",project_id="123"}
          labels:
            cluster: test-cluster
            location: us-central1
            namespace: other
            project_id: "123"
`,
			wantErr: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&ClusterRulesCustomValidator{}).ValidateCreate(t.Context(), test.apiRules)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}

			got, err := test.apiRules.RuleGroupsConfig(projectID, location, clusterName)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("expected rule groups config (-want, +got): %s", diff)
			}
		})
	}
}

func TestGenerateGlobalRules(t *testing.T) {
	tests := []struct {
		name     string
		apiRules *GlobalRules
		want     string
		wantErr  bool
	}{
		{
			name: "good",
			apiRules: &GlobalRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr
`,
			wantErr: false,
		},
		{
			name: "invalid expression",
			apiRules: &GlobalRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr{",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "project label",
			apiRules: &GlobalRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyProjectID: "other",
									},
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr
          labels:
            project_id: other
`,
		},
		{
			name: "namespace label",
			apiRules: &GlobalRules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
									Labels: map[string]string{
										export.KeyNamespace: "other",
									},
								},
							},
						},
					},
				},
			},
			want: `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr
          labels:
            namespace: other
`,
			wantErr: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&GlobalRulesCustomValidator{}).ValidateCreate(t.Context(), test.apiRules)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}

			got, err := test.apiRules.RuleGroupsConfig()
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("expected rule groups config (-want, +got): %s", diff)
			}
		})
	}
}

func TestScope(t *testing.T) {
	input := `groups:
- name: test
  rules:
  - record: rule:1
    expr: vector(1)
  - record: rule:2
    expr: sum by(foo, bar) (rate(my_metric[5m]))
    labels:
      a: b
  - alert: Bar
    expr: my_metric1 / my_metric2{a="b"} > 0
`
	groups, errs := rulefmt.Parse([]byte(input))
	if len(errs) > 0 {
		t.Fatalf("Unexpected input errors: %s", errs)
	}
	if err := scope(groups, map[string]string{
		"l1": "v1",
		"l2": "v2",
	}); err != nil {
		t.Error(err)
	}
	want := `groups:
    - name: test
      rules:
        - record: rule:1
          expr: vector(1)
          labels:
            l1: v1
            l2: v2
        - record: rule:2
          expr: sum by (foo, bar) (rate(my_metric{l1="v1",l2="v2"}[5m]))
          labels:
            a: b
            l1: v1
            l2: v2
        - alert: Bar
          expr: my_metric1{l1="v1",l2="v2"} / my_metric2{a="b",l1="v1",l2="v2"} > 0
          labels:
            l1: v1
            l2: v2
`
	got, err := yaml.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Fatalf("unexpected result (-want, +got):\n %s", diff)
	}
}
