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

var (
	errInvalidCond     = fmt.Errorf("condition needs both 'Type' and 'Status' fields set")
	errUnsupportedType = fmt.Errorf("unsupported type for CRD state")
)

// CRDStatusState maintains state of the statuses of CRDs the operator manages.
type CRDStatusState struct {
	pMons map[string]*pmState
	cMons map[string]*pmState
	// Primarily used for testing.
	now func() metav1.Time
}

type pmState struct {
	pm          *monitoringv1alpha1.PodMonitoring
	cm          *monitoringv1alpha1.ClusterPodMonitoring
	needsUpdate bool
	conds       map[monitoringv1alpha1.MonitoringConditionType]*monitoringv1alpha1.MonitoringCondition
}

func newPMState(pm *monitoringv1alpha1.PodMonitoring, cm *monitoringv1alpha1.ClusterPodMonitoring, now metav1.Time) *pmState {
	var state = &pmState{
		needsUpdate: false,
		conds:       make(map[monitoringv1alpha1.MonitoringConditionType]*monitoringv1alpha1.MonitoringCondition),
	}
	// Set up defaults.
	for _, mc := range monitoringv1alpha1.NewDefaultConditions(now) {
		state.conds[mc.Type] = &mc
	}
	if pm != nil {
		state.pm = pm
		// Overwrite with any previous state.
		for _, mc := range pm.Status.Conditions {
			state.conds[mc.Type] = &mc
		}
	} else if cm != nil {
		state.cm = cm
		// Overwrite with any previous state.
		for _, mc := range cm.Status.Conditions {
			state.conds[mc.Type] = &mc
		}
	}
	return state
}

// NewCRDStatusState returns a CRDStatusState instance with the specified conditions
// length enforcement.
func NewCRDStatusState(now func() metav1.Time) *CRDStatusState {
	return &CRDStatusState{
		pMons: make(map[string]*pmState),
		cMons: make(map[string]*pmState),
		now:   now,
	}
}

// SetPodMonitoringCondition adds the provided PodMonitoring resource to the managed state
// along with the provided condition iff the resource generation has changed or there
// is a status condition state transition.
func (c *CRDStatusState) SetPodMonitoringCondition(obj metav1.Object, obsGen int64, cond *monitoringv1alpha1.MonitoringCondition) error {
	var (
		specChanged      = obsGen != obj.GetGeneration()
		statusTransition = false
	)

	if cond.Type == "" || cond.Status == "" {
		return errInvalidCond
	}

	// Set some timestamp defaults if unspecified.
	cond.LastUpdateTime = c.now()

	// Create new entry if none exists for this podmonitoring resource.
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return err
	}

	// Check if state for CRD is cached already.
	var state *pmState
	if pm, ok := obj.(*monitoringv1alpha1.PodMonitoring); ok {
		if s, ok := c.pMons[key]; ok {
			state = s
		} else {
			state = newPMState(pm, nil, c.now())
			c.pMons[key] = state
		}
	} else if cm, ok := obj.(*monitoringv1alpha1.ClusterPodMonitoring); ok {
		if s, ok := c.cMons[key]; ok {
			state = s
		} else {
			state = newPMState(nil, cm, c.now())
			c.cMons[key] = state
		}
	} else {
		return errUnsupportedType
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

	for _, state := range c.pMons {
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

// ClusterPodMonitorings only returns podmonitoring resources where a status update
// was significant.
func (c *CRDStatusState) ClusterPodMonitorings() []monitoringv1alpha1.ClusterPodMonitoring {
	var cmons []monitoringv1alpha1.ClusterPodMonitoring

	for _, state := range c.cMons {
		if state.needsUpdate {
			cm := state.cm.DeepCopy()
			cm.Status = monitoringv1alpha1.PodMonitoringStatus{
				ObservedGeneration: cm.Generation,
			}
			for _, c := range state.conds {
				cm.Status.Conditions = append(cm.Status.Conditions, *c)
			}
			cmons = append(cmons, *cm)
		}
	}

	return cmons
}

// Reset clears all state.
func (c *CRDStatusState) Reset() {
	c.pMons = make(map[string]*pmState)
	c.cMons = make(map[string]*pmState)
}
