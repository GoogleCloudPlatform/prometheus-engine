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
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/relabel"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
)

var (
	errInvalidCond = fmt.Errorf("condition needs both 'Type' and 'Status' fields set")
)

// OperatorConfig defines configuration of the gmp-operator.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion
type OperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Rules specifies how the operator configures and deployes rule-evaluator.
	Rules RuleEvaluatorSpec `json:"rules,omitempty"`
	// Collection specifies how the operator configures collection.
	Collection CollectionSpec `json:"collection,omitempty"`
	// ManagedAlertmanager holds information for configuring the managed instance of Alertmanager.
	// +kubebuilder:default={configSecret: {name: alertmanager, key: alertmanager.yaml}}
	ManagedAlertmanager *ManagedAlertmanagerSpec `json:"managedAlertmanager,omitempty"`
	// Features holds configuration for optional managed-collection features.
	Features OperatorFeatures `json:"features,omitempty"`
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
	// The base URL used for the generator URL in the alert notification payload.
	// Should point to an instance of a query frontend that gives access to queryProjectID.
	GeneratorURL string `json:"generatorUrl,omitempty"`
	// Alerting contains how the rule-evaluator configures alerting.
	Alerting AlertingSpec `json:"alerting,omitempty"`
	// A reference to GCP service account credentials with which the rule
	// evaluator container is run. It needs to have metric read permissions
	// against queryProjectId and metric write permissions against all projects
	// to which rule results are written.
	// Within GKE, this can typically be left empty if the compute default
	// service account has the required permissions.
	Credentials *v1.SecretKeySelector `json:"credentials,omitempty"`
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
	Credentials *v1.SecretKeySelector `json:"credentials,omitempty"`
	// Configuration to scrape the metric endpoints of the Kubelets.
	KubeletScraping *KubeletScraping `json:"kubeletScraping,omitempty"`
	// Compression enables compression of metrics collection data
	Compression CompressionType `json:"compression,omitempty"`
}

// OperatorFeatures holds configuration for optional managed-collection features.
type OperatorFeatures struct {
	// Configuration of target status reporting.
	TargetStatus TargetStatusSpec `json:"targetStatus,omitempty"`
	// Settings for the collector configuration propagation.
	Config ConfigSpec `json:"config,omitempty"`
}

// ConfigSpec holds configurations for the Prometheus configuration.
type ConfigSpec struct {
	// Compression enables compression of the config data propagated by the operator to collectors.
	// It is recommended to use the gzip option when using a large number of ClusterPodMonitoring
	// and/or PodMonitoring.
	Compression CompressionType `json:"compression,omitempty"`
}

// TargetStatusSpec holds configuration for target status reporting.
type TargetStatusSpec struct {
	// Enable target status reporting.
	Enabled bool `json:"enabled,omitempty"`
}

// +kubebuilder:validation:Enum=none;gzip
type CompressionType string

const CompressionNone CompressionType = "none"
const CompressionGzip CompressionType = "gzip"

// KubeletScraping allows enabling scraping of the Kubelets' metric endpoints.
type KubeletScraping struct {
	// The interval at which the metric endpoints are scraped.
	Interval string `json:"interval"`
}

// ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.
type ExportFilters struct {
	// A list Prometheus time series matchers. Every time series must match at least one
	// of the matchers to be exported. This field can be used equivalently to the match[]
	// parameter of the Prometheus federation endpoint to selectively export data.
	// Example: `["{job!='foobar'}", "{__name__!~'container_foo.*|container_bar.*'}"]`
	MatchOneOf []string `json:"matchOneOf,omitempty"`
}

// AlertingSpec defines alerting configuration.
type AlertingSpec struct {
	// Alertmanagers contains endpoint configuration for designated Alertmanagers.
	Alertmanagers []AlertmanagerEndpoints `json:"alertmanagers,omitempty"`
}

// ManagedAlertmanagerSpec holds configuration information for the managed
// Alertmanager instance.
type ManagedAlertmanagerSpec struct {
	// ConfigSecret refers to the name of a single-key Secret in the public namespace that
	// holds the managed Alertmanager config file.
	ConfigSecret *v1.SecretKeySelector `json:"configSecret,omitempty"`
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
	Credentials *v1.SecretKeySelector `json:"credentials,omitempty"`
}

// TLS specifies TLS configuration parameters from Kubernetes resources.
type TLS struct {
	// Used to verify the hostname for the targets.
	ServerName string `json:"serverName,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MinVersion string `json:"minVersion,omitempty"`
	// Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MaxVersion string `json:"maxVersion,omitempty"`
}

// TLSConfig specifies TLS configuration parameters from Kubernetes resources.
type TLSConfig struct {
	// Struct containing the CA cert to use for the targets.
	CA *SecretOrConfigMap `json:"ca,omitempty"`
	// Struct containing the client cert file for the targets.
	Cert *SecretOrConfigMap `json:"cert,omitempty"`
	// Secret containing the client key file for the targets.
	KeySecret *v1.SecretKeySelector `json:"keySecret,omitempty"`
	// Used to verify the hostname for the targets.
	ServerName string `json:"serverName,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MinVersion string `json:"minVersion,omitempty"`
	// Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MaxVersion string `json:"maxVersion,omitempty"`
}

// SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive.
// Taking inspiration from prometheus-operator: https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap
type SecretOrConfigMap struct {
	// Secret containing data to use for the targets.
	Secret *v1.SecretKeySelector `json:"secret,omitempty"`
	// ConfigMap containing data to use for the targets.
	ConfigMap *v1.ConfigMapKeySelector `json:"configMap,omitempty"`
}

// PodMonitoringStatusContainer represents a Kubernetes CRD that monitors pods
// and contains a status sub-resource.
type PodMonitoringStatusContainer interface {
	client.Object

	// Returns this CRD's status sub-resource.
	GetStatus() *PodMonitoringStatus
}

// PodMonitoring defines monitoring for a set of pods, scoped to pods
// within the PodMonitoring's namespace.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
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

func (p *PodMonitoring) GetKey() string {
	return fmt.Sprintf("PodMonitoring/%s/%s", p.Namespace, p.Name)
}

func (p *PodMonitoring) GetStatus() *PodMonitoringStatus {
	return &p.Status
}

// PodMonitoringList is a list of PodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMonitoring `json:"items"`
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

func (p *ClusterPodMonitoring) GetKey() string {
	return fmt.Sprintf("ClusterPodMonitoring/%s", p.Name)
}

func (p *ClusterPodMonitoring) GetStatus() *PodMonitoringStatus {
	return &p.Status
}

// ClusterPodMonitoringList is a list of ClusterPodMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterPodMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPodMonitoring `json:"items"`
}

func (cm *ClusterPodMonitoring) ValidateCreate() error {
	if len(cm.Spec.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}
	// TODO(freinartz): extract validator into dedicated object (like defaulter). For now using
	// example values has no adverse effects.
	_, err := cm.ScrapeConfigs("test_project", "test_location", "test_cluster")
	return err
}

func (cm *ClusterPodMonitoring) ValidateUpdate(old runtime.Object) error {
	// Validity does not depend on state changes.
	return cm.ValidateCreate()
}

func (cm *ClusterPodMonitoring) ValidateDelete() error {
	// Deletions are always valid.
	return nil
}

func (cm *ClusterPodMonitoring) ScrapeConfigs(projectID, location, cluster string) (res []*promconfig.ScrapeConfig, err error) {
	for i := range cm.Spec.Endpoints {
		c, err := cm.endpointScrapeConfig(i, projectID, location, cluster)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, nil
}

func (pm *PodMonitoring) ValidateCreate() error {
	if len(pm.Spec.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}
	// TODO(freinartz): extract validator into dedicated object (like defaulter). For now using
	// example values has no adverse effects.
	_, err := pm.ScrapeConfigs("test_project", "test_location", "test_cluster")
	return err
}

func (pm *PodMonitoring) ValidateUpdate(old runtime.Object) error {
	// Validity does not depend on state changes.
	return pm.ValidateCreate()
}

func (pm *PodMonitoring) ValidateDelete() error {
	// Deletions are always valid.
	return nil
}

// ScrapeConfigs generated Prometheus scrape configs for the PodMonitoring.
func (pm *PodMonitoring) ScrapeConfigs(projectID, location, cluster string) (res []*promconfig.ScrapeConfig, err error) {
	for i := range pm.Spec.Endpoints {
		c, err := pm.endpointScrapeConfig(i, projectID, location, cluster)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, nil
}

// SetPodMonitoringCondition merges the provided PodMonitoring resource to the
// along with the provided condition iff the resource generation has changed or there
// is a status condition state transition.
func (status *PodMonitoringStatus) SetPodMonitoringCondition(gen int64, now metav1.Time, cond *MonitoringCondition) (bool, error) {
	var (
		specChanged              = status.ObservedGeneration != gen
		statusTransition, update bool
		conds                    = make(map[MonitoringConditionType]*MonitoringCondition)
	)

	if cond.Type == "" || cond.Status == "" {
		return update, errInvalidCond
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
	if old := conds[cond.Type]; old.Status == cond.Status {
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

	return update, nil
}

// Environment variable for the current node that needs to be interpolated in generated
// scrape configurations for a PodMonitoring resource.
const EnvVarNodeName = "NODE_NAME"

func (pm *PodMonitoring) endpointScrapeConfig(index int, projectID, location, cluster string) (*promconfig.ScrapeConfig, error) {
	relabelCfgs := []*relabel.Config{
		// Filter targets by namespace of the PodMonitoring configuration.
		{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
			Regex:        relabel.MustNewRegexp(pm.Namespace),
		},
	}

	// Filter targets that belong to selected pods.
	selectors, err := relabelingsForSelector(pm.Spec.Selector)
	if err != nil {
		return nil, err
	}
	relabelCfgs = append(relabelCfgs, selectors...)

	metadataLabels := map[string]struct{}{}
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility and won't add any labels in that case.
	if pm.Spec.TargetLabels.Metadata != nil {
		for _, l := range *pm.Spec.TargetLabels.Metadata {
			if allowed := []string{"pod", "container", "node"}; !containsString(allowed, l) {
				return nil, fmt.Errorf("metadata label %q not allowed, must be one of %v", l, allowed)
			}
			metadataLabels[l] = struct{}{}
		}
	}
	relabelCfgs = append(relabelCfgs, relabelingsForMetadata(metadataLabels)...)

	// The namespace label is always set for PodMonitorings.
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:       relabel.Replace,
		SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
		TargetLabel:  "namespace",
	})
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:      relabel.Replace,
		Replacement: pm.Name,
		TargetLabel: "job",
	})

	return endpointScrapeConfig(
		pm.GetKey(),
		projectID, location, cluster,
		pm.Spec.Endpoints[index],
		relabelCfgs,
		pm.Spec.TargetLabels.FromPod,
		pm.Spec.Limits,
	)
}

// relabelingsForSelector generates a sequence of relabeling rules that implement
// the label selector for the meta labels produced by the Kubernetes service discovery.
func relabelingsForSelector(selector metav1.LabelSelector) ([]*relabel.Config, error) {
	// Simple equal matchers. Sort by keys first to ensure that generated configs are reproducible.
	// (Go map iteration is non-deterministic.)
	var selectorKeys []string
	for k := range selector.MatchLabels {
		selectorKeys = append(selectorKeys, k)
	}
	sort.Strings(selectorKeys)

	var relabelCfgs []*relabel.Config

	for _, k := range selectorKeys {
		re, err := relabel.NewRegexp(selector.MatchLabels[k])
		if err != nil {
			return nil, err
		}
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(k)},
			Regex:        re,
		})
	}
	// Expression matchers are mapped to relabeling rules with the same behavior.
	for _, exp := range selector.MatchExpressions {
		switch exp.Operator {
		case metav1.LabelSelectorOpIn:
			re, err := relabel.NewRegexp(strings.Join(exp.Values, "|"))
			if err != nil {
				return nil, err
			}
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(exp.Key)},
				Regex:        re,
			})
		case metav1.LabelSelectorOpNotIn:
			re, err := relabel.NewRegexp(strings.Join(exp.Values, "|"))
			if err != nil {
				return nil, err
			}
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(exp.Key)},
				Regex:        re,
			})
		case metav1.LabelSelectorOpExists:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_labelpresent_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		case metav1.LabelSelectorOpDoesNotExist:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_labelpresent_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		}
	}

	return relabelCfgs, nil
}

func endpointScrapeConfig(id, projectID, location, cluster string, ep ScrapeEndpoint, relabelCfgs []*relabel.Config, podLabels []LabelMapping, limits *ScrapeLimits) (*promconfig.ScrapeConfig, error) {
	// Configure how Prometheus talks to the Kubernetes API server to discover targets.
	// This configuration is the same for all scrape jobs (esp. selectors).
	// This ensures that Prometheus can reuse the underlying client and caches, which reduces
	// load on the Kubernetes API server.
	discoveryCfgs := discovery.Configs{
		&discoverykube.SDConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			Role:             discoverykube.RolePod,
			// Drop all potential targets not the same node as the collector. The $(NODE_NAME) variable
			// is interpolated by the config reloader sidecar before the config reaches the Prometheus collector.
			// Doing it through selectors rather than relabeling should substantially reduce the client and
			// server side load.
			Selectors: []discoverykube.SelectorConfig{
				{
					Role:  discoverykube.RolePod,
					Field: fmt.Sprintf("spec.nodeName=$(%s)", EnvVarNodeName),
				},
			},
		},
	}

	relabelCfgs = append(relabelCfgs,
		// Force target labels so they cannot be overwritten by metric labels.
		&relabel.Config{
			Action:      relabel.Replace,
			TargetLabel: "project_id",
			Replacement: projectID,
		},
		&relabel.Config{
			Action:      relabel.Replace,
			TargetLabel: "location",
			Replacement: location,
		},
		&relabel.Config{
			Action:      relabel.Replace,
			TargetLabel: "cluster",
			Replacement: cluster,
		},
		// Use the pod name as the primary identifier in the instance label. Unless the pod
		// is controlled by a DaemonSet, in which case the node name will be used.
		// This provides a better user experience on dashboards which template on the instance label
		// and expect it to have meaningful value, such as common node exporter dashboards.
		//
		// Save the value in a temporary label and use it further down.
		&relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_name"},
			TargetLabel:  "__tmp_instance",
		},
		&relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind", "__meta_kubernetes_pod_node_name"},
			Regex:        relabel.MustNewRegexp(`DaemonSet;(.*)`),
			TargetLabel:  "__tmp_instance",
			Replacement:  "$1",
		},
	)

	// Filter targets by the configured port.
	if ep.Port.StrVal != "" {
		portValue, err := relabel.NewRegexp(ep.Port.StrVal)
		if err != nil {
			return nil, fmt.Errorf("invalid port name %q: %w", ep.Port, err)
		}
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_container_port_name"},
			Regex:        portValue,
		})
		// The instance label being the pod name would be ideal UX-wise. But we cannot be certain
		// that multiple metrics endpoints on a pod don't expose metrics with the same name. Thus
		// we have to disambiguate along the port as well.
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__tmp_instance", "__meta_kubernetes_pod_container_port_name"},
			Regex:        relabel.MustNewRegexp("(.+);(.+)"),
			Replacement:  "$1:$2",
			TargetLabel:  "instance",
		})
	} else if ep.Port.IntVal != 0 {
		// Prometheus generates a target candidate for each declared port in a pod.
		// If a container in a pod has no declared port, a single target candidate is generated for
		// that container.
		//
		// If a numeric port is specified for scraping but not declared in the pod, we still
		// want to allow scraping it. For that we must ensure that we produce a single final output
		// target for that numeric port. The only way to achieve this is to produce identical output
		// targets for all incoming target candidates for that pod and producing identical output
		// targets for each.
		// This requires leaving the container label empty (or at a singleton value) even if it is
		// requested as an output label via .targetLabels.metadata. This algins with the Pod specification,
		// which requires port names in a Pod to be unique but not port numbers. Thus the container is
		// potentially ambigious for numerical ports in any case.

		// First, drop the container label even it it was added before.
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action: relabel.LabelDrop,
			Regex:  relabel.MustNewRegexp("container"),
		})
		// Then, rewrite the instance and __address__ for each candidate to the same values.
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__tmp_instance"},
			Replacement:  fmt.Sprintf("$1:%d", ep.Port.IntVal),
			TargetLabel:  "instance",
		})
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_ip"},
			Replacement:  fmt.Sprintf("$1:%d", ep.Port.IntVal),
			TargetLabel:  "__address__",
		})
	} else {
		return nil, errors.New("port must be set")
	}

	// Add pod labels.
	if pCfgs, err := labelMappingRelabelConfigs(podLabels, "__meta_kubernetes_pod_label_"); err != nil {
		return nil, fmt.Errorf("invalid pod label mapping: %w", err)
	} else {
		relabelCfgs = append(relabelCfgs, pCfgs...)
	}

	interval, err := prommodel.ParseDuration(ep.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid scrape interval: %w", err)
	}
	timeout := interval
	if ep.Timeout != "" {
		timeout, err = prommodel.ParseDuration(ep.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid scrape timeout: %w", err)
		}
		if timeout > interval {
			return nil, fmt.Errorf("scrape timeout %v must not be greater than scrape interval %v", timeout, interval)
		}
	}

	metricsPath := "/metrics"
	if ep.Path != "" {
		metricsPath = ep.Path
	}

	var metricRelabelCfgs []*relabel.Config
	for _, r := range ep.MetricRelabeling {
		rcfg, err := convertRelabelingRule(r)
		if err != nil {
			return nil, err
		}
		metricRelabelCfgs = append(metricRelabelCfgs, rcfg)
	}

	httpCfg := config.DefaultHTTPClientConfig
	if ep.ProxyURL != "" {
		proxyURL, err := url.Parse(ep.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		// Marshalling the config will redact the password, so we don't support those.
		// It's not a good idea anyway and we will later support basic auth based on secrets to
		// cover the general use case.
		if _, ok := proxyURL.User.Password(); ok {
			return nil, errors.New("passwords encoded in URLs are not supported")
		}
		// Initialize from default as encode/decode does not work correctly with the type definition.
		httpCfg.ProxyURL.URL = proxyURL
	}

	if ep.HTTPClientConfig.TLS != nil {
		tlsConfig, err := ep.HTTPClientConfig.TLS.ToPrometheusConfig()
		if err != nil {
			return nil, err
		}
		httpCfg.TLSConfig = *tlsConfig
	}

	if err := httpCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Prometheus HTTP client config: %w", err)
	}

	scrapeCfg := &promconfig.ScrapeConfig{
		// Generate a job name to make it easy to track what generated the scrape configuration.
		// The actual job label attached to its metrics is overwritten via relabeling.
		JobName:                 fmt.Sprintf("%s/%s", id, &ep.Port),
		ServiceDiscoveryConfigs: discoveryCfgs,
		MetricsPath:             metricsPath,
		Scheme:                  ep.Scheme,
		Params:                  ep.Params,
		HTTPClientConfig:        httpCfg,
		ScrapeInterval:          interval,
		ScrapeTimeout:           timeout,
		RelabelConfigs:          relabelCfgs,
		MetricRelabelConfigs:    metricRelabelCfgs,
	}
	if limits != nil {
		scrapeCfg.SampleLimit = uint(limits.Samples)
		scrapeCfg.LabelLimit = uint(limits.Labels)
		scrapeCfg.LabelNameLengthLimit = uint(limits.LabelNameLength)
		scrapeCfg.LabelValueLengthLimit = uint(limits.LabelValueLength)
	}
	// The Prometheus configuration structs do not generally have validation methods and embed their
	// validation logic in the UnmarshalYAML methods. To keep things reasonable we don't re-validate
	// everything and simply do a final marshal-unmarshal cycle at the end to run all validation
	// upstream provides at the end of this method.
	b, err := yaml.Marshal(scrapeCfg)
	if err != nil {
		return nil, fmt.Errorf("scrape config cannot be marshalled: %w", err)
	}
	var scrapeCfgCopy promconfig.ScrapeConfig
	if err := yaml.Unmarshal(b, &scrapeCfgCopy); err != nil {
		return nil, fmt.Errorf("invalid scrape configuration: %w", err)
	}
	return scrapeCfg, nil
}

func relabelingsForMetadata(keys map[string]struct{}) (res []*relabel.Config) {
	if _, ok := keys["namespace"]; ok {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
			TargetLabel:  "namespace",
		})
	}
	if _, ok := keys["pod"]; ok {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_name"},
			TargetLabel:  "pod",
		})
	}
	if _, ok := keys["container"]; ok {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_container_name"},
			TargetLabel:  "container",
		})
	}
	if _, ok := keys["node"]; ok {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_node_name"},
			TargetLabel:  "node",
		})
	}
	return res
}

func (cm *ClusterPodMonitoring) endpointScrapeConfig(index int, projectID, location, cluster string) (*promconfig.ScrapeConfig, error) {
	// Filter targets that belong to selected pods.
	relabelCfgs, err := relabelingsForSelector(cm.Spec.Selector)
	if err != nil {
		return nil, err
	}

	metadataLabels := map[string]struct{}{}
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility. In that case we must always add the namespace label.
	if cm.Spec.TargetLabels.Metadata == nil {
		metadataLabels = map[string]struct{}{
			"namespace": struct{}{},
		}
	} else {
		for _, l := range *cm.Spec.TargetLabels.Metadata {
			if allowed := []string{"namespace", "pod", "container", "node"}; !containsString(allowed, l) {
				return nil, fmt.Errorf("metadata label %q not allowed, must be one of %v", l, allowed)
			}
			metadataLabels[l] = struct{}{}
		}
	}
	relabelCfgs = append(relabelCfgs, relabelingsForMetadata(metadataLabels)...)

	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:      relabel.Replace,
		Replacement: cm.Name,
		TargetLabel: "job",
	})

	return endpointScrapeConfig(
		cm.GetKey(),
		projectID, location, cluster,
		cm.Spec.Endpoints[index],
		relabelCfgs,
		cm.Spec.TargetLabels.FromPod,
		cm.Spec.Limits,
	)
}

// convertRelabelingRule converts the rule to a relabel configuration. An error is returned
// if the rule would modify one of the protected labels.
func convertRelabelingRule(r RelabelingRule) (*relabel.Config, error) {
	rcfg := &relabel.Config{
		// Upstream applies ToLower when digesting the config, so we allow the same.
		Action:      relabel.Action(strings.ToLower(r.Action)),
		TargetLabel: r.TargetLabel,
		Separator:   r.Separator,
		Replacement: r.Replacement,
		Modulus:     r.Modulus,
	}
	for _, n := range r.SourceLabels {
		rcfg.SourceLabels = append(rcfg.SourceLabels, prommodel.LabelName(n))
	}
	// Instantiate the default regex Prometheus uses so that the checks below can be run
	// if no explicit value is provided.
	re := relabel.MustNewRegexp(`(.*)`)

	// We must only set the regex if its not empty. Like in other cases, the Prometheus code does
	// not setup the structs correctly and this would default to the string "null" when marshalled,
	// which is then interpreted as a regex again when read by Prometheus.
	if r.Regex != "" {
		var err error
		re, err = relabel.NewRegexp(r.Regex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", r.Regex, err)
		}
		rcfg.Regex = re
	}

	// Validate that the protected target labels are not mutated by the provided relabeling rules.
	switch rcfg.Action {
	// Default action is "replace" per https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config.
	case relabel.Replace, relabel.HashMod, "":
		// These actions write into the target label and it must not be a protected one.
		if isProtectedLabel(r.TargetLabel) {
			return nil, fmt.Errorf("cannot relabel with action %q onto protected label %q", r.Action, r.TargetLabel)
		}
	case relabel.LabelDrop:
		if matchesAnyProtectedLabel(re) {
			return nil, fmt.Errorf("regex %s would drop at least one of the protected labels %s", r.Regex, strings.Join(protectedLabels, ", "))
		}
	case relabel.LabelKeep:
		// Keep drops all labels that don't match the regex. So all protected labels must
		// match keep.
		if !matchesAllProtectedLabels(re) {
			return nil, fmt.Errorf("regex %s would drop at least one of the protected labels %s", r.Regex, strings.Join(protectedLabels, ", "))
		}
	case relabel.LabelMap:
		// It is difficult to prove for certain that labelmap does not override a protected label.
		// Thus we just prohibit its use for now.
		// The most feasible way to support this would probably be store all protected labels
		// in __tmp_protected_<name> via a replace rule, then apply labelmap, then replace the
		// __tmp label back onto the protected label.
		return nil, fmt.Errorf("relabeling with action %q not allowed", r.Action)
	case relabel.Keep, relabel.Drop:
		// These actions don't modify a series and are OK.
	default:
		return nil, fmt.Errorf("unknown relabeling action %q", r.Action)
	}
	return rcfg, nil
}

var protectedLabels = []string{
	export.KeyProjectID,
	export.KeyLocation,
	export.KeyCluster,
	export.KeyNamespace,
	export.KeyJob,
	export.KeyInstance,
	"__address__",
}

func isProtectedLabel(s string) bool {
	return containsString(protectedLabels, s)
}

func matchesAnyProtectedLabel(re relabel.Regexp) bool {
	for _, pl := range protectedLabels {
		if re.MatchString(pl) {
			return true
		}
	}
	return false
}

func matchesAllProtectedLabels(re relabel.Regexp) bool {
	for _, pl := range protectedLabels {
		if !re.MatchString(pl) {
			return false
		}
	}
	return true
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if s == x {
			return true
		}
	}
	return false
}

// labelMappingRelabelConfigs generates relabel configs using a provided mapping and resource prefix.
func labelMappingRelabelConfigs(mappings []LabelMapping, prefix string) ([]*relabel.Config, error) {
	var relabelCfgs []*relabel.Config
	for _, m := range mappings {
		// `To` can be unset, default to `From`.
		if m.To == "" {
			m.To = m.From
		}
		rcfg, err := convertRelabelingRule(RelabelingRule{
			Action:       "replace",
			SourceLabels: []string{prefix + string(sanitizeLabelName(m.From))},
			TargetLabel:  m.To,
		})
		if err != nil {
			return nil, err
		}
		relabelCfgs = append(relabelCfgs, rcfg)
	}
	return relabelCfgs, nil
}

// PodMonitoringSpec contains specification parameters for PodMonitoring.
type PodMonitoringSpec struct {
	// Label selector that specifies which pods are selected for this monitoring
	// configuration.
	Selector metav1.LabelSelector `json:"selector"`
	// The endpoints to scrape on the selected pods.
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints.
	// The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`
	// if the scraped pod is controlled by a DaemonSet.
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
	// Labels to add to the Prometheus target for discovered endpoints.
	// The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`
	// if the scraped pod is controlled by a DaemonSet.
	TargetLabels TargetLabels `json:"targetLabels,omitempty"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
}

// ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.
type ScrapeEndpoint struct {
	// Name or number of the port to scrape.
	// The container metadata label is only populated if the port is referenced by name
	// because port numbers are not unique across containers.
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

// HTTPClientConfig stores HTTP-client configurations.
type HTTPClientConfig struct {
	// Configures the scrape request's TLS settings.
	TLS *TLS `json:"tls,omitempty"`
}

// TargetLabels configures labels for the discovered Prometheus targets.
type TargetLabels struct {
	// Pod metadata labels that are set on all scraped targets.
	// Permitted keys are `pod`, `container`, and `node` for PodMonitoring and
	// `pod`, `container`, `node`, and `namespace` for ClusterPodMonitoring. The `container`
	// label is only populated if the scrape port is referenced by name.
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
	// The generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// Represents the latest available observations of a podmonitor's current state.
	Conditions []MonitoringCondition `json:"conditions,omitempty"`
	// Represents the latest available observations of target state for each ScrapeEndpoint.
	EndpointStatuses []ScrapeEndpointStatus `json:"endpointStatuses,omitempty"`
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

// Rules defines Prometheus alerting and recording rules that are scoped
// to the namespace of the resource. Only metric data from this namespace is processed
// and all rule results have their project_id, cluster, and namespace label preserved
// for query processing.
// If the location label is not preserved by the rule, it defaults to the cluster's location.
//
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
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
// +kubebuilder:storageversion
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
// +kubebuilder:storageversion
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

var invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeLabelName reproduces the label name cleanup Prometheus's service discovery applies.
func sanitizeLabelName(name string) prommodel.LabelName {
	return prommodel.LabelName(invalidLabelCharRE.ReplaceAllString(name, "_"))
}
