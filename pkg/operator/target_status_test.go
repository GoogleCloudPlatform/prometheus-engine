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
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/google/go-cmp/cmp"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	tclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type updateTargetStatusTestCase struct {
	desc                  string
	targets               []*prometheusv1.TargetsResult
	podMonitorings        []monitoringv1.PodMonitoring
	clusterPodMonitorings []monitoringv1.ClusterPodMonitoring
	expErr                bool
}

// Given a list of test cases on PodMonitoring, creates a new list containing
// those test cases and equivalent test cases for ClusterPodMonitoring and
// another equivalent set including both PodMonitoring and ClusterPodMonitoring.
func expand(testCases []updateTargetStatusTestCase) []updateTargetStatusTestCase {
	dataFinal := make([]updateTargetStatusTestCase, 0)
	for _, tc := range testCases {
		if len(tc.podMonitorings) == 0 {
			dataFinal = append(dataFinal, updateTargetStatusTestCase{
				desc:    tc.desc,
				targets: tc.targets,
				expErr:  tc.expErr,
			})
			continue
		}
		clusterTargets := make([]*prometheusv1.TargetsResult, 0, len(tc.targets))
		clusterPodMonitorings := make([]monitoringv1.ClusterPodMonitoring, 0, len(tc.podMonitorings))
		for _, target := range tc.targets {
			if target == nil {
				clusterTargets = append(clusterTargets, nil)
				continue
			}
			clusterActive := make([]prometheusv1.ActiveTarget, 0, len(target.Active))
			for _, active := range target.Active {
				activeCluster := active
				activeCluster.ScrapePool = podMonitoringScrapePoolToClusterPodMonitoringScrapePool(active.ScrapePool)
				clusterActive = append(clusterActive, activeCluster)
			}
			targetClusterPodMonitoring := &prometheusv1.TargetsResult{
				Active: clusterActive,
			}
			clusterTargets = append(clusterTargets, targetClusterPodMonitoring)
		}
		for _, pm := range tc.podMonitorings {
			copy := pm.DeepCopy()
			cpm := monitoringv1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name: copy.Name,
				},
				Spec: monitoringv1.ClusterPodMonitoringSpec{
					Selector:     copy.Spec.Selector,
					Endpoints:    copy.Spec.Endpoints,
					TargetLabels: copy.Spec.TargetLabels,
					Limits:       copy.Spec.Limits,
				},
				Status: copy.Status,
			}
			for idx, status := range cpm.Status.EndpointStatuses {
				cpm.Status.EndpointStatuses[idx].Name = podMonitoringScrapePoolToClusterPodMonitoringScrapePool(status.Name)
			}
			clusterPodMonitorings = append(clusterPodMonitorings, cpm)
		}
		dataPodMonitorings := updateTargetStatusTestCase{
			desc:           tc.desc + "-pod-monitoring",
			targets:        tc.targets,
			podMonitorings: tc.podMonitorings,
			expErr:         tc.expErr,
		}
		dataFinal = append(dataFinal, dataPodMonitorings)
		dataClusterPodMonitorings := updateTargetStatusTestCase{
			desc:                  tc.desc + "-cluster-pod-monitoring",
			targets:               clusterTargets,
			clusterPodMonitorings: clusterPodMonitorings,
			expErr:                tc.expErr,
		}
		prometheusTargetsBoth := append(tc.targets, clusterTargets...)
		dataBoth := updateTargetStatusTestCase{
			desc:                  tc.desc + "-both",
			targets:               prometheusTargetsBoth,
			podMonitorings:        tc.podMonitorings,
			clusterPodMonitorings: clusterPodMonitorings,
			expErr:                tc.expErr,
		}
		dataFinal = append(dataFinal, dataClusterPodMonitorings)
		dataFinal = append(dataFinal, dataBoth)
	}
	return dataFinal
}

func podMonitoringScrapePoolToClusterPodMonitoringScrapePool(podMonitoringScrapePool string) string {
	scrapePool := podMonitoringScrapePool[len("PodMonitoring/"):]
	scrapePool = scrapePool[strings.Index(scrapePool, "/")+1:]
	return "ClusterPodMonitoring/" + scrapePool
}

func targetFetchFromMap(m map[string]*prometheusv1.TargetsResult) getTargetFn {
	return func(_ context.Context, _ logr.Logger, httpClient *http.Client, port int32, pod *corev1.Pod) (*prometheusv1.TargetsResult, error) {
		key := getPodKey(pod, port)
		targetsResult, ok := m[key]
		if !ok {
			return nil, fmt.Errorf("Pod target does not exist: %s", key)
		}
		return targetsResult, nil
	}
}

func TestUpdateTargetStatus(t *testing.T) {
	var date = metav1.Date(2022, time.January, 4, 0, 0, 0, 0, time.UTC)

	testCases := expand([]updateTargetStatusTestCase{
		// All empty -- nothing happens.
		{
			desc: "empty-monitorings",
		},
		// Single target, no monitorings -- nothing happens.
		{
			desc: "single-target-no-monitorings",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			expErr: true,
		},
		// Single healthy target with no error, with matching PodMonitoring.
		{
			desc: "single-healthy-target",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						ObservedGeneration: 2,
						Conditions: []monitoringv1.MonitoringCondition{{
							Type:               monitoringv1.ConfigurationCreateSuccess,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     metav1.Time{},
							LastTransitionTime: metav1.Time{},
							Reason:             "",
							Message:            "",
						}},
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 0,
								LastUpdateTime:   date,
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
					},
				}},
		},
		// Collectors target fetch failure.
		{
			desc: "collectors-target-fetch-failure",
			targets: []*prometheusv1.TargetsResult{
				nil,
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-2/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "b",
						}),
						LastScrapeDuration: 2.4,
					}},
				},
				nil,
				nil,
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 0,
								LastUpdateTime:   date,
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
								CollectorsFraction: "0.4",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-2", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-2/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 0,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health: "up",
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "b",
												},
												LastScrapeDurationSeconds: "2.4",
											},
										},
										Count: pointer.Int32(1),
									},
								},
								CollectorsFraction: "0.4",
							},
						},
					},
				}},
		},
		// Single healthy target with no error, with non-matching PodMonitoring.
		{
			desc: "single-healthy-target-no-match",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-2/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
				}},
			expErr: true,
		},
		// Single healthy target with no error, with single matching PodMonitoring.
		{
			desc: "single-healthy-target-matching",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-2/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-2", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-2/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 0,
								LastUpdateTime:   date,
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
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-3", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
				}},
		},
		// Single healthy target with an error, with matching PodMonitoring.
		{
			desc: "single-healthy-target-with-error-matching",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 0,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "up",
												LastError: pointer.String("err x"),
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
					},
				}},
		},
		// Single unhealthy target with an error, with matching PodMonitoring.
		{
			desc: "single-unhealthy-target-with-error-matching",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    1,
								UnhealthyTargets: 1,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
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
					},
				}},
		},
		// One healthy and one unhealthy target.
		{
			desc: "single-healthy-single-unhealthy",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "b",
						}),
						LastScrapeDuration: 1.2,
					}, {
						Health:     "up",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 4.3,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    2,
								UnhealthyTargets: 1,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "b",
												},
												LastScrapeDurationSeconds: "1.2",
											},
										},
										Count: pointer.Int32(1),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health: "up",
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "a",
												},
												LastScrapeDurationSeconds: "4.3",
											},
										},
										Count: pointer.Int32(1),
									},
								},
								CollectorsFraction: "1",
							},
						},
					},
				}},
		},
		// Multiple targets with multiple endpoints.
		{
			desc: "multiple-targets-multiple-endpoints",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics-2",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "d",
						}),
						LastScrapeDuration: 3.6,
					}, {
						Health:     "down",
						LastError:  "err y",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics-1",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "b",
						}),
						LastScrapeDuration: 7.0,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics-1",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 5.3,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics-2",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "c",
						}),
						LastScrapeDuration: 1.2,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics-1"),
						}, {
							Port: intstr.FromString("metrics-2"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics-1",
								ActiveTargets:    2,
								UnhealthyTargets: 2,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "a",
												},
												LastScrapeDurationSeconds: "5.3",
											},
										},
										Count: pointer.Int32(1),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err y"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "b",
												},
												LastScrapeDurationSeconds: "7",
											},
										},
										Count: pointer.Int32(1),
									},
								},
								CollectorsFraction: "1",
							},
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics-2",
								ActiveTargets:    2,
								UnhealthyTargets: 2,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "c",
												},
												LastScrapeDurationSeconds: "1.2",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "d",
												},
												LastScrapeDurationSeconds: "3.6",
											},
										},
										Count: pointer.Int32(2),
									},
								},
								CollectorsFraction: "1",
							},
						},
					},
				}},
		},
		// Multiple unhealthy target with different errors.
		{
			desc: "multiple-unhealthy-targets",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "f",
						}),
						LastScrapeDuration: 1.2,
					}, {
						Health:     "down",
						LastError:  "err y",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "c",
						}),
						LastScrapeDuration: 2.4,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "e",
						}),
						LastScrapeDuration: 3.6,
					}, {
						Health:     "down",
						LastError:  "err z",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "d",
						}),
						LastScrapeDuration: 4.7,
					}, {
						Health:     "down",
						LastError:  "err z",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 5.0,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "b",
						}),
						LastScrapeDuration: 6.8,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    6,
								UnhealthyTargets: 6,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "b",
												},
												LastScrapeDurationSeconds: "6.8",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "e",
												},
												LastScrapeDurationSeconds: "3.6",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "f",
												},
												LastScrapeDurationSeconds: "1.2",
											},
										},
										Count: pointer.Int32(3),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err y"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "c",
												},
												LastScrapeDurationSeconds: "2.4",
											},
										},
										Count: pointer.Int32(1),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err z"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "a",
												},
												LastScrapeDurationSeconds: "5",
											},
											{
												Health:    "down",
												LastError: pointer.String("err z"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "d",
												},
												LastScrapeDurationSeconds: "4.7",
											},
										},
										Count: pointer.Int32(2),
									},
								},
								CollectorsFraction: "1",
							},
						},
					},
				}},
		},
		// Multiple unhealthy targets, one cut-off.
		{
			desc: "multiple-unhealthy-targets-cut-off",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "f",
						}),
						LastScrapeDuration: 1.2,
					}, {
						Health:     "down",
						LastError:  "err y",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "c",
						}),
						LastScrapeDuration: 2.4,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 3.6,
					}, {
						Health:     "down",
						LastError:  "err z",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "d",
						}),
						LastScrapeDuration: 4.7,
					}, {
						Health:     "down",
						LastError:  "err z",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "a",
						}),
						LastScrapeDuration: 5.0,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "b",
						}),
						LastScrapeDuration: 6.8,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "e",
						}),
						LastScrapeDuration: 4.1,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "f",
						}),
						LastScrapeDuration: 7.3,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "c",
						}),
						LastScrapeDuration: 2.7,
					}, {
						Health:     "down",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": "d",
						}),
						LastScrapeDuration: 9.5,
					}},
				},
			},
			podMonitorings: []monitoringv1.PodMonitoring{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{{
							Port: intstr.FromString("metrics"),
						}},
					},
					Status: monitoringv1.PodMonitoringStatus{
						EndpointStatuses: []monitoringv1.ScrapeEndpointStatus{
							{
								Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
								ActiveTargets:    10,
								UnhealthyTargets: 10,
								LastUpdateTime:   date,
								SampleGroups: []monitoringv1.SampleGroup{
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "a",
												},
												LastScrapeDurationSeconds: "3.6",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "b",
												},
												LastScrapeDurationSeconds: "6.8",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "c",
												},
												LastScrapeDurationSeconds: "2.7",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "d",
												},
												LastScrapeDurationSeconds: "9.5",
											},
											{
												Health:    "down",
												LastError: pointer.String("err x"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "e",
												},
												LastScrapeDurationSeconds: "4.1",
											},
										},
										Count: pointer.Int32(7),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err y"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "c",
												},
												LastScrapeDurationSeconds: "2.4",
											},
										},
										Count: pointer.Int32(1),
									},
									{
										SampleTargets: []monitoringv1.SampleTarget{
											{
												Health:    "down",
												LastError: pointer.String("err z"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "a",
												},
												LastScrapeDurationSeconds: "5",
											},
											{
												Health:    "down",
												LastError: pointer.String("err z"),
												Labels: map[model.LabelName]model.LabelValue{
													"instance": "d",
												},
												LastScrapeDurationSeconds: "4.7",
											},
										},
										Count: pointer.Int32(2),
									},
								},
								CollectorsFraction: "1",
							},
						},
					},
				}},
		},
		{
			desc: "kubelet hardcoded scrape configs",
			targets: []*prometheusv1.TargetsResult{
				{
					Active: []prometheusv1.ActiveTarget{
						{
							Health:     "up",
							LastError:  "",
							ScrapePool: "kubelet/cadvisor",
							Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
								"instance": "node-1-default-pool-abcd1234:cadvisor",
								"job":      "kubelet",
								"node":     "node-1-default-pool-abcd1234",
							}),
							LastScrapeDuration: 0.2,
						},
						{
							Health:     "up",
							LastError:  "",
							ScrapePool: "kubelet/metrics",
							Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
								"instance": "node-1-default-pool-abcd1234:metrics",
								"job":      "kubelet",
								"node":     "node-1-default-pool-abcd1234",
							}),
							LastScrapeDuration: 0.2,
						},
					},
				},
			},
		},
	})

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("target-status-conversion-%s", testCase.desc), func(t *testing.T) {
			clientBuilder := newFakeClientBuilder()
			for _, podMonitoring := range testCase.podMonitorings {
				copy := podMonitoring.DeepCopy()
				copy.GetStatus().EndpointStatuses = nil
				clientBuilder.WithObjects(copy)
			}
			for _, clusterPodMonitoring := range testCase.clusterPodMonitorings {
				copy := clusterPodMonitoring.DeepCopy()
				copy.GetStatus().EndpointStatuses = nil
				clientBuilder.WithObjects(copy)
			}

			kubeClient := clientBuilder.Build()

			err := updateTargetStatus(context.Background(), testr.New(t), kubeClient, testCase.targets)
			if err != nil && !testCase.expErr {
				t.Fatalf("unexpected error updating target status: %s", err)
			}

			for _, podMonitoring := range testCase.podMonitorings {
				var after monitoringv1.PodMonitoring
				if err := kubeClient.Get(context.Background(), types.NamespacedName{
					Namespace: podMonitoring.GetNamespace(),
					Name:      podMonitoring.GetName(),
				}, &after); err != nil {
					t.Fatal("Unable to find PodMonitoring:", podMonitoring.GetKey(), err)
				}
				normalizeEndpointStatuses(after.Status.EndpointStatuses, date)
				if !cmp.Equal(podMonitoring.Status, after.Status) {
					t.Errorf("PodMonitoring does not match: %s\n%s", podMonitoring.GetKey(), cmp.Diff(podMonitoring.Status, after.Status))
				}
			}

			for _, clusterPodMonitoring := range testCase.clusterPodMonitorings {
				var after monitoringv1.ClusterPodMonitoring
				if err := kubeClient.Get(context.Background(), types.NamespacedName{
					Name: clusterPodMonitoring.GetName(),
				}, &after); err != nil {
					t.Fatal("Unable to find ClusterPodMonitoring:", clusterPodMonitoring.GetKey(), err)
				}
				normalizeEndpointStatuses(after.Status.EndpointStatuses, date)
				if !cmp.Equal(clusterPodMonitoring.Status, after.Status) {
					t.Errorf("ClusterPodMonitoring does not match: %s\n%s", clusterPodMonitoring.GetKey(), cmp.Diff(clusterPodMonitoring.Status, after.Status))
				}
			}
		})
	}
}

func getPodKey(pod *corev1.Pod, port int32) string {
	return fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
}

func normalizeEndpointStatuses(endpointStatuses []monitoringv1.ScrapeEndpointStatus, time metav1.Time) {
	for i := range endpointStatuses {
		endpointStatuses[i].LastUpdateTime = time
	}
}

// Test that polling propagates all the way through and only on ticks.
func TestPolling(t *testing.T) {
	ctx := context.Background()
	logger := testr.New(t)
	opts := Options{
		ProjectID:             "test-proj",
		Location:              "test-loc",
		Cluster:               "test-cluster",
		OperatorNamespace:     "gmp-system",
		TargetPollConcurrency: 4,
	}
	if err := opts.defaultAndValidate(logger); err != nil {
		t.Fatal("Invalid options:", err)
	}

	fakeClock := tclock.NewFakeClock(time.Now())

	port := int32(19090)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-a",
			Namespace: opts.OperatorNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "prometheus",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "127.0.0.1",
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "prometheus",
				Ready: true,
			}},
		},
	}

	kubeClient := newFakeClientBuilder().WithObjects(&appsv1.DaemonSet{
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
						Ports: []corev1.ContainerPort{{
							Name:          "prom-metrics",
							ContainerPort: port,
						}},
					}},
				},
			},
		},
	}).WithObjects(
		&monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config",
				Namespace: "gmp-system",
			},
			Features: monitoringv1.OperatorFeatures{
				TargetStatus: monitoringv1.TargetStatusSpec{
					Enabled: true,
				},
			},
		},
	).WithObjects(&monitoringv1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{Name: "prom-example-1", Namespace: "gmp-test"},
		Spec: monitoringv1.PodMonitoringSpec{
			Endpoints: []monitoringv1.ScrapeEndpoint{{
				Port: intstr.FromString("metrics"),
			}},
		},
	}).WithObjects(pod).Build()

	prometheusTargetMap := make(map[string]*prometheusv1.TargetsResult, 1)
	key := getPodKey(pod, port)
	prometheusTargetMap[key] = &prometheusv1.TargetsResult{
		Active: []prometheusv1.ActiveTarget{{
			Health: "up",
			Labels: map[model.LabelName]model.LabelValue{
				"instance": model.LabelValue("a"),
			},
			ScrapePool:         "PodMonitoring/gmp-test/prom-example-1/metrics",
			LastError:          "err x",
			LastScrapeDuration: 1.2,
		}},
	}

	ch := make(chan event.GenericEvent, 1)
	reconciler := &targetStatusReconciler{
		ch:         ch,
		opts:       opts,
		getTarget:  targetFetchFromMap(prometheusTargetMap),
		logger:     logger,
		kubeClient: kubeClient,
		clock:      fakeClock,
	}

	expectStatus := func(t *testing.T, description string, expected []monitoringv1.ScrapeEndpointStatus) {
		// Must poll because status is updated via other thread.
		var err error
		if pollErr := wait.Poll(100*time.Millisecond, 2*time.Second, func() (bool, error) {
			var podMonitorings monitoringv1.PodMonitoringList
			if err := kubeClient.List(ctx, &podMonitorings); err != nil {
				return false, nil
			}
			switch amount := len(podMonitorings.Items); amount {
			case 0:
				err = fmt.Errorf("Could not find %s PodMonitoring", description)
				return false, nil
			case 1:
				status := podMonitorings.Items[0].Status.EndpointStatuses
				normalizeEndpointStatuses(status, metav1.Time{})
				diff := cmp.Diff(status, expected)
				if diff != "" {
					err = fmt.Errorf("Expected %s endpoint statuses to be: %s", description, diff)
					return false, nil
				}
				return true, nil
			default:
				err = fmt.Errorf("invalid PodMonitorings found: %d", amount)
				return false, err
			}
		}); pollErr != nil {
			t.Fatalf("Failed waiting for %s status: %s", description, err)
		}
	}

	// Status should be empty initially, until the reconciler starts.
	expectStatus(t, "initial", nil)

	go func() {
		// Emulate Kubernetes controller manager event handler behavior.
		ch <- event.GenericEvent{
			Object: &appsv1.DaemonSet{},
		}
		for range ch {
			if _, err := reconciler.Reconcile(ctx, reconcile.Request{}); err != nil {
				t.Errorf("error reconciling: %s", err)
			}
		}
	}()

	// First tick.
	fakeClock.Step(minPollDuration)
	statusTick1 := []monitoringv1.ScrapeEndpointStatus{
		{
			Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
			ActiveTargets:    1,
			UnhealthyTargets: 0,
			SampleGroups: []monitoringv1.SampleGroup{
				{
					SampleTargets: []monitoringv1.SampleTarget{
						{
							Health: "up",
							Labels: map[model.LabelName]model.LabelValue{
								"instance": "a",
							},
							LastError:                 pointer.String("err x"),
							LastScrapeDurationSeconds: "1.2",
						},
					},
					Count: pointer.Int32(1),
				},
			},
			CollectorsFraction: "1",
		},
	}
	expectStatus(t, "first tick", statusTick1)

	active := &prometheusTargetMap[key].Active[0]
	active.Health = "down"
	active.LastError = "err y"
	active.LastScrapeDuration = 5.4
	// We didn't tick yet so we don't expect a change yet.
	expectStatus(t, "first wait", statusTick1)

	// Second tick.
	fakeClock.Step(minPollDuration)
	statusTick2 := []monitoringv1.ScrapeEndpointStatus{
		{
			Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
			ActiveTargets:    1,
			UnhealthyTargets: 1,
			SampleGroups: []monitoringv1.SampleGroup{
				{
					SampleTargets: []monitoringv1.SampleTarget{
						{
							Health: "down",
							Labels: map[model.LabelName]model.LabelValue{
								"instance": "a",
							},
							LastError:                 pointer.String("err y"),
							LastScrapeDurationSeconds: "5.4",
						},
					},
					Count: pointer.Int32(1),
				},
			},
			CollectorsFraction: "1",
		},
	}
	expectStatus(t, "second tick", statusTick2)

	active = &prometheusTargetMap[key].Active[0]
	active.Health = "up"
	active.LastError = "err z"
	active.LastScrapeDuration = 8.3
	// We didn't tick yet so we don't expect a change yet.
	expectStatus(t, "second wait", statusTick2)

	fakeClock.Step(minPollDuration)
	statusTick3 := []monitoringv1.ScrapeEndpointStatus{
		{
			Name:             "PodMonitoring/gmp-test/prom-example-1/metrics",
			ActiveTargets:    1,
			UnhealthyTargets: 0,
			SampleGroups: []monitoringv1.SampleGroup{
				{
					SampleTargets: []monitoringv1.SampleTarget{
						{
							Health: "up",
							Labels: map[model.LabelName]model.LabelValue{
								"instance": "a",
							},
							LastError:                 pointer.String("err z"),
							LastScrapeDurationSeconds: "8.3",
						},
					},
					Count: pointer.Int32(1),
				},
			},
			CollectorsFraction: "1",
		},
	}
	expectStatus(t, "third tick", statusTick3)
}

func TestShouldPoll(t *testing.T) {
	cases := []struct {
		desc   string
		objs   []client.Object
		should bool
		expErr bool
	}{
		{
			desc: "should poll targets - podmonitorings",
			objs: []client.Object{
				&monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "gmp-public",
					},
					Features: monitoringv1.OperatorFeatures{
						TargetStatus: monitoringv1.TargetStatusSpec{
							Enabled: true,
						},
					},
				},
				&monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pm1",
						Namespace: "default",
					},
				},
			},
			should: true,
			expErr: false,
		},
		{
			desc: "should poll targets - clusterpodmonitorings",
			objs: []client.Object{
				&monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "gmp-public",
					},
					Features: monitoringv1.OperatorFeatures{
						TargetStatus: monitoringv1.TargetStatusSpec{
							Enabled: true,
						},
					},
				},
				&monitoringv1.ClusterPodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cpm1",
						Namespace: "default",
					},
				},
			},
			should: true,
			expErr: false,
		},
		{
			desc: "should not poll targets - no operatorconfig error",
			objs: []client.Object{
				&monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pm1",
						Namespace: "default",
					},
				},
			},
			should: false,
			expErr: true,
		},
		{
			desc: "should not poll targets - no podmonitorings",
			objs: []client.Object{
				&monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "gmp-public",
					},
					Features: monitoringv1.OperatorFeatures{
						TargetStatus: monitoringv1.TargetStatusSpec{
							Enabled: true,
						},
					},
				},
			},
			should: false,
			expErr: false,
		},
		{
			desc: "should not poll targets - disabled",
			objs: []client.Object{
				&monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "gmp-public",
					},
					Features: monitoringv1.OperatorFeatures{
						TargetStatus: monitoringv1.TargetStatusSpec{
							Enabled: false,
						},
					},
				},
				&monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pm1",
						Namespace: "default",
					},
				},
			},
			should: false,
			expErr: false,
		},
	}
	for _, tc := range cases {
		ctx := context.Background()
		nn := types.NamespacedName{
			Name:      "config",
			Namespace: "gmp-public",
		}
		kubeClient := newFakeClientBuilder().WithObjects(tc.objs...).Build()
		t.Run(tc.desc, func(t *testing.T) {
			should, err := shouldPoll(ctx, nn, kubeClient)
			if err != nil && !tc.expErr {
				t.Errorf("unexpected shouldPoll error: %s", err)
			}
			if should != tc.should {
				t.Errorf("got %t, want %t", should, tc.should)
			}
		})
	}
}

// Tests that for pod, targets are fetched correctly (concurrently).
func TestFetchTargets(t *testing.T) {
	ctx := context.Background()
	logger := testr.New(t)
	concurrency := uint16(4)
	opts := Options{
		ProjectID:             "test-proj",
		Location:              "test-loc",
		Cluster:               "test-cluster",
		TargetPollConcurrency: concurrency,
	}
	if err := opts.defaultAndValidate(logger); err != nil {
		t.Fatal("Invalid options:", err)
	}

	concurrencyInt := int(concurrency)
	// Test 0 where we have no pods to ensure the thread pool does not stall or
	// panic. Also sanity test that the thread pool can ingest at and over max
	// capacity.
	podCounts := []int{0, 1, 2, concurrencyInt - 1, concurrencyInt, concurrencyInt + 1, concurrencyInt * 3}
	for _, podCnt := range podCounts {
		t.Run(fmt.Sprintf("fetch-%d-pods", podCnt), func(t *testing.T) {
			port := int32(19090)
			prometheusTargetMap := make(map[string]*prometheusv1.TargetsResult, podCnt)
			targetsExpected := make([]*prometheusv1.TargetsResult, 0, podCnt)
			kubeClientBuilder := newFakeClientBuilder().WithObjects(&appsv1.DaemonSet{
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
								Ports: []corev1.ContainerPort{{
									Name:          "prom-metrics",
									ContainerPort: port,
								}},
							}},
						},
					},
				},
			})
			for i := 0; i < podCnt; i++ {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("pod-%d", i),
						Namespace: opts.OperatorNamespace,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "prometheus",
						}},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: fmt.Sprint(i),
						ContainerStatuses: []corev1.ContainerStatus{{
							Name:  "prometheus",
							Ready: true,
						}},
					},
				}
				kubeClientBuilder.WithObjects(pod)

				target := &prometheusv1.TargetsResult{
					Active: []prometheusv1.ActiveTarget{{
						Health:     "up",
						LastError:  "err x",
						ScrapePool: "PodMonitoring/gmp-test/prom-example-1/metrics",
						Labels: model.LabelSet(map[model.LabelName]model.LabelValue{
							"instance": model.LabelValue(fmt.Sprint(i)),
						}),
						LastScrapeDuration: 1.2,
					}},
				}
				prometheusTargetMap[getPodKey(pod, port)] = target

				targetsExpected = append(targetsExpected, target)
			}

			kubeClient := kubeClientBuilder.Build()

			targets, err := fetchTargets(ctx, logger, opts, nil, targetFetchFromMap(prometheusTargetMap), kubeClient)
			if err != nil {
				t.Fatal("Unable to fetch targets", err)
			}

			// Concurrency causes the targets slice to come back randomly.
			sort.Slice(targets, func(i, j int) bool {
				lhsName := targets[i].Active[0].Labels["instance"]
				rhsName := targets[j].Active[0].Labels["instance"]
				lhsValue, err := strconv.Atoi(string(lhsName))
				if err != nil {
					return false
				}
				rhsValue, err := strconv.Atoi(string(rhsName))
				if err != nil {
					return false
				}
				return lhsValue < rhsValue
			})

			diff := cmp.Diff(targets, targetsExpected)
			if diff != "" {
				t.Errorf("Targets:")
				for i, target := range targets {
					t.Errorf("%d: %v", i, target)
				}
				t.Errorf("Targets Expected:")
				for i, target := range targetsExpected {
					t.Errorf("%d: %v", i, target)
				}
				t.Fatalf("Targets do not match expected: %s", diff)
			}
		})
	}
}
