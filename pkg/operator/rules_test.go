// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package operator

import (
	"context"
	"errors"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestGenerateRules(t *testing.T) {
	var wantRules = `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr{cluster="test-cluster",location="us-central1",namespace="test-namespace",project_id="123"}
          labels:
            cluster: test-cluster
            location: us-central1
            namespace: test-namespace
            project_id: "123"
`

	tests := []struct {
		name        string
		apiRules    *monitoringv1.Rules
		projectID   string
		location    string
		clusterName string
		want        string
		wantErr     bool
	}{
		{
			name: "good rules",
			apiRules: &monitoringv1.Rules{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			projectID:   "123",
			location:    "us-central1",
			clusterName: "test-cluster",
			want:        wantRules,
			wantErr:     false,
		},
		{
			name: "invalid rules",
			apiRules: &monitoringv1.Rules{
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateRules(test.apiRules, test.projectID, test.location, test.clusterName)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if string(got) != test.want {
				t.Errorf("expected: %v; actual %v", test.want, string(got))
			}
		})
	}
}

func TestGenerateClusterRules(t *testing.T) {
	var wantClusterRules = `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr{cluster="test-cluster",location="us-central1",project_id="123"}
          labels:
            cluster: test-cluster
            location: us-central1
            project_id: "123"
`

	tests := []struct {
		name        string
		apiRules    *monitoringv1.ClusterRules
		projectID   string
		location    string
		clusterName string
		want        string
		wantErr     bool
	}{
		{
			name: "good cluster rules",
			apiRules: &monitoringv1.ClusterRules{
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			projectID:   "123",
			location:    "us-central1",
			clusterName: "test-cluster",
			want:        wantClusterRules,
			wantErr:     false,
		},
		{
			name: "invalid cluster rules",
			apiRules: &monitoringv1.ClusterRules{
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateClusterRules(test.apiRules, test.projectID, test.location, test.clusterName)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if diff := cmp.Diff(test.want, string(got)); diff != "" {
				t.Fatalf("unexpected result (-want, +got):\n %s", diff)
			}
		})
	}
}

func TestGenerateGlobalRules(t *testing.T) {
	var wantGlobalRules = `groups:
    - name: test-group
      rules:
        - record: test_record
          expr: test_expr
`

	tests := []struct {
		name     string
		apiRules *monitoringv1.GlobalRules
		want     string
		wantErr  bool
	}{
		{
			name: "good global rules",
			apiRules: &monitoringv1.GlobalRules{
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
								{
									Record: "test_record",
									Expr:   "test_expr",
								},
							},
						},
					},
				},
			},
			want:    wantGlobalRules,
			wantErr: false,
		},
		{
			name: "invalid global rules",
			apiRules: &monitoringv1.GlobalRules{
				Spec: monitoringv1.RulesSpec{
					Groups: []monitoringv1.RuleGroup{
						{
							Name: "test-group",
							Rules: []monitoringv1.Rule{
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateGlobalRules(test.apiRules)
			if (err == nil && test.wantErr) || (err != nil && !test.wantErr) {
				t.Fatalf("expected err: %v; actual %v", test.wantErr, err)
			}
			if diff := cmp.Diff(test.want, string(got)); diff != "" {
				t.Fatalf("unexpected result (-want, +got):\n %s", diff)
			}
		})
	}
}

func TestHasRules(t *testing.T) {
	emptyRules := newFakeClientBuilder().Build()
	hasRule := newFakeClientBuilder().WithObjects(&monitoringv1.Rules{}).Build()
	errGettingRules := newFakeClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
			return errors.New("error")
		}}).Build()

	type test struct {
		client  client.Client
		want    bool
		wantErr bool
	}

	tests := map[string]test{
		"no rules": {client: emptyRules, want: false},
		"has rule": {client: hasRule, want: true},
		"error":    {client: errGettingRules, want: false, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := hasRules(context.Background(), tc.client)
			if got != tc.want {
				t.Errorf("want: %t, got: %t", tc.want, got)
			}
			if err != nil && !tc.wantErr {
				t.Errorf("Unexpected error: %s", err)
			}
		})
	}
}

func TestScaleRuleConsumers(t *testing.T) {
	var alertmanagerReplicas int32
	alertManager := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alertmanager",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &alertmanagerReplicas,
		},
	}
	var ruleEvaluatorReplicas int32
	ruleEvaluator := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rule-evaluator",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &ruleEvaluatorReplicas,
		},
	}
	emptyRules := newFakeClientBuilder().WithObjects(&alertManager, &ruleEvaluator).Build()
	hasRule := newFakeClientBuilder().WithObjects(&alertManager, &ruleEvaluator, &monitoringv1.Rules{}).Build()
	errGettingRules := newFakeClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
			return errors.New("error")
		}}).Build()
	alertmanagerDeleted := newFakeClientBuilder().WithObjects(&ruleEvaluator).Build()
	ruleEvaluatorDeleted := newFakeClientBuilder().WithObjects(&alertManager).Build()
	alertmanagerDeletedWithRules := newFakeClientBuilder().WithObjects(&ruleEvaluator, &monitoringv1.Rules{}).Build()
	ruleEvaluatorDeletedWithRules := newFakeClientBuilder().WithObjects(&alertManager, &monitoringv1.Rules{}).Build()

	type test struct {
		client  client.Client
		want    int32
		wantErr bool
	}

	tests := map[string]test{
		"no rules":                          {client: emptyRules, want: 0},
		"has rule":                          {client: hasRule, want: 1},
		"error":                             {client: errGettingRules, want: 0, wantErr: true},
		"alertmanager deleted":              {client: alertmanagerDeleted, want: 0, wantErr: false},
		"rule-evaluator deleted":            {client: ruleEvaluatorDeleted, want: 0, wantErr: false},
		"alertmanager deleted with rules":   {client: alertmanagerDeletedWithRules, want: 1, wantErr: false},
		"rule-evaluator deleted with rules": {client: ruleEvaluatorDeletedWithRules, want: 1, wantErr: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := rulesReconciler{
				client: tc.client,
			}
			err := r.scaleRuleConsumers(context.Background())
			if err != nil {
				if !tc.wantErr {
					t.Errorf("Unexpected error: %s", err)
				}
				return
			}

			var alertmanager appsv1.StatefulSet
			if err := r.client.Get(context.Background(), client.ObjectKey{Name: "alertmanager"}, &alertmanager); client.IgnoreNotFound(err) != nil {
				t.Error(err)
			}
			if alertmanager.Spec.Replicas != nil && *alertmanager.Spec.Replicas != tc.want {
				t.Errorf("want: %d, got: %d", tc.want, *alertmanager.Spec.Replicas)
			}

			var ruleEvaluator appsv1.Deployment
			if err := r.client.Get(context.Background(), client.ObjectKey{Name: "rule-evaluator"}, &ruleEvaluator); client.IgnoreNotFound(err) != nil {
				t.Error(err)
			}
			if ruleEvaluator.Spec.Replicas != nil && *ruleEvaluator.Spec.Replicas != tc.want {
				t.Errorf("want: %d, got: %d", tc.want, ruleEvaluator.Spec.Replicas)
			}
		})
	}
}
