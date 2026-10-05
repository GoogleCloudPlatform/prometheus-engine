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

package operator

import (
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"

	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsVPAAvailable(t *testing.T) {
	vpaCRDGroupResource := schema.GroupResource{
		Group:    "apiextensions.k8s.io",
		Resource: "customresourcedefinitions",
	}
	vpaCRDName := "verticalpodautoscalers.autoscaling.k8s.io"

	cases := []struct {
		desc             string
		clientset        func() *apiextensionsfake.Clientset
		wantVPAAvailable bool
		wantErr          bool
	}{
		{
			desc: "VPA CRD available",
			clientset: func() *apiextensionsfake.Clientset {
				return apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
					Name: vpaCRDName,
				})
			},
			wantVPAAvailable: true,
		},
		{
			desc: "VPA CRD not found",
			clientset: func() *apiextensionsfake.Clientset {
				return apiextensionsfake.NewClientset()
			},
			wantVPAAvailable: false,
		},
		{
			desc: "VPA CRD forbidden",
			clientset: func() *apiextensionsfake.Clientset {
				cs := apiextensionsfake.NewClientset()
				cs.PrependReactor("get", "customresourcedefinitions", func(_ ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(vpaCRDGroupResource, vpaCRDName, errors.New("forbidden"))
				})
				return cs
			},
			wantVPAAvailable: false,
		},
		{
			desc: "transient internal server error",
			clientset: func() *apiextensionsfake.Clientset {
				cs := apiextensionsfake.NewClientset()
				cs.PrependReactor("get", "customresourcedefinitions", func(_ ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewInternalError(errors.New("apiserver unavailable"))
				})
				return cs
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := isVPAAvailable(t.Context(), testr.New(t), tc.clientset())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantVPAAvailable {
				t.Errorf("isVPAAvailable() = %v, want %v", got, tc.wantVPAAvailable)
			}
		})
	}
}

func TestCleanupOldResources(t *testing.T) {
	var cases = []struct {
		desc             string
		cleanupAnnotKey  string
		collectorAnnots  map[string]string
		evaluatorAnnots  map[string]string
		collectorDeleted bool
		evaluatorDeleted bool
	}{
		{
			desc:            "keep both",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: false,
		},
		{
			desc:            "delete both",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: true,
			evaluatorDeleted: true,
		},
		{
			desc:            "delete collector",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			collectorDeleted: true,
			evaluatorDeleted: false,
		},
		{
			desc:            "delete rule-evaluator",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: true,
		},
		{
			desc:            "keep both",
			cleanupAnnotKey: "",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: false,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			ds := &appsv1.DaemonSet{
				Name:        NameCollector,
				Namespace:   "gmp-system",
				Annotations: c.collectorAnnots,
			}

			deploy := &appsv1.Deployment{
				Name:        NameRuleEvaluator,
				Namespace:   "gmp-system",
				Annotations: c.evaluatorAnnots,
			}
			opts := Options{
				ProjectID:         "test-proj",
				Location:          "test-loc",
				Cluster:           "test-cluster",
				OperatorNamespace: "gmp-system",
				CleanupAnnotKey:   c.cleanupAnnotKey,
			}
			cl := fake.NewClientBuilder().WithObjects(ds, deploy).Build()

			op := &Operator{
				logger: testr.New(t),
				opts:   opts,
				client: cl,
			}
			if err := op.cleanupOldResources(t.Context()); err != nil {
				t.Fatal(err)
			}

			// Check if collector DaemonSet was preserved.
			var gotDS appsv1.DaemonSet
			dsErr := cl.Get(t.Context(), client.ObjectKey{
				Name:      NameCollector,
				Namespace: "gmp-system",
			}, &gotDS)
			if c.collectorDeleted {
				if !apierrors.IsNotFound(dsErr) {
					t.Errorf("collector should be deleted but found: %+v", gotDS)
				}
			} else if gotDS.Name != ds.Name || gotDS.Namespace != ds.Namespace {
				t.Error("collector DaemonSet differs")
			}

			// Check if rule-evaluator Deployment was preserved.
			var gotDeploy appsv1.Deployment
			deployErr := cl.Get(t.Context(), client.ObjectKey{
				Name:      NameRuleEvaluator,
				Namespace: "gmp-system",
			}, &gotDeploy)
			if c.evaluatorDeleted {
				if !apierrors.IsNotFound(deployErr) {
					t.Errorf("rule-evaluator should be deleted but found: %+v", gotDeploy)
				}
			} else if gotDeploy.Name != deploy.Name || gotDeploy.Namespace != deploy.Namespace {
				t.Error("rule-evaluator Deployment differs")
			}
		})
	}
}
