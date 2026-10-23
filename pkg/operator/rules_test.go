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
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

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
			got, err := hasRules(t.Context(), tc.client)
			if got != tc.want {
				t.Errorf("want: %t, got: %t", tc.want, got)
			}
			if err != nil && !tc.wantErr {
				t.Errorf("Unexpected error: %s", err)
			}
		})
	}
}

func TestRulesStatus(t *testing.T) {
	// The fake client truncates seconds, so create a time that can't be rounded down.
	timeDefault := metav1.NewTime(time.Date(2024, 5, 23, 1, 23, 0, 0, time.UTC))
	timeAfter := metav1.NewTime(timeDefault.Add(time.Minute))
	const expectedInvalidMessage = `generating rule config failed: converting rules failed: loading rules failed: 5:15: group "test-group", rule 1, "test_record": could not parse expression: 1:11: parse error: unexpected end of input inside braces`
	var testCases []struct {
		obj            monitoringv1.MonitoringCRD
		expectedStatus monitoringv1.MonitoringStatus
	}

	addTestCases := func(name string, newObj func(objectMeta metav1.ObjectMeta, spec monitoringv1.RulesSpec, status monitoringv1.RulesStatus) monitoringv1.MonitoringCRD) {
		testCases = append(testCases, []struct {
			obj            monitoringv1.MonitoringCRD
			expectedStatus monitoringv1.MonitoringStatus
		}{
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("invalid-%s-no-condition", name),
						Generation: 2,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 2,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionFalse,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeAfter,
							Reason:             reasonRuleConfigError,
							Message:            expectedInvalidMessage,
						},
					},
				},
			},
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("invalid-%s-outdated-condition", name),
						Generation: 2,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{
						ObservedGeneration: 1,
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:               monitoringv1.ConfigurationCreateSuccess,
								Status:             corev1.ConditionTrue,
								LastUpdateTime:     timeDefault,
								LastTransitionTime: timeDefault,
							},
						},
					},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 2,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionFalse,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeAfter,
							Reason:             reasonRuleConfigError,
							Message:            expectedInvalidMessage,
						},
					},
				},
			},
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("invalid-%s-correct-condition", name),
						Generation: 2,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{
						ObservedGeneration: 1,
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:               monitoringv1.ConfigurationCreateSuccess,
								Status:             corev1.ConditionFalse,
								LastUpdateTime:     timeDefault,
								LastTransitionTime: timeDefault,
							},
						},
					},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 2,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionFalse,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeDefault,
							Reason:             reasonRuleConfigError,
							Message:            expectedInvalidMessage,
						},
					},
				},
			},
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("valid-%s-no-condition", name),
						Generation: 1,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 1,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeAfter,
						},
					},
				},
			},
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("valid-%s-outdated-condition", name),
						Generation: 2,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{
						ObservedGeneration: 1,
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:               monitoringv1.ConfigurationCreateSuccess,
								Status:             corev1.ConditionFalse,
								LastUpdateTime:     timeDefault,
								LastTransitionTime: timeDefault,
							},
						},
					},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 2,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeAfter,
						},
					},
				},
			},
			{
				obj: newObj(
					metav1.ObjectMeta{
						Name:       fmt.Sprintf("valid-%s-correct-condition", name),
						Generation: 2,
					},
					monitoringv1.RulesSpec{
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
					monitoringv1.RulesStatus{
						ObservedGeneration: 1,
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:               monitoringv1.ConfigurationCreateSuccess,
								Status:             corev1.ConditionTrue,
								LastUpdateTime:     timeDefault,
								LastTransitionTime: timeDefault,
							},
						},
					},
				),
				expectedStatus: monitoringv1.MonitoringStatus{
					ObservedGeneration: 2,
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     timeAfter,
							LastTransitionTime: timeDefault,
						},
					},
				},
			},
		}...)
	}
	addTestCases("namespaced-rule", func(objectMeta metav1.ObjectMeta, spec monitoringv1.RulesSpec, status monitoringv1.RulesStatus) monitoringv1.MonitoringCRD {
		return &monitoringv1.Rules{
			ObjectMeta: objectMeta,
			Spec:       spec,
			Status:     status,
		}
	})
	addTestCases("cluster-rule", func(objectMeta metav1.ObjectMeta, spec monitoringv1.RulesSpec, status monitoringv1.RulesStatus) monitoringv1.MonitoringCRD {
		return &monitoringv1.ClusterRules{
			ObjectMeta: objectMeta,
			Spec:       spec,
			Status:     status,
		}
	})
	addTestCases("global-rule", func(objectMeta metav1.ObjectMeta, spec monitoringv1.RulesSpec, status monitoringv1.RulesStatus) monitoringv1.MonitoringCRD {
		return &monitoringv1.GlobalRules{
			ObjectMeta: objectMeta,
			Spec:       spec,
			Status:     status,
		}
	})

	objs := make([]client.Object, 0, len(testCases))
	for _, tc := range testCases {
		objs = append(objs, tc.obj)
	}
	kubeClient := newFakeClientBuilder().
		WithObjects(objs...).
		Build()

	r := rulesReconciler{
		client: kubeClient,
	}

	if err := r.ensureRuleConfigs(t.Context(), "", "", "", monitoringv1.CompressionNone); err != nil {
		t.Fatal("ensure rules configs:", err)
	}

	for _, tc := range testCases {
		t.Run(tc.obj.GetName(), func(t *testing.T) {
			objectKey := client.ObjectKeyFromObject(tc.obj)
			if err := kubeClient.Get(t.Context(), objectKey, tc.obj); err != nil {
				t.Fatal("get obj:", err)
			}

			status := tc.obj.GetMonitoringStatus()
			if len(status.Conditions) != 1 {
				t.Fatalf("invalid %q conditions amount, expected 1 but got %d", objectKey, len(status.Conditions))
			}
			condition := &status.Conditions[0]
			// If time changed, normalize to the "after time", since we don't mock the process time.
			if !condition.LastTransitionTime.Equal(&timeDefault) {
				condition.LastTransitionTime = timeAfter
			}
			if !condition.LastUpdateTime.Equal(&timeDefault) {
				condition.LastUpdateTime = timeAfter
			}

			if diff := cmp.Diff(&tc.expectedStatus, status); diff != "" {
				t.Errorf("expected %q condition (-want, +got): %s", objectKey, diff)
			}
		})
	}
}

func TestRulesInvalidEvents(t *testing.T) {
	invalidSpec := monitoringv1.RulesSpec{
		Groups: []monitoringv1.RuleGroup{
			{
				Name: "api-alerts",
				Rules: []monitoringv1.Rule{
					{
						Record: "job:requests:rate5m",
						Expr:   "sum(rate(http_requests_total[5m])",
					},
				},
			},
		},
	}

	namespacedRules := &monitoringv1.Rules{
		Name:       "bad-rules",
		Namespace:  "team-a",
		Generation: 1,
		Spec:       invalidSpec,
	}
	clusterRules := &monitoringv1.ClusterRules{
		Name:       "bad-cluster-rules",
		Generation: 1,
		Spec:       invalidSpec,
	}
	globalRules := &monitoringv1.GlobalRules{
		Name:       "bad-global-rules",
		Generation: 1,
		Spec:       invalidSpec,
	}

	kubeClient := newFakeClientBuilder().
		WithObjects(namespacedRules, clusterRules, globalRules).
		Build()

	r := rulesReconciler{
		client: kubeClient,
		opts: Options{
			OperatorNamespace: DefaultOperatorNamespace,
		},
	}

	if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
		t.Fatalf("ensureRuleConfigs failed: %v", err)
	}

	// Verify 3 workload events in gmp-system (one for each invalid resource).
	var opEvents eventsv1.EventList
	if err := kubeClient.List(t.Context(), &opEvents, client.InNamespace(DefaultOperatorNamespace)); err != nil {
		t.Fatalf("list operator events: %v", err)
	}
	if len(opEvents.Items) != 3 {
		t.Fatalf("expected 3 workload events in %s, got %d", DefaultOperatorNamespace, len(opEvents.Items))
	}
	for _, ev := range opEvents.Items {
		if ev.Type != corev1.EventTypeWarning {
			t.Errorf("expected Warning event, got %s", ev.Type)
		}
		if ev.Action != "Ignored" {
			t.Errorf("expected Action Ignored, got %s", ev.Action)
		}
		if ev.Reason != reasonRuleConfigError {
			t.Errorf("expected Reason %s, got %s", reasonRuleConfigError, ev.Reason)
		}
		if ev.ReportingController != reportingController {
			t.Errorf("expected ReportingController %s, got %s", reportingController, ev.ReportingController)
		}
		if ev.ReportingInstance != NameOperator {
			t.Errorf("expected ReportingInstance %s, got %s", NameOperator, ev.ReportingInstance)
		}
		if ev.EventTime.IsZero() {
			t.Errorf("expected non-zero EventTime on workload event: %+v", ev)
		}
		if ev.Regarding.Kind != "Deployment" || ev.Regarding.Name != NameOperator || ev.Regarding.Namespace != DefaultOperatorNamespace {
			t.Errorf("unexpected Regarding on workload event: %+v", ev.Regarding)
		}
		if ev.Related == nil {
			t.Errorf("expected non-nil Related on workload event: %+v", ev)
		}
		if !strings.Contains(ev.Note, `group "api-alerts"`) {
			t.Errorf("expected event Note to mention group \"api-alerts\", got %q", ev.Note)
		}
	}

	// Verify 1 resource event in team-a namespace for namespaced Rules.
	var nsEvents eventsv1.EventList
	if err := kubeClient.List(t.Context(), &nsEvents, client.InNamespace("team-a")); err != nil {
		t.Fatalf("list namespace events: %v", err)
	}
	if len(nsEvents.Items) != 1 {
		t.Fatalf("expected 1 resource event in team-a, got %d", len(nsEvents.Items))
	}
	nsEv := nsEvents.Items[0]
	if nsEv.Type != corev1.EventTypeWarning {
		t.Errorf("expected Warning event on namespaced event, got %s", nsEv.Type)
	}
	if nsEv.Action != "Ignored" {
		t.Errorf("expected Action Ignored on namespaced event, got %s", nsEv.Action)
	}
	if nsEv.Reason != reasonRuleConfigError {
		t.Errorf("expected Reason %s on namespaced event, got %s", reasonRuleConfigError, nsEv.Reason)
	}
	if nsEv.ReportingController != reportingController {
		t.Errorf("expected ReportingController %s on namespaced event, got %s", reportingController, nsEv.ReportingController)
	}
	if nsEv.ReportingInstance != NameOperator {
		t.Errorf("expected ReportingInstance %s on namespaced event, got %s", NameOperator, nsEv.ReportingInstance)
	}
	if nsEv.EventTime.IsZero() {
		t.Errorf("expected non-zero EventTime on namespaced event: %+v", nsEv)
	}
	if !strings.Contains(nsEv.Note, `group "api-alerts"`) {
		t.Errorf("expected namespaced event Note to mention group \"api-alerts\", got %q", nsEv.Note)
	}
	if nsEv.Regarding.Kind != "Rules" || nsEv.Regarding.Name != "bad-rules" || nsEv.Regarding.Namespace != "team-a" {
		t.Errorf("unexpected Regarding on namespaced event: %+v", nsEv.Regarding)
	}
	if nsEv.Related != nil {
		t.Errorf("expected nil Related on direct resource event, got %+v", nsEv.Related)
	}

	// Re-reconciling with the same generation should not create additional events.
	if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
		t.Fatalf("second ensureRuleConfigs failed: %v", err)
	}
	if err := kubeClient.List(t.Context(), &opEvents, client.InNamespace(DefaultOperatorNamespace)); err != nil {
		t.Fatalf("list operator events after second reconcile: %v", err)
	}
	if len(opEvents.Items) != 3 {
		t.Errorf("expected still 3 workload events after second reconcile, got %d", len(opEvents.Items))
	}

	// Bumping generation on an already-invalid Rules resource should update ObservedGeneration and emit new events.
	var gotRules monitoringv1.Rules
	if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(namespacedRules), &gotRules); err != nil {
		t.Fatalf("get Rules before generation bump: %v", err)
	}
	gotRules.Generation = 2
	if err := kubeClient.Update(t.Context(), &gotRules); err != nil {
		t.Fatalf("update Rules generation: %v", err)
	}
	if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
		t.Fatalf("third ensureRuleConfigs after generation bump failed: %v", err)
	}
	if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(namespacedRules), &gotRules); err != nil {
		t.Fatalf("get Rules after generation bump: %v", err)
	}
	if gotRules.Status.ObservedGeneration != 2 {
		t.Errorf("expected ObservedGeneration 2 after generation bump, got %d", gotRules.Status.ObservedGeneration)
	}
	if err := kubeClient.List(t.Context(), &opEvents, client.InNamespace(DefaultOperatorNamespace)); err != nil {
		t.Fatalf("list operator events after generation bump: %v", err)
	}
	if len(opEvents.Items) != 4 {
		t.Errorf("expected 4 workload events after generation bump, got %d", len(opEvents.Items))
	}
	if err := kubeClient.List(t.Context(), &nsEvents, client.InNamespace("team-a")); err != nil {
		t.Fatalf("list namespace events after generation bump: %v", err)
	}
	if len(nsEvents.Items) != 2 {
		t.Errorf("expected 2 resource events in team-a after generation bump, got %d", len(nsEvents.Items))
	}
}

func TestRulesReconcilerValidationAndEventEdgeCases(t *testing.T) {
	t.Run("scope conflict and alert template errors", func(t *testing.T) {
		scopeConflictRules := &monitoringv1.Rules{
			Name:       "scope-conflict-rules",
			Namespace:  "team-a",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "scope-group",
						Rules: []monitoringv1.Rule{
							{
								Record: "job:up:sum",
								Expr:   `up{namespace="team-b"}`,
							},
							{
								Record: "job:up:label_conflict",
								Expr:   "vector(1)",
								Labels: map[string]string{
									"namespace": "team-b",
								},
							},
						},
					},
				},
			},
		}
		scopeConflictClusterRules := &monitoringv1.ClusterRules{
			Name:       "scope-conflict-cluster-rules",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "cluster-scope-group",
						Rules: []monitoringv1.Rule{
							{
								Record: "cluster:up:sum",
								Expr:   `up{cluster="other-cluster"}`,
							},
						},
					},
				},
			},
		}
		templateErrorRules := &monitoringv1.Rules{
			Name:       "template-error-rules",
			Namespace:  "team-a",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "alert-template-group",
						Rules: []monitoringv1.Rule{
							{
								Alert: "HighErrorRate",
								Expr:  "up == 0",
								Annotations: map[string]string{
									"summary": "{{ $labels.instance }",
								},
							},
						},
					},
				},
			},
		}

		kubeClient := newFakeClientBuilder().
			WithObjects(scopeConflictRules, scopeConflictClusterRules, templateErrorRules).
			Build()
		r := rulesReconciler{
			client: kubeClient,
			opts:   Options{OperatorNamespace: DefaultOperatorNamespace},
		}

		if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
			t.Fatalf("ensureRuleConfigs failed: %v", err)
		}

		var gotScopeRules monitoringv1.Rules
		if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(scopeConflictRules), &gotScopeRules); err != nil {
			t.Fatalf("get scopeConflictRules: %v", err)
		}
		if len(gotScopeRules.Status.Conditions) != 1 ||
			!strings.Contains(gotScopeRules.Status.Conditions[0].Message, `group "scope-group" rule #1: set isolation selector namespace="team-a" on up{namespace="team-b"}: conflicting label matcher`) ||
			!strings.Contains(gotScopeRules.Status.Conditions[0].Message, `group "scope-group" rule #2: set result isolation label namespace="team-a"`) {
			t.Errorf("unexpected status on scopeConflictRules: %+v", gotScopeRules.Status.Conditions)
		}

		var gotClusterRules monitoringv1.ClusterRules
		if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(scopeConflictClusterRules), &gotClusterRules); err != nil {
			t.Fatalf("get scopeConflictClusterRules: %v", err)
		}
		if len(gotClusterRules.Status.Conditions) != 1 || !strings.Contains(gotClusterRules.Status.Conditions[0].Message, `group "cluster-scope-group" rule #1`) || !strings.Contains(gotClusterRules.Status.Conditions[0].Message, "conflicting label matcher") {
			t.Errorf("unexpected status on scopeConflictClusterRules: %+v", gotClusterRules.Status.Conditions)
		}

		var gotTemplateRules monitoringv1.Rules
		if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(templateErrorRules), &gotTemplateRules); err != nil {
			t.Fatalf("get templateErrorRules: %v", err)
		}
		if len(gotTemplateRules.Status.Conditions) != 1 || !strings.Contains(gotTemplateRules.Status.Conditions[0].Message, "__alert_HighErrorRate") {
			t.Errorf("unexpected status on templateErrorRules: %+v", gotTemplateRules.Status.Conditions)
		}
	})

	t.Run("multi-error aggregation and ConfigMap isolation", func(t *testing.T) {
		multiErrRules := &monitoringv1.Rules{
			Name:       "multi-err-rules",
			Namespace:  "team-a",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "g1",
						Rules: []monitoringv1.Rule{
							{
								Record: "r1",
								Expr:   `up{namespace="team-b"}`,
							},
						},
					},
					{
						Name: "g2",
						Rules: []monitoringv1.Rule{
							{
								Record: "r2_ok",
								Expr:   "vector(1)",
							},
							{
								Record: "r2_bad",
								Expr:   `up{cluster="other-cluster"}`,
							},
						},
					},
				},
			},
		}
		validRules := &monitoringv1.Rules{
			Name:       "valid-rules",
			Namespace:  "team-a",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "valid-group",
						Rules: []monitoringv1.Rule{
							{
								Record: "valid_metric",
								Expr:   "vector(1)",
							},
						},
					},
				},
			},
		}
		validClusterRules := &monitoringv1.ClusterRules{
			Name:       "valid-cluster-rules",
			Generation: 1,
			Spec:       validRules.Spec,
		}
		validGlobalRules := &monitoringv1.GlobalRules{
			Name:       "valid-global-rules",
			Generation: 1,
			Spec:       validRules.Spec,
		}

		kubeClient := newFakeClientBuilder().
			WithObjects(multiErrRules, validRules, validClusterRules, validGlobalRules).
			Build()
		r := rulesReconciler{
			client: kubeClient,
			opts:   Options{OperatorNamespace: DefaultOperatorNamespace},
		}

		if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
			t.Fatalf("ensureRuleConfigs failed: %v", err)
		}

		var gotMulti monitoringv1.Rules
		if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(multiErrRules), &gotMulti); err != nil {
			t.Fatalf("get multiErrRules: %v", err)
		}
		msg := gotMulti.Status.Conditions[0].Message
		if !strings.Contains(msg, `group "g1" rule #1`) || !strings.Contains(msg, `group "g2" rule #2`) {
			t.Errorf("expected aggregated message to contain both group/rule indices, got %q", msg)
		}

		var cm corev1.ConfigMap
		if err := kubeClient.Get(t.Context(), client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: nameRulesGenerated}, &cm); err != nil {
			t.Fatalf("get rules-generated ConfigMap: %v", err)
		}
		for _, wantKey := range []string{
			"rules__team-a__valid-rules.yaml",
			"clusterrules__valid-cluster-rules.yaml",
			"globalrules__valid-global-rules.yaml",
		} {
			if _, ok := cm.Data[wantKey]; !ok {
				t.Errorf("expected valid key %q in rules-generated ConfigMap, got keys %v", wantKey, cm.Data)
			}
		}
		if _, ok := cm.Data["rules__team-a__multi-err-rules.yaml"]; ok {
			t.Error("expected invalid rules__team-a__multi-err-rules.yaml to be excluded from rules-generated ConfigMap")
		}
	})

	t.Run("event Note truncation at 1024 bytes", func(t *testing.T) {
		longGroup := strings.Repeat("long-group-name-", 70)
		longErrRules := &monitoringv1.Rules{
			Name:       "long-err-rules",
			Namespace:  "team-a",
			Generation: 1,
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: longGroup,
						Rules: []monitoringv1.Rule{
							{
								Record: "test_record",
								Expr:   "invalid===",
							},
						},
					},
				},
			},
		}

		kubeClient := newFakeClientBuilder().WithObjects(longErrRules).Build()
		r := rulesReconciler{
			client: kubeClient,
			opts:   Options{OperatorNamespace: DefaultOperatorNamespace},
		}

		if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
			t.Fatalf("ensureRuleConfigs failed: %v", err)
		}

		var gotRules monitoringv1.Rules
		if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(longErrRules), &gotRules); err != nil {
			t.Fatalf("get longErrRules: %v", err)
		}
		if len(gotRules.Status.Conditions[0].Message) <= maxEventNoteLength {
			t.Fatalf("expected full status condition Message length > %d, got %d", maxEventNoteLength, len(gotRules.Status.Conditions[0].Message))
		}

		var evs eventsv1.EventList
		if err := kubeClient.List(t.Context(), &evs, client.InNamespace(DefaultOperatorNamespace)); err != nil {
			t.Fatalf("list events: %v", err)
		}
		if len(evs.Items) != 1 {
			t.Fatalf("expected 1 workload event, got %d", len(evs.Items))
		}
		if len(evs.Items[0].Note) != maxEventNoteLength {
			t.Errorf("expected truncated event Note length %d, got %d", maxEventNoteLength, len(evs.Items[0].Note))
		}
		if !strings.HasSuffix(evs.Items[0].Note, "...") {
			t.Errorf("expected truncated event Note to end with '...', got suffix %q", evs.Items[0].Note[len(evs.Items[0].Note)-10:])
		}
	})

	t.Run("event name collision resistance across same-named resources", func(t *testing.T) {
		badSpec := monitoringv1.RulesSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "g1",
					Rules: []monitoringv1.Rule{
						{
							Record: "test_record",
							Expr:   "invalid===",
						},
					},
				},
			},
		}
		objs := []client.Object{
			&monitoringv1.Rules{Name: "shared-name", Namespace: "team-a", Generation: 1, Spec: badSpec},
			&monitoringv1.Rules{Name: "shared-name", Namespace: "team-b", Generation: 1, Spec: badSpec},
			&monitoringv1.Rules{Name: "shared-name", Namespace: DefaultOperatorNamespace, Generation: 1, Spec: badSpec},
			&monitoringv1.ClusterRules{Name: "shared-name", Generation: 1, Spec: badSpec},
			&monitoringv1.GlobalRules{Name: "shared-name", Generation: 1, Spec: badSpec},
		}

		kubeClient := newFakeClientBuilder().WithObjects(objs...).Build()
		r := rulesReconciler{
			client: kubeClient,
			opts:   Options{OperatorNamespace: DefaultOperatorNamespace},
		}

		if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
			t.Fatalf("ensureRuleConfigs failed: %v", err)
		}

		// In gmp-system, expect 5 workload events (one per resource) + 1 direct resource event (for Rules in gmp-system) = 6 events.
		var opEvents eventsv1.EventList
		if err := kubeClient.List(t.Context(), &opEvents, client.InNamespace(DefaultOperatorNamespace)); err != nil {
			t.Fatalf("list gmp-system events: %v", err)
		}
		if len(opEvents.Items) != 6 {
			t.Errorf("expected 6 distinct events in %s without name collisions, got %d", DefaultOperatorNamespace, len(opEvents.Items))
		}
	})

	t.Run("event Note UTF-8 rune boundary truncation", func(t *testing.T) {
		// Place a 3-byte rune ('€', 0xE2 0x82 0xAC) starting at byte 1020 so byte 1021
		// lands in the middle of the multi-byte character.
		note := strings.Repeat("a", maxEventNoteLength-4) + "€" + strings.Repeat("b", 10)
		got := truncateEventNote(note)
		if len(got) > maxEventNoteLength {
			t.Errorf("expected truncated Note length <= %d, got %d", maxEventNoteLength, len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("expected truncated Note to be valid UTF-8, got %q", got)
		}
		if !strings.HasSuffix(got, "...") {
			t.Errorf("expected truncated Note to end with '...', got %q", got[len(got)-6:])
		}
		// Because '€' at index 1020 cannot fit before index 1021, cut backs up to 1020.
		if len(got) != maxEventNoteLength-1 {
			t.Errorf("expected truncated Note length %d, got %d", maxEventNoteLength-1, len(got))
		}
	})

	t.Run("long resource name event Name length cap and prefix collision resistance", func(t *testing.T) {
		// Build two 253-character resource names sharing the same 140-character prefix,
		// with a hyphen at index maxEventObjNameLength-10 to test both length capping,
		// trailing separator trimming, and FNV-1a hash disambiguation.
		sharedPrefix := strings.Repeat("a", maxEventObjNameLength-10) + "-" + strings.Repeat("b", 9)
		longName1 := sharedPrefix + strings.Repeat("b", 253-len(sharedPrefix))
		longName2 := sharedPrefix + strings.Repeat("c", 253-len(sharedPrefix))
		longNamespace := strings.Repeat("n", 63)
		badSpec := monitoringv1.RulesSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "g1",
					Rules: []monitoringv1.Rule{
						{
							Record: "test_record",
							Expr:   "invalid===",
						},
					},
				},
			},
		}
		badRules1 := &monitoringv1.Rules{
			Name:       longName1,
			Namespace:  longNamespace,
			Generation: 1,
			Spec:       badSpec,
		}
		badRules2 := &monitoringv1.Rules{
			Name:       longName2,
			Namespace:  longNamespace,
			Generation: 1,
			Spec:       badSpec,
		}

		kubeClient := newFakeClientBuilder().WithObjects(badRules1, badRules2).Build()
		r := rulesReconciler{
			client: kubeClient,
			opts:   Options{OperatorNamespace: DefaultOperatorNamespace},
		}

		if err := r.ensureRuleConfigs(t.Context(), "test-proj", "us-central1", "test-cluster", monitoringv1.CompressionNone); err != nil {
			t.Fatalf("ensureRuleConfigs failed: %v", err)
		}

		for _, ns := range []string{DefaultOperatorNamespace, longNamespace} {
			var evs eventsv1.EventList
			if err := kubeClient.List(t.Context(), &evs, client.InNamespace(ns)); err != nil {
				t.Fatalf("list events in %s: %v", ns, err)
			}
			if len(evs.Items) != 2 {
				t.Fatalf("expected 2 distinct events in %s without prefix collision, got %d", ns, len(evs.Items))
			}
			for _, ev := range evs.Items {
				evName := ev.Name
				if len(evName) > 253 {
					t.Errorf("expected event Name in %s to be <= 253 chars, got %d (%q)", ns, len(evName), evName)
				}
				if strings.Contains(evName, "--") || strings.Contains(evName, "-.") || strings.Contains(evName, "..") {
					t.Errorf("expected event Name in %s to have valid RFC 1123 labels without double hyphens, got %q", ns, evName)
				}
			}
		}
	})
}

func TestScaleRuleConsumers(t *testing.T) {
	var alertmanagerReplicas int32
	alertManager := appsv1.StatefulSet{
		Name: "alertmanager",
		Spec: appsv1.StatefulSetSpec{
			Replicas: &alertmanagerReplicas,
		},
	}
	var ruleEvaluatorReplicas int32
	ruleEvaluator := appsv1.Deployment{
		Name: "rule-evaluator",
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
				client: &fakeClientWithScale{tc.client},
			}
			err := r.scaleRuleConsumers(t.Context())
			if err != nil {
				if !tc.wantErr {
					t.Errorf("Unexpected error: %s", err)
				}
				return
			}

			var alertmanager appsv1.StatefulSet
			if err := r.client.Get(t.Context(), client.ObjectKey{Name: "alertmanager"}, &alertmanager); client.IgnoreNotFound(err) != nil {
				t.Error(err)
			}
			if alertmanager.Spec.Replicas != nil && *alertmanager.Spec.Replicas != tc.want {
				t.Errorf("want: %d, got: %d", tc.want, *alertmanager.Spec.Replicas)
			}

			var ruleEvaluator appsv1.Deployment
			if err := r.client.Get(t.Context(), client.ObjectKey{Name: "rule-evaluator"}, &ruleEvaluator); client.IgnoreNotFound(err) != nil {
				t.Error(err)
			}
			if ruleEvaluator.Spec.Replicas != nil && *ruleEvaluator.Spec.Replicas != tc.want {
				t.Errorf("want: %d, got: %d", tc.want, *ruleEvaluator.Spec.Replicas)
			}
		})
	}
}

// TODO: Remove after https://github.com/kubernetes-sigs/controller-runtime/pull/2855
type fakeClientWithScale struct {
	client.Client
}

func (c *fakeClientWithScale) SubResource(subResource string) client.SubResourceClient {
	if subResource == "scale" {
		return &fakeScaleClient{client: c.Client}
	}
	return c.Client.SubResource(subResource)
}

type fakeScaleClient struct {
	client client.Client
}

func (c *fakeScaleClient) Get(ctx context.Context, obj, subResource client.Object, _ ...client.SubResourceGetOption) error {
	scale, isScale := subResource.(*autoscalingv1.Scale)
	if !isScale {
		return apierrors.NewBadRequest(fmt.Sprintf("got invalid type %t, expected Scale", subResource))
	}
	if err := c.client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return err
	}
	scaleOut, err := extractScale(obj)
	if err != nil {
		return err
	}
	*scale = scaleOut
	return nil
}

func (c *fakeScaleClient) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return errors.New("fakeSubResourceWriter does not support create")
}

func (c *fakeScaleClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	updateOptions := client.SubResourceUpdateOptions{}
	updateOptions.ApplyOptions(opts)

	body := obj
	if updateOptions.SubResourceBody == nil {
		return apierrors.NewBadRequest("expected SubResourceBody")
	}
	scale, isScale := updateOptions.SubResourceBody.(*autoscalingv1.Scale)
	if !isScale {
		return apierrors.NewBadRequest(fmt.Sprintf("got invalid type %t, expected Scale", updateOptions.SubResourceBody))
	}
	if err := c.client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return err
	}
	if err := applyScale(obj, scale); err != nil {
		return err
	}
	return c.client.Update(ctx, body, &updateOptions.UpdateOptions)
}

func (c *fakeScaleClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	patchOptions := client.SubResourcePatchOptions{}
	patchOptions.ApplyOptions(opts)

	body := obj
	if patchOptions.SubResourceBody != nil {
		body = patchOptions.SubResourceBody
	}

	return c.client.Patch(ctx, body, patch, &patchOptions.PatchOptions)
}

func (c *fakeScaleClient) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.SubResourceApplyOption) error {
	return errors.New("fakeScaleClient does not support apply")
}

func extractScale(obj client.Object) (autoscalingv1.Scale, error) {
	switch obj := obj.(type) {
	case *appsv1.Deployment:
		replicas := int32(1)
		if obj.Spec.Replicas != nil {
			replicas = *obj.Spec.Replicas
		}
		return autoscalingv1.Scale{
			ObjectMeta: obj.ObjectMeta,
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: obj.Status.Replicas,
				Selector: obj.Spec.Selector.String(),
			},
		}, nil
	case *appsv1.ReplicaSet:
		replicas := int32(1)
		if obj.Spec.Replicas != nil {
			replicas = *obj.Spec.Replicas
		}
		return autoscalingv1.Scale{
			ObjectMeta: obj.ObjectMeta,
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: obj.Status.Replicas,
				Selector: obj.Spec.Selector.String(),
			},
		}, nil
	case *corev1.ReplicationController:
		replicas := int32(1)
		if obj.Spec.Replicas != nil {
			replicas = *obj.Spec.Replicas
		}
		return autoscalingv1.Scale{
			ObjectMeta: obj.ObjectMeta,
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: obj.Status.Replicas,
				Selector: labels.Set(obj.Spec.Selector).String(),
			},
		}, nil
	case *appsv1.StatefulSet:
		replicas := int32(1)
		if obj.Spec.Replicas != nil {
			replicas = *obj.Spec.Replicas
		}
		return autoscalingv1.Scale{
			ObjectMeta: obj.ObjectMeta,
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: obj.Status.Replicas,
				Selector: obj.Spec.Selector.String(),
			},
		}, nil
	default:
		// TODO: CRDs https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#scale-subresource
		return autoscalingv1.Scale{}, fmt.Errorf("unable to extract scale from type %T", obj)
	}
}

func applyScale(obj client.Object, scale *autoscalingv1.Scale) error {
	switch obj := obj.(type) {
	case *appsv1.Deployment:
		obj.Spec.Replicas = &scale.Spec.Replicas
	case *appsv1.ReplicaSet:
		obj.Spec.Replicas = &scale.Spec.Replicas
	case *corev1.ReplicationController:
		obj.Spec.Replicas = &scale.Spec.Replicas
	case *appsv1.StatefulSet:
		obj.Spec.Replicas = &scale.Spec.Replicas
	default:
		// TODO: CRDs https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#scale-subresource
		return fmt.Errorf("unable to extract scale from type %T", obj)
	}
	return nil
}
