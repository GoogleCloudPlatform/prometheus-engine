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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	model "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/google/export"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"go.yaml.in/yaml/v4"
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
				Namespace: "test-namespace",
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
			name: "multiline strings with leading newline and spaces",
			apiRules: &Rules{
				Namespace: "test-namespace",
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "\n0",
							Rules: []Rule{
								{
									Alert: " alert\nname",
									Expr:  "  test_expr\n+ test_expr",
								},
							},
						},
						{
							Name: " group\nname",
							Rules: []Rule{
								{
									Record: "test_record",
									Expr:   "\ntest_expr",
								},
							},
						},
					},
				},
			},
			want: `groups:
  - name: |-

      0
    rules:
      - alert: |2-
           alert
          name
        expr: test_expr{cluster="test-cluster",location="us-central1",namespace="test-namespace",project_id="123"} + test_expr{cluster="test-cluster",location="us-central1",namespace="test-namespace",project_id="123"}
        labels:
          cluster: test-cluster
          location: us-central1
          namespace: test-namespace
          project_id: "123"
  - name: |2-
       group
      name
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
			name: "invalid record name",
			apiRules: &Rules{
				Spec: RulesSpec{
					Groups: []RuleGroup{
						{
							Name: "test-group",
							Rules: []Rule{
								{
									Record: "invalid-metric-name",
									Expr:   "test_expr",
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
	groups, errs := rulefmt.Parse([]byte(input), false, model.LegacyValidation, parser.NewParser(parser.Options{}), nil)
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

func TestRuleGroupsConfigErrorAccumulation(t *testing.T) {
	const (
		projectID   = "test-proj"
		location    = "us-central1"
		clusterName = "test-cluster"
		namespace   = "team-a"
	)

	t.Run("combined syntax error and selector scope conflict", func(t *testing.T) {
		r := &Rules{
			Namespace: namespace,
			Spec: RulesSpec{
				Groups: []RuleGroup{
					{
						Name: "invalid-group",
						Rules: []Rule{
							{
								Record: "bad_expr",
								Expr:   "sum(rate(up[5m])",
							},
							{
								Record: "bad_scope",
								Expr:   `up{namespace="other-ns"}`,
							},
						},
					},
				},
			},
		}
		_, err := r.RuleGroupsConfig(projectID, location, clusterName)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		for _, wantSubstr := range []string{
			"converting rules failed:",
			"could not parse expression",
			"isolating rules failed:",
			"conflicting label matcher",
		} {
			if !strings.Contains(msg, wantSubstr) {
				t.Errorf("expected error %q to contain %q", msg, wantSubstr)
			}
		}
		if strings.Contains(msg, "parse PromQL expression") {
			t.Errorf("expected no duplicate scope PromQL parse error in %q", msg)
		}

		cr := &ClusterRules{
			Spec: RulesSpec{
				Groups: []RuleGroup{
					{
						Name: "invalid-cluster-group",
						Rules: []Rule{
							{
								Record: "bad_expr",
								Expr:   "sum(rate(up[5m])",
							},
							{
								Record: "bad_scope",
								Expr:   `up{cluster="other-cluster"}`,
							},
						},
					},
				},
			},
		}
		_, err = cr.RuleGroupsConfig(projectID, location, clusterName)
		if err == nil {
			t.Fatal("expected error for ClusterRules, got nil")
		}
		if !strings.Contains(err.Error(), "could not parse expression") || !strings.Contains(err.Error(), "conflicting label matcher") {
			t.Errorf("expected ClusterRules error %q to contain both syntax and scope errors", err.Error())
		}
	})

	t.Run("syntax error only emits no duplicate parse error", func(t *testing.T) {
		r := &Rules{
			Namespace: namespace,
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
		}
		_, err := r.RuleGroupsConfig(projectID, location, clusterName)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if got := strings.Count(msg, "unexpected end of input inside braces"); got != 1 {
			t.Errorf("expected exactly 1 PromQL parse error occurrence, got %d in %q", got, msg)
		}
		for _, unwanted := range []string{"isolating rules failed", "parse PromQL expression"} {
			if strings.Contains(msg, unwanted) {
				t.Errorf("expected error %q not to contain %q", msg, unwanted)
			}
		}
	})

	t.Run("single rule with syntax error and result label conflict", func(t *testing.T) {
		r := &Rules{
			Namespace: namespace,
			Spec: RulesSpec{
				Groups: []RuleGroup{
					{
						Name: "test-group",
						Rules: []Rule{
							{
								Record: "bad_both",
								Expr:   "sum(rate(up[5m])",
								Labels: map[string]string{
									export.KeyNamespace: "other-ns",
								},
							},
						},
					},
				},
			},
		}
		_, err := r.RuleGroupsConfig(projectID, location, clusterName)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		for _, wantSubstr := range []string{
			"could not parse expression",
			`set result isolation label namespace="team-a" on {bad_both  sum(rate(up[5m]) 0s 0s map[namespace:other-ns] map[]}: label "namespace" already set on rule with unexpected value "other-ns"`,
		} {
			if !strings.Contains(msg, wantSubstr) {
				t.Errorf("expected error %q to contain %q", msg, wantSubstr)
			}
		}
		if strings.Contains(msg, "parse PromQL expression") {
			t.Errorf("expected no duplicate scope PromQL parse error in %q", msg)
		}
	})

	t.Run("invalid durations and recording rule with for combined with scope conflict", func(t *testing.T) {
		r := &Rules{
			Namespace: namespace,
			Spec: RulesSpec{
				Groups: []RuleGroup{
					{
						Name:     "group-1",
						Interval: "not-a-duration",
						Rules: []Rule{
							{
								Alert: "BadForDuration",
								Expr:  "up == 0",
								For:   "also-not-a-duration",
							},
							{
								Record: "rec_with_for",
								Expr:   "vector(1)",
								For:    "5m",
							},
						},
					},
					{
						Name: "group-2",
						Rules: []Rule{
							{
								Record: "bad_syntax",
								Expr:   "rate(up[5m]",
							},
							{
								Record: "bad_scope",
								Expr:   `up{namespace="other-ns"}`,
							},
						},
					},
				},
			},
		}
		_, err := r.RuleGroupsConfig(projectID, location, clusterName)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		for _, wantSubstr := range []string{
			"parse 'for' duration:",
			"invalid field 'for' in recording rule",
			"parse evaluation interval:",
			"could not parse expression",
			"conflicting label matcher",
		} {
			if !strings.Contains(msg, wantSubstr) {
				t.Errorf("expected error %q to contain %q", msg, wantSubstr)
			}
		}
		if got := strings.Count(msg, "invalid field 'for' in recording rule"); got != 1 {
			t.Errorf("expected exactly 1 occurrence of recording rule 'for' error, got %d in %q", got, msg)
		}
	})

	t.Run("scopeRules suppresses parse errors when already validated", func(t *testing.T) {
		groups := &rulefmt.RuleGroups{
			Groups: []rulefmt.RuleGroup{
				{
					Name: "prevalidated",
					Rules: []rulefmt.Rule{
						{
							Record: "bad_syntax_and_label",
							Expr:   "sum(rate(up[5m])",
							Labels: map[string]string{"namespace": "other-ns"},
						},
						{
							Record: "bad_scope",
							Expr:   `up{namespace="other-ns"}`,
						},
					},
				},
			},
		}
		err := scopeRules(groups, map[string]string{"namespace": namespace}, false)
		if err == nil {
			t.Fatal("expected error from scopeRules, got nil")
		}
		msg := err.Error()
		for _, wantSubstr := range []string{
			`group "prevalidated" rule #1: set result isolation label namespace="team-a"`,
			`group "prevalidated" rule #2: set isolation selector namespace="team-a"`,
			"conflicting label matcher",
		} {
			if !strings.Contains(msg, wantSubstr) {
				t.Errorf("expected error %q to contain %q", msg, wantSubstr)
			}
		}
		if strings.Contains(msg, "parse PromQL expression") {
			t.Errorf("expected scopeRules with reportParseErrors=false not to contain %q, got %q", "parse PromQL expression", msg)
		}
	})

	t.Run("standalone scope reports parse error and result label conflict", func(t *testing.T) {
		groups := &rulefmt.RuleGroups{
			Groups: []rulefmt.RuleGroup{
				{
					Name: "standalone",
					Rules: []rulefmt.Rule{
						{
							Record: "bad_expr_and_label",
							Expr:   "up{",
							Labels: map[string]string{"namespace": "other-ns"},
						},
					},
				},
			},
		}
		err := scope(groups, map[string]string{"namespace": namespace})
		if err == nil {
			t.Fatal("expected error from standalone scope, got nil")
		}
		msg := err.Error()
		for _, wantSubstr := range []string{
			`group "standalone" rule #1: parse PromQL expression:`,
			`group "standalone" rule #1: set result isolation label namespace="team-a"`,
		} {
			if !strings.Contains(msg, wantSubstr) {
				t.Errorf("expected error %q to contain %q", msg, wantSubstr)
			}
		}
	})
}
