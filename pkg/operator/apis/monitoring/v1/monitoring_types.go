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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MonitoringCRD interface {
	client.Object

	// GetMonitoringStatus returns this CRD's status sub-resource, which must be
	// available at the top-level.
	GetMonitoringStatus() *MonitoringStatus
}

// MonitoringConditionType is the type of MonitoringCondition.
type MonitoringConditionType string

const (
	// ConfigurationCreateSuccess indicates that the config generated from the
	// monitoring resource was created successfully.
	ConfigurationCreateSuccess MonitoringConditionType = "ConfigurationCreateSuccess"
	// CollectorDaemonSetExists indicates whether the collector DaemonSet exists.
	CollectorDaemonSetExists MonitoringConditionType = "CollectorDaemonSetExists"
)

// MonitoringCondition describes the condition of a PodMonitoring.
type MonitoringCondition struct {
	Type MonitoringConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human-readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// NewDefaultConditions returns a list of default conditions.
func NewDefaultConditions(now metav1.Time) []MonitoringCondition {
	return []MonitoringCondition{
		{
			Type:               ConfigurationCreateSuccess,
			Status:             corev1.ConditionUnknown,
			LastUpdateTime:     now,
			LastTransitionTime: now,
		},
	}
}

// IsValid returns true if the condition has a valid type and status.
func (cond *MonitoringCondition) IsValid() bool {
	return cond.Type != "" && cond.Status != ""
}

// MonitoringStatus holds status information of a monitoring resource.
type MonitoringStatus struct {
	// The generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// Represents the latest available observations of a podmonitor's current state.
	Conditions []MonitoringCondition `json:"conditions,omitempty"`
}

// SetMonitoringCondition merges the provided valid condition if the resource generation changed or
// there is a status condition state transition.
func (status *MonitoringStatus) SetMonitoringCondition(gen int64, now metav1.Time, cond *MonitoringCondition) bool {
	var (
		specChanged              = status.ObservedGeneration != gen
		statusTransition, update bool
		conds                    = make(map[MonitoringConditionType]*MonitoringCondition)
	)

	if !cond.IsValid() {
		return false
	}

	// Set up defaults.
	for _, mc := range NewDefaultConditions(now) {
		conds[mc.Type] = &mc
	}
	// Overwrite with any previous state.
	for _, mc := range status.Conditions {
		conds[mc.Type] = &mc
	}

	// Set some timestamp defaults if unspecified.
	cond.LastUpdateTime = now

	// Check if the condition results in a transition of status state.
	if old := conds[cond.Type]; old != nil && old.Status == cond.Status {
		cond.LastTransitionTime = old.LastTransitionTime
	} else {
		cond.LastTransitionTime = cond.LastUpdateTime
		statusTransition = true
	}

	// Set condition.
	conds[cond.Type] = cond

	// Only update status if the spec has changed (indicated by Generation field) or
	// if this update transitions status state.
	if specChanged || statusTransition {
		update = true
		status.ObservedGeneration = gen
		status.Conditions = status.Conditions[:0]
		for _, c := range conds {
			status.Conditions = append(status.Conditions, *c)
		}
	}

	return update
}
