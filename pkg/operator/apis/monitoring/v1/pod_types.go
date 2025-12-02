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
	"fmt"

	prommodel "github.com/prometheus/common/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PodMonitoringCRD represents a Kubernetes CRD that monitors Pod endpoints.
type PodMonitoringCRD interface {
	MonitoringCRD

	// IsNamespaceScoped returns true for PodMonitoring and false for ClusterPodMonitoring.
	// This is used for namespace tenancy isolation (e.g. for secrets).
	IsNamespaceScoped() bool

	// GetKey returns a unique identifier for this CRD.
	GetKey() string

	// GetEndpoints returns the endpoints scraped by this CRD.
	GetEndpoints() []ScrapeEndpoint

	// GetPodMonitoringStatus returns this CRD's status sub-resource, which must
	// be available at the top-level.
	GetPodMonitoringStatus() *PodMonitoringStatus
}

// PodMonitoring defines monitoring for a set of pods, scoped to pods
// within the PodMonitoring's namespace.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.authorization) || !has(e.authorization.credentials) || !has(e.authorization.credentials.secret) || !has(e.authorization.credentials.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.basicAuth) || !has(e.basicAuth.password) || !has(e.basicAuth.password.secret) || !has(e.basicAuth.password.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.tls) || !has(e.tls.ca) || !has(e.tls.ca.secret) || !has(e.tls.ca.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.tls) || !has(e.tls.cert) || !has(e.tls.cert.secret) || !has(e.tls.cert.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.tls) || !has(e.tls.key) || !has(e.tls.key.secret) || !has(e.tls.key.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
// +kubebuilder:validation:XValidation:rule="self.spec.endpoints.all(e, !has(e.oauth2) || !has(e.oauth2.clientSecret) || !has(e.oauth2.clientSecret.secret) || !has(e.oauth2.clientSecret.secret.__namespace__))",message="Namespace not allowed on PodMonitoring secret references.",reason="FieldValueForbidden"
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

func (p *PodMonitoring) IsNamespaceScoped() bool {
	return true
}

func (p *PodMonitoring) GetKey() string {
	return fmt.Sprintf("PodMonitoring/%s/%s", p.Namespace, p.Name)
}

func (p *PodMonitoring) GetEndpoints() []ScrapeEndpoint {
	return p.Spec.Endpoints
}

func (p *PodMonitoring) GetPodMonitoringStatus() *PodMonitoringStatus {
	return &p.Status
}

func (p *PodMonitoring) GetMonitoringStatus() *MonitoringStatus {
	return &p.Status.MonitoringStatus
}

// PodMonitoringList is a list of PodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PodMonitoring `json:"items"`
}

// ClusterPodMonitoring defines monitoring for a set of pods, scoped to all
// pods within the cluster.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type ClusterPodMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of desired Pod selection for target discovery by
	// Prometheus.
	Spec ClusterPodMonitoringSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status PodMonitoringStatus `json:"status"`
}

func (c *ClusterPodMonitoring) IsNamespaceScoped() bool {
	return false
}

func (c *ClusterPodMonitoring) GetKey() string {
	return fmt.Sprintf("ClusterPodMonitoring/%s", c.Name)
}

func (c *ClusterPodMonitoring) GetEndpoints() []ScrapeEndpoint {
	return c.Spec.Endpoints
}

func (c *ClusterPodMonitoring) GetPodMonitoringStatus() *PodMonitoringStatus {
	return &c.Status
}

func (c *ClusterPodMonitoring) GetMonitoringStatus() *MonitoringStatus {
	return &c.Status.MonitoringStatus
}

// ClusterPodMonitoringList is a list of ClusterPodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterPodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ClusterPodMonitoring `json:"items"`
}

// PodMonitoringSpec contains specification parameters for PodMonitoring.
type PodMonitoringSpec struct {
	// Label selector that specifies which pods are selected for this monitoring
	// configuration.
	Selector metav1.LabelSelector `json:"selector"`
	// The endpoints to scrape on the selected pods.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints.
	// The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`
	// if the scraped pod is controlled by a DaemonSet.
	// +optional
	// +default:value={}
	TargetLabels TargetLabels `json:"targetLabels"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
	// FilterRunning will drop any pods that are in the "Failed" or "Succeeded"
	// pod lifecycle.
	// See: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
	FilterRunning *bool `json:"filterRunning,omitempty"`
}

// ScrapeLimits limits applied to scraped targets.
type ScrapeLimits struct {
	// Maximum number of samples accepted within a single scrape.
	// Uses Prometheus default if left unspecified.
	Samples uint64 `json:"samples,omitempty"`
	// Maximum number of labels accepted for a single sample.
	// Uses Prometheus default if left unspecified.
	Labels uint64 `json:"labels,omitempty"`
	// Maximum label name length.
	// Uses Prometheus default if left unspecified.
	LabelNameLength uint64 `json:"labelNameLength,omitempty"`
	// Maximum label value length.
	// Uses Prometheus default if left unspecified.
	LabelValueLength uint64 `json:"labelValueLength,omitempty"`
}

// ClusterPodMonitoringSpec contains specification parameters for ClusterPodMonitoring.
type ClusterPodMonitoringSpec struct {
	// Label selector that specifies which pods are selected for this monitoring
	// configuration.
	Selector metav1.LabelSelector `json:"selector"`
	// The endpoints to scrape on the selected pods.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints.
	// The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`
	// if the scraped pod is controlled by a DaemonSet.
	// +optional
	// +default:value={}
	TargetLabels ClusterTargetLabels `json:"targetLabels,omitempty"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
	// FilterRunning will drop any pods that are in the "Failed" or "Succeeded"
	// pod lifecycle.
	// See: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
	// Specifically, this prevents scraping Succeeded pods from K8s jobs, which
	// could contribute to noisy logs or irrelevant metrics.
	// Additionally, it can mitigate issues with reusing stale target
	// labels in cases where Pod IPs are reused (e.g. spot containers).
	// See: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/145
	FilterRunning *bool `json:"filterRunning,omitempty"`
}

// ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.
// +kubebuilder:validation:XValidation:rule="!has(self.timeout) || self.timeout <= self.interval",messageExpression="'scrape timeout (%s) must not be greater than scrape interval (%s)'.format([self.timeout, self.interval])"
type ScrapeEndpoint struct {
	// Prometheus HTTP client configuration.
	HTTPClientConfig `json:",inline"`

	// Name or number of the port to scrape.
	// The container metadata label is only populated if the port is referenced by name
	// because port numbers are not unique across containers.
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self != 0",message="Port is required"
	// +required
	Port intstr.IntOrString `json:"port,omitempty"`
	// Protocol scheme to use to scrape.
	// +kubebuilder:validation:Enum=http;https
	Scheme string `json:"scheme,omitempty"`
	// HTTP path to scrape metrics from. Defaults to "/metrics".
	Path string `json:"path,omitempty"`
	// HTTP GET params to use when scraping.
	Params map[string][]string `json:"params,omitempty"`
	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	// +kubebuilder:validation:Format=duration
	// +required
	Interval string `json:"interval"`
	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	// Must not be larger than the scrape interval.
	// +kubebuilder:validation:Format=duration
	Timeout string `json:"timeout,omitempty"`
	// Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
	// override protected target labels (project_id, location, cluster, namespace, job,
	// instance, top_level_controller, top_level_controller_type, or __address__) are
	// not permitted. The labelmap action is not permitted in general.
	// +kubebuilder:validation:MaxItems=250
	MetricRelabeling []RelabelingRule `json:"metricRelabeling,omitempty"`
}

// TargetLabels configures labels for the discovered Prometheus targets.
type TargetLabels struct {
	// Pod metadata labels that are set on all scraped targets.
	// Permitted keys are `container`, `node`, `pod`, `top_level_controller_name`,
	// and `top_level_controller_type`. The `container`
	// label is only populated if the scrape port is referenced by name.
	// Defaults to [container, pod, top_level_controller_name, top_level_controller_type].
	// If set to null, it will be interpreted as the empty list.
	// This is for backwards-compatibility only.
	// +kubebuilder:validation:items:Enum=container;node;pod;top_level_controller_name;top_level_controller_type
	// +listType=set
	// +optional
	// +kubebuilder:default=container;pod;top_level_controller_name;top_level_controller_type
	Metadata *[]string `json:"metadata"`
	// Labels to transfer from the Kubernetes Pod to Prometheus target labels.
	// Mappings are applied in order.
	// +kubebuilder:validation:MaxItems=100
	FromPod []LabelMapping `json:"fromPod,omitempty"`
}

// ClusterTargetLabels configures labels for the discovered Prometheus targets.
type ClusterTargetLabels struct {
	// Pod metadata labels that are set on all scraped targets.
	// Permitted keys are `container`, `namespace`, `node`, `pod`,
	// `top_level_controller_name` and `top_level_controller_type`. The `container`
	// label is only populated if the scrape port is referenced by name.
	// Defaults to [container, namespace, pod, top_level_controller_name, top_level_controller_type].
	// If set to null, it will be interpreted as  [namespace]. This is for backwards-compatibility
	// only.
	// +kubebuilder:validation:items:Enum=container;namespace;node;pod;top_level_controller_name;top_level_controller_type
	// +listType=set
	// +optional
	// +kubebuilder:default=container;namespace;pod;top_level_controller_name;top_level_controller_type
	Metadata *[]string `json:"metadata,omitempty"`
	// Labels to transfer from the Kubernetes Pod to Prometheus target labels.
	// Mappings are applied in order.
	// +kubebuilder:validation:MaxItems=100
	FromPod []LabelMapping `json:"fromPod,omitempty"`
}

// LabelMapping specifies how to transfer a label from a Kubernetes resource
// onto a Prometheus target.
type LabelMapping struct {
	// Kubernetes resource label to remap.
	From string `json:"from"`
	// Remapped Prometheus target label.
	// Defaults to the same name as `From`.
	// +kubebuilder:validation:Pattern=^[a-zA-Z_][a-zA-Z0-9_]*$
	// +kubebuilder:validation:MaxLength:100
	// +kubebuilder:validation:XValidation:rule="self != 'project_id' && self != 'location' && self != 'cluster' && self != 'namespace' && self != 'job' && self != 'instance' && self != 'top_level_controller' && self != 'top_level_controller_type' && self != '__address__'",messageExpression="'cannot relabel onto protected label \"%s\"'.format([self])"
	To string `json:"to,omitempty"`
}

// RelabelingRule defines a single Prometheus relabeling rule.
// +kubebuilder:validation:XValidation:rule="!has(self.action) ||  self.action != 'labeldrop' || has(self.regex)"
type RelabelingRule struct {
	// The source labels select values from existing labels. Their content is concatenated
	// using the configured separator and matched against the configured regular expression
	// for the replace, keep, and drop actions.
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:Pattern=^[a-zA-Z_][a-zA-Z0-9_]*$
	SourceLabels []string `json:"sourceLabels,omitempty"`
	// Separator placed between concatenated source label values. Defaults to ';'.
	Separator string `json:"separator,omitempty"`
	// Label to which the resulting value is written in a replace action.
	// It is mandatory for replace actions. Regex capture groups are available.
	// +kubebuilder:validation:Pattern=^[a-zA-Z_][a-zA-Z0-9_]*$
	// +kubebuilder:validation:MaxLength:100
	// +kubebuilder:validation:XValidation:rule="self != 'project_id' && self != 'location' && self != 'cluster' && self != 'namespace' && self != 'job' && self != 'instance' && self != 'top_level_controller' && self != 'top_level_controller_type' && self != '__address__'",messageExpression="'cannot relabel onto protected label \"%s\"'.format([self])"
	TargetLabel string `json:"targetLabel,omitempty"`
	// Regular expression against which the extracted value is matched. Defaults to '(.*)'.
	// +kubebuilder:validation:MaxLength=10000
	Regex string `json:"regex,omitempty"`
	// Modulus to take of the hash of the source label values.
	Modulus uint64 `json:"modulus,omitempty"`
	// Replacement value against which a regex replace is performed if the
	// regular expression matches. Regex capture groups are available. Defaults to '$1'.
	Replacement string `json:"replacement,omitempty"`
	// Action to perform based on regex matching. Defaults to 'replace'.
	// +kubebuilder:validation:Enum=replace;lowercase;uppercase;keep;drop;keepequal;dropequal;hashmod;labeldrop;labelkeep
	Action string `json:"action,omitempty"`
}

type ScrapeEndpointStatus struct {
	// The name of the ScrapeEndpoint.
	Name string `json:"name"`
	// Total number of active targets.
	ActiveTargets int64 `json:"activeTargets,omitempty"`
	// Total number of active, unhealthy targets.
	UnhealthyTargets int64 `json:"unhealthyTargets,omitempty"`
	// Last time this status was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// A fixed sample of targets grouped by error type.
	SampleGroups []SampleGroup `json:"sampleGroups,omitempty"`
	// Fraction of collectors included in status, bounded [0,1].
	// Ideally, this should always be 1. Anything less can
	// be considered a problem and should be investigated.
	CollectorsFraction string `json:"collectorsFraction,omitempty"`
}

type SampleGroup struct {
	// Targets emitting the error message.
	SampleTargets []SampleTarget `json:"sampleTargets,omitempty"`
	// Total count of similar errors.
	// +optional
	Count *int32 `json:"count,omitempty"`
}

type SampleTarget struct {
	// The label set, keys and values, of the target.
	Labels prommodel.LabelSet `json:"labels,omitempty"`
	// Error message.
	LastError *string `json:"lastError,omitempty"`
	// Scrape duration in seconds.
	LastScrapeDurationSeconds string `json:"lastScrapeDurationSeconds,omitempty"`
	// Health status.
	Health string `json:"health,omitempty"`
}

// PodMonitoringStatus holds status information of a PodMonitoring resource.
type PodMonitoringStatus struct {
	MonitoringStatus `json:",inline"`

	// Represents the latest available observations of target state for each ScrapeEndpoint.
	EndpointStatuses []ScrapeEndpointStatus `json:"endpointStatuses,omitempty"`
}
