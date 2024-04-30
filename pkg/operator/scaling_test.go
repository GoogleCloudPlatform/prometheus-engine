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
	vpa := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectorVPAName,
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
			c: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&vpa).Build(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := applyVPA(context.TODO(), tc.c, "")
			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.wantErr:
				t.Errorf("expected error, but got no error")
			case err != nil && tc.wantErr:
				// Ok
			case err == nil && !tc.wantErr:
				// Ok
			}
		})
	}
}

func TestDeleteVPA(t *testing.T) {
	vpa := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectorVPAName,
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
			c: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(&vpa).Build(),
		},
		"err": {
			c:       fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(deleteInterceptorWithError).Build(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := deleteVPA(context.TODO(), tc.c, "")
			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.wantErr:
				t.Errorf("expected error, but got no error")
			case err != nil && tc.wantErr:
				// Ok
			case err == nil && !tc.wantErr:
				// Ok
			}
		})
	}
}
