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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Probe defines monitoring for black-box probes.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Probe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired black-box probing.
	Spec ProbeSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status ProbeStatus `json:"status"`
}

// ProbeList is a list of Probe.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ProbeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Probe `json:"items"`
}

// ProbeSpec contains specification parameters for Probe. Analogous to PodMonitoringSpec.
type ProbeSpec struct {
	// The targets to probe.
	Targets []ProbeTarget `json:"targets"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
}

// ProbeTarget specifies targets to probe. Analogous to ScrapeEndpoint.
type ProbeTarget struct {
	// The probe module to use. See
	// https://github.com/prometheus/blackbox_exporter/blob/master/blackbox.yml for a list of all
	// possible probe modules.
	Module string `json:"module"`
	// The list of targets to probe. Each target must be represented as a hostname or IP followed by an optional port number.
	StaticTargets []string `json:"staticTargets"`
	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	// +kubebuilder:validation:Pattern="^((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)$"
	// +kubebuilder:default="1m"
	Interval string `json:"interval,omitempty"`
	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	// Must not be larger then the scrape interval.
	Timeout string `json:"timeout,omitempty"`
	// Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
	// override protected target labels (project_id, location, cluster, namespace, job,
	// instance, or __address__) are not permitted. The labelmap action is not permitted
	// in general.
	MetricRelabeling []RelabelingRule `json:"metricRelabeling,omitempty"`
	// Prometheus HTTP client configuration.
	HTTPClientConfig `json:",inline"`
}

// ProbeStatus holds status information of a Probe resource.
type ProbeStatus struct {
	MonitoringStatus `json:",inline"`
	// Represents the latest available observations of target state for each ProbeTarget.
	EndpointStatuses []ScrapeEndpointStatus `json:"endpointStatuses,omitempty"`
}
