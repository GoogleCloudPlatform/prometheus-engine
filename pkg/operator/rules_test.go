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
	"testing"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
							Message:            "generating rule config failed",
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
						MonitoringStatus: monitoringv1.MonitoringStatus{
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
							Message:            "generating rule config failed",
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
						MonitoringStatus: monitoringv1.MonitoringStatus{
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
							Message:            "generating rule config failed",
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
						MonitoringStatus: monitoringv1.MonitoringStatus{
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
						MonitoringStatus: monitoringv1.MonitoringStatus{
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
			// The message is good enough. Don't need reason.
			condition.Reason = ""

			if diff := cmp.Diff(&tc.expectedStatus, status); diff != "" {
				t.Errorf("expected %q condition (-want, +got): %s", objectKey, diff)
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
				t.Errorf("want: %d, got: %d", tc.want, ruleEvaluator.Spec.Replicas)
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

// flakyClient simulates a client that fails the first update/create, then succeeds.
type flakyClient struct {
	client.Client
	failOnce map[string]bool
}

// Update wraps the client Update to fail once per ConfigMap name
func (fc *flakyClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	cm, ok := obj.(*corev1.ConfigMap)
	if ok && !fc.failOnce[cm.Name] {
		fc.failOnce[cm.Name] = true
		return fmt.Errorf("simulated update failure for %s", cm.Name)
	}
	return fc.Client.Update(ctx, obj, opts...)
}

// Create wraps the client Create to fail once per ConfigMap name
func (fc *flakyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	cm, ok := obj.(*corev1.ConfigMap)
	if ok && !fc.failOnce[cm.Name] {
		fc.failOnce[cm.Name] = true
		return fmt.Errorf("simulated create failure for %s", cm.Name)
	}
	return fc.Client.Create(ctx, obj, opts...)
}

func TestEnsureRuleConfigs_SplitConfigMaps(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = monitoringv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create two rules to verify they go into single "rules" ConfigMap
	rule1 := &monitoringv1.Rules{
		TypeMeta:   metav1.TypeMeta{APIVersion: "monitoring.googleapis.com/v1", Kind: "Rules"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "r1"},
		Spec: monitoringv1.RulesSpec{
			Groups: []monitoringv1.RuleGroup{{
				Name:  "g1",
				Rules: []monitoringv1.Rule{{Record: "r", Expr: "vector(1)"}},
			}},
		},
	}
	rule2 := &monitoringv1.Rules{
		TypeMeta:   metav1.TypeMeta{APIVersion: "monitoring.googleapis.com/v1", Kind: "Rules"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "r2"},
		Spec: monitoringv1.RulesSpec{
			Groups: []monitoringv1.RuleGroup{{
				Name:  "g2",
				Rules: []monitoringv1.Rule{{Record: "r2", Expr: "vector(2)"}},
			}},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rule1, rule2).Build()
	reconciler := &rulesReconciler{
		client: c,
		opts:   Options{OperatorNamespace: "gmp-system"},
	}
	err := reconciler.ensureRuleConfigs(t.Context(), "proj", "loc", "cluster", monitoringv1.CompressionNone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cmList corev1.ConfigMapList
	if err := c.List(t.Context(), &cmList); err != nil {
		t.Fatalf("listing configmaps: %v", err)
	}

	// Should have exactly 3 ConfigMaps: rules, clusterrules, globalrules
	if len(cmList.Items) != 3 {
		t.Fatalf("expected 3 configmaps (rules, clusterrules, globalrules), got %d", len(cmList.Items))
	}

	// Find the "rules" ConfigMap
	var rulesConfigMap *corev1.ConfigMap
	for i := range cmList.Items {
		cm := &cmList.Items[i]
		if cm.Name == "rules" {
			rulesConfigMap = cm
			break
		}
	}

	if rulesConfigMap == nil {
		t.Fatalf("rules configmap not found")
	}

	// Verify both rule files are in the single "rules" ConfigMap
	found := map[string]bool{"rules__ns1__r1.yaml": false, "rules__ns2__r2.yaml": false}
	for key := range rulesConfigMap.Data {
		if _, ok := found[key]; ok {
			found[key] = true
		}
	}

	for k, v := range found {
		if !v {
			t.Errorf("expected rule file %s not found in rules configmap", k)
		}
	}
}

func TestEnsureRuleConfigs_InterruptionRecovery(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = monitoringv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	rule := &monitoringv1.Rules{
		TypeMeta:   metav1.TypeMeta{APIVersion: "monitoring.googleapis.com/v1", Kind: "Rules"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "r1"},
		Spec: monitoringv1.RulesSpec{
			Groups: []monitoringv1.RuleGroup{{
				Name:  "g1",
				Rules: []monitoringv1.Rule{{Record: "r", Expr: "vector(1)"}},
			}},
		},
	}

	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(rule).Build()
	fc := &flakyClient{Client: baseClient, failOnce: make(map[string]bool)}
	reconciler := &rulesReconciler{
		client: fc,
		opts:   Options{OperatorNamespace: "gmp-system"},
	}

	// First call: will fail once per ConfigMap, but retry and succeed
	err := reconciler.ensureRuleConfigs(t.Context(), "proj", "loc", "cluster", monitoringv1.CompressionNone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cmList corev1.ConfigMapList
	if err := fc.List(t.Context(), &cmList); err != nil {
		t.Fatalf("listing configmaps: %v", err)
	}
	// Should have 3 ConfigMaps: rules, clusterrules, globalrules
	if len(cmList.Items) != 3 {
		t.Fatalf("expected 3 configmaps (rules, clusterrules, globalrules), got %d", len(cmList.Items))
	}

	// Find the rules ConfigMap
	var rulesConfigMap *corev1.ConfigMap
	for i := range cmList.Items {
		cm := &cmList.Items[i]
		if cm.Name == "rules" {
			rulesConfigMap = cm
			break
		}
	}

	if rulesConfigMap == nil {
		t.Fatalf("rules configmap not found")
	}

	if _, ok := rulesConfigMap.Data["rules__ns1__r1.yaml"]; !ok {
		t.Errorf("expected rule file not found after recovery")
	}
}
