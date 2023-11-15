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
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
			ctx := context.Background()
			ds := &appsv1.DaemonSet{
				ObjectMeta: v1.ObjectMeta{
					Name:        NameCollector,
					Namespace:   "gmp-system",
					Annotations: c.collectorAnnots,
				},
			}

			deploy := &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:        NameRuleEvaluator,
					Namespace:   "gmp-system",
					Annotations: c.evaluatorAnnots,
				},
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
			if err := op.cleanupOldResources(ctx); err != nil {
				t.Fatal(err)
			}

			// Check if collector DaemonSet was preserved.
			var gotDS appsv1.DaemonSet
			dsErr := cl.Get(ctx, client.ObjectKey{
				Name:      NameCollector,
				Namespace: "gmp-system",
			}, &gotDS)
			if c.collectorDeleted {
				if !apierrors.IsNotFound(dsErr) {
					t.Errorf("collector should be deleted but found: %+v", gotDS)
				}
			} else if gotDS.Name != ds.Name || gotDS.Namespace != ds.Namespace {
				t.Errorf("collector DaemonSet differs")
			}

			// Check if rule-evaluator Deployment was preserved.
			var gotDeploy appsv1.Deployment
			deployErr := cl.Get(ctx, client.ObjectKey{
				Name:      NameRuleEvaluator,
				Namespace: "gmp-system",
			}, &gotDeploy)
			if c.evaluatorDeleted {
				if !apierrors.IsNotFound(deployErr) {
					t.Errorf("rule-evaluator should be deleted but found: %+v", gotDeploy)
				}
			} else if gotDeploy.Name != deploy.Name || gotDeploy.Namespace != deploy.Namespace {
				t.Errorf("rule-evaluator Deployment differs")
			}
		})
	}
}
