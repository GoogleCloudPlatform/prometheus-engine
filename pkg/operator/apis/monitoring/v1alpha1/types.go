package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceMonitoring defines monitoring for a set of services.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired Service selection for target discovery by
	// Prometheus.
	Spec ServiceMonitoringSpec `json:"spec"`
}

// ServiceMonitoringList is a list of ServiceMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceMonitoring `json:"items"`
}

// ServiceMonitoringSpec contains specification parameters for ServiceMonitoring.
type ServiceMonitoringSpec struct {
	Selector     metav1.LabelSelector `json:"selector"`
	Endpoints    []ScrapeEndpoint     `json:"endpoints"`
	TargetLabels TargetLabels         `json:"targetLabels,omitempty"`
}

// PodMonitoring defines monitoring for a set of pods.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired Pod selection for target discovery by
	// Prometheus.
	Spec PodMonitoringSpec `json:"spec"`
}

// PodMonitoringList is a list of PodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMonitoring `json:"items"`
}

// PodMonitoringSpec contains specification parameters for PodMonitoring.
type PodMonitoringSpec struct {
	Selector     metav1.LabelSelector `json:"selector"`
	Endpoints    []ScrapeEndpoint     `json:"endpoints"`
	TargetLabels TargetLabels         `json:"targetLabels,omitempty"`
}

// ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.
type ScrapeEndpoint struct {
	// Name or number of the port to scrape.
	// For ServiceMonitoring resources, only port names are allowed.
	Port intstr.IntOrString `json:"port,omitempty"`
	// HTTP path to scrape metrics from. Defaults to "/metrics".
	Path string `json:"path,omitempty"`
	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	Interval string `json:"interval,omitempty"`
	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	Timeout string `json:"timeout,omitempty"`
}

// TargetLabels groups label mappings by Kubernetes resource.
type TargetLabels struct {
	// Labels to transfer from the Kubernetes Pod to Prometheus target labels.
	// In the case of a label mapping conflict:
	// - Mappings at the end of the array take precedence.
	// - FromPod mappings take precedence over FromService mappings.
	FromPod []LabelMapping `json:"fromPod,omitempty"`
	// Labels to transfer from the Kubernetes Service to Prometheus target labels.
	FromService []LabelMapping `json:"fromService,omitempty"`
}

// LabelMapping specifies how to transfer a label from a Kubernetes resource
// onto a Prometheus target.
type LabelMapping struct {
	// Kubenetes resource label to remap.
	From string `json:"from"`
	// Remapped Prometheus target label.
	// Defaults to the same name as `From`.
	To string `json:"to,omitempty"`
}
