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
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		WithStatusSubresource(&monitoringv1.ClusterPodMonitoring{}).
		WithStatusSubresource(&monitoringv1.ClusterNodeMonitoring{}).
		WithStatusSubresource(&monitoringv1.Rules{}).
		WithStatusSubresource(&monitoringv1.ClusterRules{}).
		WithStatusSubresource(&monitoringv1.GlobalRules{})
}

func TestCollectionReconcile(t *testing.T) {
	exampleObjectMeta := metav1.ObjectMeta{
		Name:            "prom-example",
		Namespace:       "gmp-test",
		ResourceVersion: "1",
	}
	exampleTargetLabels := monitoringv1.TargetLabels{
		Metadata: &[]string{"node"},
	}
	exampleEndpointStatus := []monitoringv1.ScrapeEndpointStatus{
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
	}
	validScrapeEndpoints := []monitoringv1.ScrapeEndpoint{
		{
			Port:     intstr.FromString("metrics"),
			Interval: "10s",
		},
	}
	validScrapeNodeEndpoints := []monitoringv1.ScrapeNodeEndpoint{
		{
			Path:     "kubelet",
			Interval: "10s",
		},
	}

	testCases := []struct {
		desc     string
		input    monitoringv1.MonitoringCRD
		expected monitoringv1.MonitoringCRD
	}{
		{
			desc: "podmonitoring: no update",
			input: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "podmonitoring: update status: missing monitoring status",
			input: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
				},
			},
			expected: &monitoringv1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				},
				Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "podmonitoring: update status: empty endpoint",
			input: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    []monitoringv1.ScrapeEndpoint{{}},
				},
			},
			expected: &monitoringv1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				}, Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    []monitoringv1.ScrapeEndpoint{{}},
				},
				Status: monitoringv1.PodMonitoringStatus{
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:    "ConfigurationCreateSuccess",
								Status:  corev1.ConditionFalse,
								Reason:  "ScrapeConfigError",
								Message: "generating scrape config failed for PodMonitoring endpoint",
							},
						},
					},
				},
			},
		},
		{
			desc: "podmonitoring: default spec",
			input: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: &monitoringv1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				}, Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: monitoringv1.TargetLabels{
						Metadata: &[]string{"pod", "container", "workload_controller", "workload_controller_type"},
					},
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "podmonitoring: default spec and update status",
			input: &monitoringv1.PodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.PodMonitoringSpec{
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
				},
			},
			expected: &monitoringv1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "3",
				}, Spec: monitoringv1.PodMonitoringSpec{
					TargetLabels: monitoringv1.TargetLabels{
						Metadata: &[]string{"pod", "container", "workload_controller", "workload_controller_type"},
					},
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "clusterpodmonitoring: no update",
			input: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "clusterpodmonitoring: update status: missing monitoring status",
			input: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
				},
			},
			expected: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				},
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "clusterpodmonitoring: update status: empty endpoint",
			input: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    []monitoringv1.ScrapeEndpoint{{}},
				},
			},
			expected: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				}, Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: exampleTargetLabels,
					Endpoints:    []monitoringv1.ScrapeEndpoint{{}},
				},
				Status: monitoringv1.PodMonitoringStatus{
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:    "ConfigurationCreateSuccess",
								Status:  corev1.ConditionFalse,
								Reason:  "ScrapeConfigError",
								Message: "generating scrape config failed for ClusterPodMonitoring endpoint",
							},
						},
					},
				},
			},
		},
		{
			desc: "clusterpodmonitoring: default spec",
			input: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				}, Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: monitoringv1.TargetLabels{
						Metadata: &[]string{"namespace", "pod", "container", "workload_controller", "workload_controller_type"},
					},
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "clusterpodmonitoring: default spec and update status",
			input: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
				},
			},
			expected: &monitoringv1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "3",
				}, Spec: monitoringv1.ClusterPodMonitoringSpec{
					TargetLabels: monitoringv1.TargetLabels{
						Metadata: &[]string{"namespace", "pod", "container", "workload_controller", "workload_controller_type"},
					},
					Endpoints: validScrapeEndpoints,
				},
				Status: monitoringv1.PodMonitoringStatus{
					EndpointStatuses: exampleEndpointStatus,
					MonitoringStatus: monitoringv1.MonitoringStatus{
						Conditions: []monitoringv1.MonitoringCondition{
							{
								Type:   "ConfigurationCreateSuccess",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			desc: "clusternodemonitoring: no update",
			input: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: validScrapeNodeEndpoints,
				},
				Status: monitoringv1.MonitoringStatus{
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:   "ConfigurationCreateSuccess",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: validScrapeNodeEndpoints,
				},
				Status: monitoringv1.MonitoringStatus{
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:   "ConfigurationCreateSuccess",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			desc: "clusternodemonitoring: update status: missing monitoring status",
			input: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: validScrapeNodeEndpoints,
				},
			},
			expected: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				},
				Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: validScrapeNodeEndpoints,
				},
				Status: monitoringv1.MonitoringStatus{
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:   "ConfigurationCreateSuccess",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			desc: "clusternodemonitoring: update status: empty endpoint",
			input: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: exampleObjectMeta,
				Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: []monitoringv1.ScrapeNodeEndpoint{{}},
				},
			},
			expected: &monitoringv1.ClusterNodeMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "prom-example",
					Namespace:       "gmp-test",
					ResourceVersion: "2",
				}, Spec: monitoringv1.ClusterNodeMonitoringSpec{
					Endpoints: []monitoringv1.ScrapeNodeEndpoint{{}},
				},
				Status: monitoringv1.MonitoringStatus{
					Conditions: []monitoringv1.MonitoringCondition{
						{
							Type:    "ConfigurationCreateSuccess",
							Status:  corev1.ConditionFalse,
							Reason:  "ScrapeConfigError",
							Message: "generating scrape config failed for ClusterNodeMonitoring endpoint",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
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
				WithObjects(tc.input).
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

			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(tc.input), tc.input); err != nil {
				t.Fatal(err)
			}

			for i := range tc.input.GetMonitoringStatus().Conditions {
				// Normalize times because we cannot predict this.
				condition := &tc.input.GetMonitoringStatus().Conditions[i]
				condition.LastUpdateTime = metav1.Time{}
				condition.LastTransitionTime = metav1.Time{}
			}
			if diff := cmp.Diff(tc.expected, tc.input); diff != "" {
				t.Fatalf("unexpected update (-want, +got): %s", diff)
			}
		})
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
