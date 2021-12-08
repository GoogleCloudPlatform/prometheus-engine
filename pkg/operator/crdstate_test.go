// Copyright 2021 Google LLC
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
	"fmt"
	"reflect"
	"testing"
	"time"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddPodMonitoringCondition(t *testing.T) {
	var (
		before = metav1.NewTime(time.Unix(1234, 0))
		now    = metav1.NewTime(time.Unix(5678, 0))
	)
	cases := []struct {
		doc                   string
		cond                  *monitoringv1alpha1.MonitoringCondition
		generation            int64
		now                   metav1.Time
		currStatus, expStatus monitoringv1alpha1.PodMonitoringStatus
	}{
		{
			doc:        "no previous status",
			currStatus: monitoringv1alpha1.PodMonitoringStatus{},
			cond: &monitoringv1alpha1.MonitoringCondition{
				Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 1,
			now:        now,
			expStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
		},
		{
			doc: "matching previous status - prevent cycle",
			currStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &monitoringv1alpha1.MonitoringCondition{
				Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 1,
			now:        now,
			expStatus:  monitoringv1alpha1.PodMonitoringStatus{},
		},
		{
			doc: "success to success transition due to spec change",
			currStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &monitoringv1alpha1.MonitoringCondition{
				Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 2,
			now:        now,
			expStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 2,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: before,
					},
				},
			},
		},
		{
			doc: "failure to success transition due to spec fix",
			currStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionFalse,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &monitoringv1alpha1.MonitoringCondition{
				Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 2,
			now:        now,
			expStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 2,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
		},
		{
			doc: "success to failure transition due to status update",
			currStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &monitoringv1alpha1.MonitoringCondition{
				Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
				Status: corev1.ConditionFalse,
			},
			generation: 1,
			now:        now,
			expStatus: monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []monitoringv1alpha1.MonitoringCondition{
					{
						Type:               monitoringv1alpha1.ConfigurationCreateSuccess,
						Status:             corev1.ConditionFalse,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s - one podmonitoring, clusterpodmonitoring", c.doc), func(t *testing.T) {
			// Init state.
			state := NewCRDStatusState(func() metav1.Time { return c.now })

			pm := &monitoringv1alpha1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "pm1",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			err := state.SetPodMonitoringCondition(pm, pm.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}

			cm := &monitoringv1alpha1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cm1",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			err = state.SetPodMonitoringCondition(cm, cm.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}

			// Get resolved podmonitorings.
			pms := state.PodMonitorings()
			if len(c.expStatus.Conditions) == 0 {
				if size := len(pms); size != 0 {
					t.Errorf("state should not return any resources. returned: %d", size)
				}
			} else if size := len(pms); size != 1 {
				t.Errorf("more than one podmonitoring resource, got: %d", size)
			} else if diff := cmp.Diff(pms[0].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if pm.Name != "pm1" || pm.Generation != c.generation || !reflect.DeepEqual(pm.Status, c.currStatus) {
				t.Errorf("podmonitoring resource mutated: %+v", pm)
			}

			// Get resolved clusterpodmonitorings.
			cms := state.ClusterPodMonitorings()
			if len(c.expStatus.Conditions) == 0 {
				if size := len(cms); size != 0 {
					t.Errorf("state should not return any resources. returned: %d", size)
				}
			} else if size := len(cms); size != 1 {
				t.Errorf("more than one clusterpodmonitoring resource, got: %d", size)
			} else if diff := cmp.Diff(cms[0].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if cm.Name != "cm1" || cm.Generation != c.generation || !reflect.DeepEqual(cm.Status, c.currStatus) {
				t.Errorf("clusterpodmonitoring resource mutated: %+v", cm)
			}
		})
		// Ensure separate podmonitoring resources state is honored.
		t.Run(fmt.Sprintf("%s - two podmonitorings, clusterpodmonitorings", c.doc), func(t *testing.T) {
			// Init state.
			state := NewCRDStatusState(func() metav1.Time { return c.now })

			pm1 := &monitoringv1alpha1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "pm1",
					Namespace:  "ns1",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			pm2 := &monitoringv1alpha1.PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "pm1",
					Namespace:  "ns2",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			err := state.SetPodMonitoringCondition(pm1, pm1.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}
			err = state.SetPodMonitoringCondition(pm2, pm2.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}

			// Get resolved podmonitorings.
			pms := state.PodMonitorings()
			if len(c.expStatus.Conditions) == 0 {
				if size := len(pms); size != 0 {
					t.Errorf("state should not return any resources. returned: %d", size)
				}
			} else if size := len(pms); size != 2 {
				t.Errorf("expected 2 podmonitoring resources, got: %d", size)
			} else if diff := cmp.Diff(pms[0].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if diff := cmp.Diff(pms[1].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if pm1.Name != "pm1" || pm1.Generation != c.generation || !reflect.DeepEqual(pm1.Status, c.currStatus) {
				t.Errorf("podmonitoring resource mutated: %+v", pm1)
			} else if pm2.Name != "pm1" || pm2.Generation != c.generation || !reflect.DeepEqual(pm2.Status, c.currStatus) {
				t.Errorf("podmonitoring resource mutated: %+v", pm2)
			}

			cm1 := &monitoringv1alpha1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cm1",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			cm2 := &monitoringv1alpha1.ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cm2",
					Generation: c.generation,
				},
				Status: c.currStatus,
			}
			err = state.SetPodMonitoringCondition(cm1, cm1.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}
			err = state.SetPodMonitoringCondition(cm2, cm2.Status.ObservedGeneration, c.cond)
			if err != nil {
				t.Fatalf("setting podmonitoring state: %s", err)
			}

			// Get resolved podmonitorings.
			cms := state.ClusterPodMonitorings()
			if len(c.expStatus.Conditions) == 0 {
				if size := len(cms); size != 0 {
					t.Errorf("state should not return any resources. returned: %d", size)
				}
			} else if size := len(cms); size != 2 {
				t.Errorf("expected 2 clusterpodmonitoring resources, got: %d", size)
			} else if diff := cmp.Diff(cms[0].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if diff := cmp.Diff(cms[1].Status, c.expStatus); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			} else if cm1.Name != "cm1" || cm1.Generation != c.generation || !reflect.DeepEqual(cm1.Status, c.currStatus) {
				t.Errorf("clusterpodmonitoring resource mutated: %+v", cm1)
			} else if cm2.Name != "cm2" || cm2.Generation != c.generation || !reflect.DeepEqual(cm2.Status, c.currStatus) {
				t.Errorf("clusterpodmonitoring resource mutated: %+v", cm2)
			}
		})
	}
}

func TestReset(t *testing.T) {
	state := NewCRDStatusState(metav1.Now)
	for i := 0; i < 5; i++ {
		err := state.SetPodMonitoringCondition(&monitoringv1alpha1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pm-%d", i),
			},
		}, 0, &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		})
		if err != nil {
			t.Fatalf("setting podmonitoring state: %s", err)
		}
		err = state.SetPodMonitoringCondition(&monitoringv1alpha1.ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pm-%d", i),
			},
		}, 0, &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		})
		if err != nil {
			t.Fatalf("setting podmonitoring state: %s", err)
		}
	}
	if size := len(state.PodMonitorings()); size != 5 {
		t.Errorf("podmonitorings getter returning unexpected size: %d", size)
	}
	if size := len(state.ClusterPodMonitorings()); size != 5 {
		t.Errorf("clusterpodmonitorings getter returning unexpected size: %d", size)
	}

	state.Reset()
	if size := len(state.PodMonitorings()); size != 0 {
		t.Errorf("podmonitorings getter returning unexpected size after reset: %d", size)
	}
	if size := len(state.ClusterPodMonitorings()); size != 0 {
		t.Errorf("clusterpodmonitorings getter returning unexpected size after reset: %d", size)
	}
}
