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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testScheme *runtime.Scheme

func TestMain(m *testing.M) {
	var err error
	testScheme, err = NewScheme()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to get scheme: %s", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func newFakeClientBuilder() *fake.ClientBuilder {
	return fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&monitoringv1.PodMonitoring{}).
		WithStatusSubresource(&monitoringv1.ClusterPodMonitoring{})
}

func TestLoop(t *testing.T) {
	var a []*int
	var one = 1
	var two = 2
	var three = 3
	a = append(a, &one)
	a = append(a, &two)
	a = append(a, &three)

	b := []int{1, 2, 3}

	for i, aa := range a {
		fmt.Printf("i: %+v, aa: %+v, val: %+v\n", i, aa, *aa)
	}
	for i, bb := range b {
		fmt.Printf("i: %+v, bb: %+v, val: %+v\n", i, bb, &bb)
	}
}

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
						Count: ptr.To(int32(1)),
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

	kubeClient := newFakeClientBuilder().
		WithObjects(&monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
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
			ObjectMeta: metav1.ObjectMeta{
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
	if _, err := collectionReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: opts.PublicNamespace,
			Name:      NameOperatorConfig,
		},
	}); err != nil {
		t.Fatal(err)
	}

	var podMonitorings monitoringv1.PodMonitoringList
	if err := kubeClient.List(ctx, &podMonitorings); err != nil {
		t.Fatal(err)
	}
	switch amount := len(podMonitorings.Items); amount {
	case 1:
		status := podMonitorings.Items[0].Status
		for i := range status.Conditions {
			// Normalize times because we cannot predict this.
			condition := &status.Conditions[i]
			condition.LastUpdateTime = metav1.Time{}
			condition.LastTransitionTime = metav1.Time{}
		}
		if diff := cmp.Diff(status, statusOut); diff != "" {
			t.Fatalf("invalid PodMonitoringStatus: %s", diff)
		}
	default:
		t.Fatalf("invalid PodMonitorings found: %d", amount)
	}
}

func TestSetConfigMapData(t *testing.T) {
	const data = "§psdmopnwepg30t-3ivp msdlc\n\r`1-k`23dvpdmfpdfgfn-p"

	c := &corev1.ConfigMap{}
	{
		// Set & check uncompressed.
		if err := setConfigMapData(c, monitoringv1.CompressionNone, "abc.yaml", data); err != nil {
			t.Fatal(err)
		}
		if len(c.Data) != 1 {
			t.Fatalf("expected one element in configMap Data, got: %s", c.Data)
		}
		if c.BinaryData != nil {
			t.Fatalf("expected nil configMap BinaryData, got: %s", c.BinaryData)
		}
		if diff := cmp.Diff(data, c.Data["abc.yaml"]); diff != "" {
			t.Fatalf("unexpected uncompressed data: %s", diff)
		}
	}
	{
		// Set & check compressed.
		if err := setConfigMapData(c, monitoringv1.CompressionGzip, "abc2.yaml", data); err != nil {
			t.Fatal(err)
		}
		if len(c.Data) != 1 {
			t.Fatalf("expected one element in configMap Data, got: %s", c.Data)
		}
		if len(c.BinaryData) != 1 {
			t.Fatalf("expected nil configMap BinaryData, got: %s", c.BinaryData)
		}
		// Make sure previous data still exists.
		if diff := cmp.Diff(data, c.Data["abc.yaml"]); diff != "" {
			t.Fatalf("unexpected uncompressed data: %s", diff)
		}

		r, err := gzip.NewReader(bytes.NewReader(c.BinaryData["abc2.yaml"]))
		if err != nil {
			t.Fatal(err)
		}
		uncompressed, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(data, string(uncompressed)); diff != "" {
			t.Fatalf("unexpected uncompressed data: %s", diff)
		}
	}
}
