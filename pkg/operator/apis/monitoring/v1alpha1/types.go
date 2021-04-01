package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PodMonitoring defines monitoring for a set of pods.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired Pod selection for target discovery by
	// Prometheus.
	Spec PodMonitoringSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status PodMonitoringStatus `json:"status"`
}

// PodMonitoringList is a list of PodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMonitoring `json:"items"`
}

// MonitoringConditionType is the type of MonitoringCondition.
type MonitoringConditionType string

const (
	// ConfigurationCreateSuccess indicates that the config generated from the
	// monitoring resource was created successfully.
	ConfigurationCreateSuccess MonitoringConditionType = "ConfigurationCreateSuccess"
)

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

// NewDefaultConditions returns a list of default conditions for at the given
// time for a `PodMonitoringStatus` if never explicitly set.
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

type PodMonitoringStatus struct {
	// The generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// Represents the latest available observations of a podmonitor's current state.
	Conditions []MonitoringCondition `json:"conditions,omitempty"`
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
	FromPod []LabelMapping `json:"fromPod,omitempty"`
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
