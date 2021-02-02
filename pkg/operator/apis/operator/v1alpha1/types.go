package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// ServiceMonitoringSpec contains specification parameters for ServiceMonitoring.
type ServiceMonitoringSpec struct {
	// TODO(freinartz): populate with proper fields.
	Test string `json:"test`
}

// ServiceMonitoringList is a list of ServiceMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*ServiceMonitoring `json:"items"`
}
