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
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Tests that the collection does not overwrite the non-managed status fields.
func TestCollectionStatus(t *testing.T) {
	statusIn := monitoringv1.PodMonitoringStatus{
		EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
			{
				Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
				ActiveTargets:    1,
				UnhealthyTargets: 0,
				LastUpdateTime:   metav1.Date(2022, time.November, 1, 0, 0, 0, 0, time.UTC),
				SampleGroups: []monitoringv1.SampleGroup{
					{
						SampleTargets: []monitoringv1.SampleTarget{
							{
								Health: "up",
								Labels: map[model.LabelName]model.LabelValue{
									"instance": "a",
								},
								LastScrapeDurationSeconds: "1.2",
							},
						},
						Count: pointer.Int32(1),
					},
				},
				CollectorsFraction: "1",
			},
		},
	}

	statusOut := *statusIn.DeepCopy()
	statusOut.Conditions = append(statusOut.Conditions, monitoringv1.MonitoringCondition{
		Type:               monitoringv1.ConfigurationCreateSuccess,
		Status:             corev1.ConditionTrue,
		LastUpdateTime:     metav1.Time{},
		LastTransitionTime: metav1.Time{},
		Reason:             "",
		Message:            "",
	})

	scheme, err := NewScheme()
	if err != nil {
		t.Fatal("Unable to get scheme")
	}

	logger := testr.New(t)
	ctx := logr.NewContext(context.Background(), logger)
	opts := Options{
		ProjectID: "test-proj",
		Location:  "test-loc",
		Cluster:   "test-cluster",
	}
	if err := opts.defaultAndValidate(logger); err != nil {
		t.Fatal("Invalid options:", err)
	}

	kubeClient := fake.
		NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&monitoringv1.PodMonitoring{
			ObjectMeta: v1.ObjectMeta{
				Name:      "prom-example",
				Namespace: "gmp-test",
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Endpoints: []monitoringv1.ScrapeEndpoint{{
					Port:     intstr.FromString("metrics"),
					Interval: "10s",
				}},
			},
			Status: statusIn,
		}).
		WithObjects(&monitoringv1.OperatorConfig{
			ObjectMeta: v1.ObjectMeta{
				Name:      NameOperatorConfig,
				Namespace: opts.PublicNamespace,
			},
		}).
		WithObjects(&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NameCollector,
				Namespace: opts.OperatorNamespace,
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "prometheus",
						}},
					},
				},
			},
		}).
		Build()

	collectionReconciler := newCollectionReconciler(kubeClient, opts)
	collectionReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: opts.PublicNamespace,
			Name:      NameOperatorConfig,
		},
	})

	var podMonitorings monitoringv1.PodMonitoringList
	kubeClient.List(ctx, &podMonitorings)
	switch amount := len(podMonitorings.Items); amount {
	case 1:
		status := podMonitorings.Items[0].Status
		for i := range status.Conditions {
			// Normalize times because we cannot predict this.
			condition := &status.Conditions[i]
			condition.LastUpdateTime = v1.Time{}
			condition.LastTransitionTime = v1.Time{}
		}
		if diff := cmp.Diff(status, statusOut); diff != "" {
			t.Fatalf("invalid PodMonitoringStatus: %s", diff)
		}
		break
	default:
		t.Fatalf("invalid PodMonitorings found: %d", amount)
	}
}
