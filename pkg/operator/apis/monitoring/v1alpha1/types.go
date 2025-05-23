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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// OperatorConfig defines configuration of the gmp-operator.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:deprecatedversion
type OperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Rules specifies how the operator configures and deployes rule-evaluator.
	Rules RuleEvaluatorSpec `json:"rules,omitempty"`
	// Collection specifies how the operator configures collection.
	Collection CollectionSpec `json:"collection,omitempty"`
}

// OperatorConfigList is a list of OperatorConfigs.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperatorConfig `json:"items"`
}

// RuleEvaluatorSpec defines configuration for deploying rule-evaluator.
type RuleEvaluatorSpec struct {
	// ExternalLabels specifies external labels that are attached to any rule
	// results and alerts produced by rules. The precedence behavior matches that
	// of Prometheus.
	ExternalLabels map[string]string `json:"externalLabels,omitempty"`
	// QueryProjectID is the GCP project ID to evaluate rules against.
	// If left blank, the rule-evaluator will try attempt to infer the Project ID
	// from the environment.
	QueryProjectID string `json:"queryProjectID,omitempty"`
	// Alerting contains how the rule-evaluator configures alerting.
	Alerting AlertingSpec `json:"alerting,omitempty"`
	// A reference to GCP service account credentials with which the rule
	// evaluator container is run. It needs to have metric read permissions
	// against queryProjectId and metric write permissions against all projects
	// to which rule results are written.
	// Within GKE, this can typically be left empty if the compute default
	// service account has the required permissions.
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
}

// CollectionSpec specifies how the operator configures collection of metric data.
type CollectionSpec struct {
	// ExternalLabels specifies external labels that are attached to all scraped
	// data before being written to Cloud Monitoring. The precedence behavior matches that
	// of Prometheus.
	ExternalLabels map[string]string `json:"externalLabels,omitempty"`
	// Filter limits which metric data is sent to Cloud Monitoring.
	Filter ExportFilters `json:"filter,omitempty"`
	// A reference to GCP service account credentials with which Prometheus collectors
	// are run. It needs to have metric write permissions for all project IDs to which
	// data is written.
	// Within GKE, this can typically be left empty if the compute default
	// service account has the required permissions.
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
}

// ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.
type ExportFilters struct {
	// A list Prometheus time series matchers. Every time series must match at least one
	// of the matchers to be exported. This field can be used equivalently to the match[]
	// parameter of the Prometheus federation endpoint to selectively export data.
	// Example: `["{job='prometheus'}", "{__name__=~'job:.*'}"]`
	MatchOneOf []string `json:"matchOneOf,omitempty"`
}

// AlertingSpec defines alerting configuration.
type AlertingSpec struct {
	// Alertmanagers contains endpoint configuration for designated Alertmanagers.
	Alertmanagers []AlertmanagerEndpoints `json:"alertmanagers,omitempty"`
}

// AlertmanagerEndpoints defines a selection of a single Endpoints object
// containing alertmanager IPs to fire alerts against.
type AlertmanagerEndpoints struct {
	// Namespace of Endpoints object.
	Namespace string `json:"namespace"`
	// Name of Endpoints object in Namespace.
	Name string `json:"name"`
	// Port the Alertmanager API is exposed on.
	Port intstr.IntOrString `json:"port"`
	// Scheme to use when firing alerts.
	Scheme string `json:"scheme,omitempty"`
	// Prefix for the HTTP path alerts are pushed to.
	PathPrefix string `json:"pathPrefix,omitempty"`
	// TLS Config to use for alertmanager connection.
	TLS *TLSConfig `json:"tls,omitempty"`
	// Authorization section for this alertmanager endpoint
	Authorization *Authorization `json:"authorization,omitempty"`
	// Version of the Alertmanager API that rule-evaluator uses to send alerts. It
	// can be "v1" or "v2".
	APIVersion string `json:"apiVersion,omitempty"`
	// Timeout is a per-target Alertmanager timeout when pushing alerts.
	Timeout string `json:"timeout,omitempty"`
}

// Authorization specifies a subset of the Authorization struct, that is
// safe for use in Endpoints (no CredentialsFile field).
type Authorization struct {
	// Set the authentication type. Defaults to Bearer, Basic will cause an
	// error
	Type string `json:"type,omitempty"`
	// The secret's key that contains the credentials of the request
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
}

// SafeTLSConfig specifies TLS configuration parameters from Kubernetes resources.
type TLSConfig struct {
	// Struct containing the CA cert to use for the targets.
	CA *SecretOrConfigMap `json:"ca,omitempty"`
	// Struct containing the client cert file for the targets.
	Cert *SecretOrConfigMap `json:"cert,omitempty"`
	// Secret containing the client key file for the targets.
	KeySecret *corev1.SecretKeySelector `json:"keySecret,omitempty"`
	// Used to verify the hostname for the targets.
	ServerName string `json:"serverName,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive.
// Taking inspiration from prometheus-operator: https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap
type SecretOrConfigMap struct {
	// Secret containing data to use for the targets.
	Secret *corev1.SecretKeySelector `json:"secret,omitempty"`
	// ConfigMap containing data to use for the targets.
	ConfigMap *corev1.ConfigMapKeySelector `json:"configMap,omitempty"`
}

// PodMonitoring defines monitoring for a set of pods.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion
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

// ClusterPodMonitoring defines monitoring for a set of pods.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion
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

// ClusterPodMonitoringList is a list of ClusterPodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterPodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPodMonitoring `json:"items"`
}

// PodMonitoringSpec contains specification parameters for PodMonitoring.
type PodMonitoringSpec struct {
	// Label selector that specifies which pods are selected for this monitoring
	// configuration.
	Selector metav1.LabelSelector `json:"selector"`
	// The endpoints to scrape on the selected pods.
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints.
	TargetLabels TargetLabels `json:"targetLabels,omitempty"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
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

// ClusterPodMonitoringSpec contains specification parameters for PodMonitoring.
type ClusterPodMonitoringSpec struct {
	// Label selector that specifies which pods are selected for this monitoring
	// configuration.
	Selector metav1.LabelSelector `json:"selector"`
	// The endpoints to scrape on the selected pods.
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints
	TargetLabels TargetLabels `json:"targetLabels,omitempty"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
}

// ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.
type ScrapeEndpoint struct {
	// Name or number of the port to scrape.
	Port intstr.IntOrString `json:"port"`
	// Protocol scheme to use to scrape.
	Scheme string `json:"scheme,omitempty"`
	// HTTP path to scrape metrics from. Defaults to "/metrics".
	Path string `json:"path,omitempty"`
	// HTTP GET params to use when scraping.
	Params map[string][]string `json:"params,omitempty"`
	// Proxy URL to scrape through. Encoded passwords are not supported.
	ProxyURL string `json:"proxyUrl,omitempty"`
	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	Interval string `json:"interval,omitempty"`
	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	// Must not be larger then the scrape interval.
	Timeout string `json:"timeout,omitempty"`
	// Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
	// override protected target labels (project_id, location, cluster, namespace, job,
	// instance, or __address__) are not permitted. The labelmap action is not permitted
	// in general.
	MetricRelabeling []RelabelingRule `json:"metricRelabeling,omitempty"`
}

// TargetLabels configures labels for the discovered Prometheus targets.
type TargetLabels struct {
	// Pod metadata labels that are set on all scraped targets.
	// Permitted keys are `pod`, `container`, and `node` for PodMonitoring and
	// `pod`, `container`, `node`, and `namespace` for ClusterPodMonitoring.
	// Defaults to [pod, container] for PodMonitoring and [namespace, pod, container]
	// for ClusterPodMonitoring.
	// If set to null, it will be interpreted as the empty list for PodMonitoring
	// and to [namespace] for ClusterPodMonitoring. This is for backwards-compatibility
	// only.
	Metadata *[]string `json:"metadata,omitempty"`
	// Labels to transfer from the Kubernetes Pod to Prometheus target labels.
	// Mappings are applied in order.
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

// RelabelingRule defines a single Prometheus relabeling rule.
type RelabelingRule struct {
	// The source labels select values from existing labels. Their content is concatenated
	// using the configured separator and matched against the configured regular expression
	// for the replace, keep, and drop actions.
	SourceLabels []string `json:"sourceLabels,omitempty"`
	// Separator placed between concatenated source label values. Defaults to ';'.
	Separator string `json:"separator,omitempty"`
	// Label to which the resulting value is written in a replace action.
	// It is mandatory for replace actions. Regex capture groups are available.
	TargetLabel string `json:"targetLabel,omitempty"`
	// Regular expression against which the extracted value is matched. Defaults to '(.*)'.
	Regex string `json:"regex,omitempty"`
	// Modulus to take of the hash of the source label values.
	Modulus uint64 `json:"modulus,omitempty"`
	// Replacement value against which a regex replace is performed if the
	// regular expression matches. Regex capture groups are available. Defaults to '$1'.
	Replacement string `json:"replacement,omitempty"`
	// Action to perform based on regex matching. Defaults to 'replace'.
	Action string `json:"action,omitempty"`
}

// PodMonitoringStatus holds status information of a PodMonitoring resource.
type PodMonitoringStatus struct {
	// The generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// Represents the latest available observations of a podmonitor's current state.
	Conditions []MonitoringCondition `json:"conditions,omitempty"`
}

// MonitoringConditionType is the type of MonitoringCondition.
type MonitoringConditionType string

const (
	// ConfigurationCreateSuccess indicates that the config generated from the
	// monitoring resource was created successfully.
	ConfigurationCreateSuccess MonitoringConditionType = "ConfigurationCreateSuccess"
)

// MonitoringCondition describes a condition of a PodMonitoring.
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

// Rules defines Prometheus alerting and recording rules that are scoped
// to the namespace of the resource. Only metric data from this namespace is processed
// and all rule results have their project_id, cluster, and namespace label preserved
// for query processing.
// If the location label is not preserved by the rule, it defaults to the cluster's location.
//
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion
type Rules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

// RulesList is a list of Rules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rules `json:"items"`
}

// ClusterRules defines Prometheus alerting and recording rules that are scoped
// to the current cluster. Only metric data from the current cluster is processed
// and all rule results have their project_id and cluster label preserved
// for query processing.
// If the location label is not preserved by the rule, it defaults to the cluster's location.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion
type ClusterRules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

// ClusterRulesList is a list of ClusterRules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterRulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRules `json:"items"`
}

// GlobalRules defines Prometheus alerting and recording rules that are scoped
// to all data in the queried project.
// If the project_id or location labels are not preserved by the rule, they default to
// the values of the cluster.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion
type GlobalRules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

// GlobalRulesList is a list of GlobalRules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GlobalRulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalRules `json:"items"`
}

// RulesSpec contains specification parameters for a Rules resource.
type RulesSpec struct {
	// A list of Prometheus rule groups.
	Groups []RuleGroup `json:"groups"`
}

// RuleGroup declares rules in the Prometheus format:
// https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
type RuleGroup struct {
	// The name of the rule group.
	Name string `json:"name"`
	// The interval at which to evaluate the rules. Must be a valid Prometheus duration.
	Interval string `json:"interval"`
	// A list of rules that are executed sequentially as part of this group.
	Rules []Rule `json:"rules"`
}

// Rule is a single rule in the Prometheus format:
// https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
type Rule struct {
	// Record the result of the expression to this metric name.
	// Only one of `record` and `alert` must be set.
	Record string `json:"record,omitempty"`
	// Name of the alert to evaluate the expression as.
	// Only one of `record` and `alert` must be set.
	Alert string `json:"alert,omitempty"`
	// The PromQL expression to evaluate.
	Expr string `json:"expr"`
	// The duration to wait before a firing alert produced by this rule is sent to Alertmanager.
	// Only valid if `alert` is set.
	For string `json:"for,omitempty"`
	// A set of labels to attach to the result of the query expression.
	Labels map[string]string `json:"labels,omitempty"`
	// A set of annotations to attach to alerts produced by the query expression.
	// Only valid if `alert` is set.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RulesStatus contains status information for a Rules resource.
type RulesStatus struct {
	// TODO: add status information.
}
