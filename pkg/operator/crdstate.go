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

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var errInvalidCond = fmt.Errorf("condition needs both 'Type' and 'Status' fields set")

// CRDStatusState maintains state of the statuses of CRDs the operator manages.
type CRDStatusState struct {
	podmons map[string]*pmState
	// Primarily used for testing.
	now func() metav1.Time
}

type pmState struct {
	pm          *monitoringv1alpha1.PodMonitoring
	needsUpdate bool
	conds       map[monitoringv1alpha1.MonitoringConditionType]*monitoringv1alpha1.MonitoringCondition
}

func newPMState(pm *monitoringv1alpha1.PodMonitoring, now metav1.Time) *pmState {
	var state = &pmState{
		pm:          pm,
		needsUpdate: false,
		conds:       make(map[monitoringv1alpha1.MonitoringConditionType]*monitoringv1alpha1.MonitoringCondition),
	}
	// Set up defaults.
	for _, mc := range monitoringv1alpha1.NewDefaultConditions(now) {
		state.conds[mc.Type] = &mc
	}
	// Overwrite with any previous state.
	for _, mc := range pm.Status.Conditions {
		state.conds[mc.Type] = &mc
	}
	return state
}

// NewCRDStatusState returns a CRDStatusState instance with the specified conditions
// length enforcement.
func NewCRDStatusState(now func() metav1.Time) *CRDStatusState {
	return &CRDStatusState{
		podmons: make(map[string]*pmState),
		now:     now,
	}
}

// SetPodMonitoringCondition adds the provided PodMonitoring resource to the managed state
// along with the provided condition iff the resource generation has changed or there
// is a status condition state transition.
func (c *CRDStatusState) SetPodMonitoringCondition(pm *monitoringv1alpha1.PodMonitoring, cond *monitoringv1alpha1.MonitoringCondition) error {
	var (
		specChanged      = pm.Status.ObservedGeneration != pm.Generation
		statusTransition = false
	)

	if cond.Type == "" || cond.Status == "" {
		return errInvalidCond
	}

	// Set some timestamp defaults if unspecified.
	cond.LastUpdateTime = c.now()

	// Create new entry if none exists for this podmonitoring resource.
	key, err := cache.MetaNamespaceKeyFunc(pm)
	if err != nil {
		return err
	}
	state, ok := c.podmons[key]
	if !ok {
		state = newPMState(pm, c.now())
		c.podmons[key] = state
	}

	// Check if the condition results in a transition of status state.
	if old := state.conds[cond.Type]; old.Status == cond.Status {
		cond.LastTransitionTime = old.LastTransitionTime
	} else {
		cond.LastTransitionTime = cond.LastUpdateTime
		statusTransition = true
	}

	// Set condition.
	state.conds[cond.Type] = cond

	// Only update status if the spec has changed (reflected by Generation field) or
	// if this update transitions status state.
	if specChanged || statusTransition {
		state.needsUpdate = true
	}
	return nil
}

// PodMonitorings only returns podmonitoring resources where a status update
// was significant.
func (c *CRDStatusState) PodMonitorings() []monitoringv1alpha1.PodMonitoring {
	var pmons []monitoringv1alpha1.PodMonitoring

	for _, state := range c.podmons {
		if state.needsUpdate {
			pm := state.pm.DeepCopy()
			pm.Status = monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: pm.Generation,
			}
			for _, c := range state.conds {
				pm.Status.Conditions = append(pm.Status.Conditions, *c)
			}
			pmons = append(pmons, *pm)
		}
	}

	return pmons
}

// Reset clears all state.
func (c *CRDStatusState) Reset() {
	c.podmons = make(map[string]*pmState)
}
