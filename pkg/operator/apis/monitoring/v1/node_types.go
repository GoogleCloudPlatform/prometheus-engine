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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NodeMonitoringSpec contains specification parameters for NodeMonitoring.
type NodeMonitoringSpec struct {
	// Label selector that specifies which nodes are selected for this monitoring
	// configuration. If left empty all nodes are selected.
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// The endpoints to scrape on the selected nodes.
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
}

// NodeMonitoringList is a list of NodeMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeMonitoring `json:"items"`
}

// NodeMonitoring defines monitoring for a set of nodes.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type NodeMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired node selection for target discovery by
	// Prometheus.
	Spec NodeMonitoringSpec `json:"spec"`
}
