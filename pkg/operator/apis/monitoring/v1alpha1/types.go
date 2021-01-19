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
	Items           []*ServiceMonitoring `json:"items"`
}

// ServiceMonitoringSpec contains specification parameters for ServiceMonitoring.
type ServiceMonitoringSpec struct {
	Selector  metav1.LabelSelector `json:"selector"`
	Endpoints []ScrapeEndpoint     `json:"endpoints"`
}

// ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.
type ScrapeEndpoint struct {
	// Name or number of the port to scrape.
	// For ServiceMonitoring resources, only port names are allowed.
	Port *intstr.IntOrString `json:"port,omitempty"`

	// HTTP path to scrape metrics from. Defaults to "/metrics".
	Path string `json:"path,omitempty"`

	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	ScrapeInterval string `json:"scrapeInterval,omitempty"`

	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	ScrapeTimeout string `json:"scrapeTimeout,omitempty"`
}
