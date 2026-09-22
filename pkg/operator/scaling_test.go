// Copyright 2024 Google LLC
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

package operator

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestApplyVPA(t *testing.T) {
	alertmanagerVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: alertmanagerVPAName,
		},
	}
	collectorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectorVPAName,
		},
	}
	operatorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorVPAName,
		},
	}
	ruleEvaluatorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: ruleEvaluatorVPAName,
		},
	}

	scheme, err := NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	createInterceptorWithError := interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return errors.New("error other than not found")
		},
	}

	type test struct {
		c       client.Client
		wantErr bool
	}

	tests := map[string]test{
		"create": {
			c: fake.NewClientBuilder().WithScheme(scheme).Build(),
		},
		"create with error": {
			c:       fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(createInterceptorWithError).Build(),
			wantErr: true,
		},
		"update": {
			c: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&alertmanagerVPA, &collectorVPA, &operatorVPA, &ruleEvaluatorVPA).Build(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := applyVPA(t.Context(), tc.c, "")
			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.wantErr:
				t.Error("expected error, but got no error")
			case err != nil && tc.wantErr:
				// Ok.
			case err == nil && !tc.wantErr:
				expectedTargets := map[string]struct {
					kind          string
					containerName string
				}{
					alertmanagerVPAName:  {kind: "StatefulSet", containerName: "alertmanager"},
					collectorVPAName:     {kind: "DaemonSet", containerName: "prometheus"},
					operatorVPAName:      {kind: "Deployment", containerName: "operator"},
					ruleEvaluatorVPAName: {kind: "Deployment", containerName: "evaluator"},
				}
				for vpaName, want := range expectedTargets {
					var got autoscalingv1.VerticalPodAutoscaler
					if err := tc.c.Get(t.Context(), client.ObjectKey{Name: vpaName}, &got); err != nil {
						t.Errorf("get VPA %q: %v", vpaName, err)
						continue
					}
					if got.Spec.TargetRef == nil {
						t.Errorf("VPA %q has nil Spec.TargetRef", vpaName)
						continue
					}
					if got.Spec.TargetRef.Kind != want.kind || got.Spec.TargetRef.Name != vpaName {
						t.Errorf("VPA %q TargetRef = %+v, want Kind=%q Name=%q", vpaName, got.Spec.TargetRef, want.kind, vpaName)
					}
					if got.Spec.ResourcePolicy == nil || len(got.Spec.ResourcePolicy.ContainerPolicies) == 0 {
						t.Errorf("VPA %q has empty ResourcePolicy.ContainerPolicies", vpaName)
						continue
					}
					if got.Spec.ResourcePolicy.ContainerPolicies[0].ContainerName != want.containerName {
						t.Errorf("VPA %q first ContainerName = %q, want %q", vpaName, got.Spec.ResourcePolicy.ContainerPolicies[0].ContainerName, want.containerName)
					}
				}
			default:
				// Ok.
			}
		})
	}
}

func TestDeleteVPA(t *testing.T) {
	alertmanagerVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: alertmanagerVPAName,
		},
	}
	collectorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectorVPAName,
		},
	}
	operatorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorVPAName,
		},
	}
	ruleEvaluatorVPA := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: ruleEvaluatorVPAName,
		},
	}

	scheme, err := NewScheme()
	if err != nil {
		t.Fatal(err)
	}

	deleteInterceptorWithError := interceptor.Funcs{
		Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
			return errors.New("error other than not found")
		},
	}
	deleteInterceptorWithNotFoundError := interceptor.Funcs{
		Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
			return apierrors.NewNotFound(autoscalingv1.Resource("verticalpodautoscaler"), collectorVPAName)
		},
	}

	type test struct {
		c       client.Client
		wantErr bool
	}

	tests := map[string]test{
		"not found": {
			c: fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(deleteInterceptorWithNotFoundError).Build(),
		},
		"ok": {
			c: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&alertmanagerVPA, &collectorVPA, &operatorVPA, &ruleEvaluatorVPA).Build(),
		},
		"err": {
			c:       fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(deleteInterceptorWithError).Build(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := deleteVPA(t.Context(), tc.c, "")
			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.wantErr:
				t.Error("expected error, but got no error")
			case err != nil && tc.wantErr:
				// Ok.
			case err == nil && !tc.wantErr:
				if err := tc.c.Get(t.Context(), client.ObjectKey{Name: alertmanagerVPAName}, &autoscalingv1.VerticalPodAutoscaler{}); !apierrors.IsNotFound(err) {
					t.Errorf("expected not found, got %s", err)
				}
				if err := tc.c.Get(t.Context(), client.ObjectKey{Name: collectorVPAName}, &autoscalingv1.VerticalPodAutoscaler{}); !apierrors.IsNotFound(err) {
					t.Errorf("expected not found, got %s", err)
				}
				if err := tc.c.Get(t.Context(), client.ObjectKey{Name: operatorVPAName}, &autoscalingv1.VerticalPodAutoscaler{}); !apierrors.IsNotFound(err) {
					t.Errorf("expected not found, got %s", err)
				}
				if err := tc.c.Get(t.Context(), client.ObjectKey{Name: ruleEvaluatorVPAName}, &autoscalingv1.VerticalPodAutoscaler{}); !apierrors.IsNotFound(err) {
					t.Errorf("expected not found, got %s", err)
				}
			default:
				// Ok.
			}
		})
	}
}
