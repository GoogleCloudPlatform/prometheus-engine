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

package v1

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetMonitoringCondition(t *testing.T) {
	var (
		before = metav1.NewTime(time.Unix(1234, 0))
		now    = metav1.NewTime(time.Unix(5678, 0))
	)
	cases := []struct {
		doc        string
		cond       *MonitoringCondition
		generation int64
		now        metav1.Time
		curr, want *MonitoringStatus
		change     bool
	}{
		{
			doc:  "no previous status",
			curr: &MonitoringStatus{},
			cond: &MonitoringCondition{
				Type:   ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 1,
			now:        now,
			want: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
			change: true,
		},
		{
			doc: "matching previous status - prevent cycle",
			curr: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &MonitoringCondition{
				Type:   ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 1,
			now:        now,
			want: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			change: false,
		},
		{
			doc: "success to success transition due to spec change",
			curr: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &MonitoringCondition{
				Type:   ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 2,
			now:        now,
			want: &MonitoringStatus{
				ObservedGeneration: 2,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: before,
					},
				},
			},
			change: true,
		},
		{
			doc: "failure to success transition due to spec fix",
			curr: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionFalse,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &MonitoringCondition{
				Type:   ConfigurationCreateSuccess,
				Status: corev1.ConditionTrue,
			},
			generation: 2,
			now:        now,
			want: &MonitoringStatus{
				ObservedGeneration: 2,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
			change: true,
		},
		{
			doc: "success to failure transition due to status update",
			curr: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionTrue,
						LastUpdateTime:     before,
						LastTransitionTime: before,
					},
				},
			},
			cond: &MonitoringCondition{
				Type:   ConfigurationCreateSuccess,
				Status: corev1.ConditionFalse,
			},
			generation: 1,
			now:        now,
			want: &MonitoringStatus{
				ObservedGeneration: 1,
				Conditions: []MonitoringCondition{
					{
						Type:               ConfigurationCreateSuccess,
						Status:             corev1.ConditionFalse,
						LastUpdateTime:     now,
						LastTransitionTime: now,
					},
				},
			},
			change: true,
		},
	}
	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			got := c.curr
			change := got.SetMonitoringCondition(c.generation, c.now, c.cond)

			// Get resolved podmonitorings.
			if change != c.change {
				t.Errorf("unexpected change")
			} else if diff := cmp.Diff(got, c.want); diff != "" {
				t.Errorf("actual status differs from expected. diff: %s", diff)
			}
		})
	}
}
