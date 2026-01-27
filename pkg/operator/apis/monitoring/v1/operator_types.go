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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// OperatorConfig defines configuration of the gmp-operator.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type OperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Rules specifies how the operator configures and deploys rule-evaluator.
	Rules RuleEvaluatorSpec `json:"rules,omitempty"`
	// Collection specifies how the operator configures collection, including
	// scraping and an integrated export to Google Cloud Monitoring.
	Collection CollectionSpec `json:"collection,omitempty"`
	// Exports is an EXPERIMENTAL feature that specifies additional, optional endpoints to export to,
	// on top of Google Cloud Monitoring collection.
	// Note: To disable integrated export to Google Cloud Monitoring specify a non-matching filter in the "collection.filter" field.
	Exports []ExportSpec `json:"exports,omitempty"`
	// ManagedAlertmanager holds information for configuring the managed instance of Alertmanager.
	// +kubebuilder:default={configSecret: {name: alertmanager, key: alertmanager.yaml}}
	ManagedAlertmanager *ManagedAlertmanagerSpec `json:"managedAlertmanager,omitempty"`
	// Features holds configuration for optional managed-collection features.
	Features OperatorFeatures `json:"features,omitempty"`
	// Scaling contains configuration options for scaling GMP.
	Scaling ScalingSpec `json:"scaling,omitempty"`
	// Status holds the status of the OperatorConfig.
	Status OperatorConfigStatus `json:"status,omitempty"`
}

// GetMonitoringStatus returns the status of the OperatorConfig.
func (oc *OperatorConfig) GetMonitoringStatus() *MonitoringStatus {
	return &oc.Status.MonitoringStatus
}

func (oc *OperatorConfig) Validate() error {
	if _, err := oc.Collection.ScrapeConfigs(); err != nil {
		return fmt.Errorf("failed to create kubelet scrape config: %w", err)
	}

	if err := validateSecretKeySelector(oc.Collection.Credentials); err != nil {
		return fmt.Errorf("invalid collection credentials: %w", err)
	}
	if oc.ManagedAlertmanager != nil {
		if err := validateSecretKeySelector(oc.ManagedAlertmanager.ConfigSecret); err != nil {
			return fmt.Errorf("invalid managed alert manager config secret: %w", err)
		}
	}
	if err := validateRules(&oc.Rules); err != nil {
		return fmt.Errorf("invalid rules config: %w", err)
	}
	return nil
}

func validateRules(rules *RuleEvaluatorSpec) error {
	if rules.GeneratorURL != "" {
		if _, err := url.Parse(rules.GeneratorURL); err != nil {
			return fmt.Errorf("failed to parse generator URL: %w", err)
		}
	}

	if err := validateSecretKeySelector(rules.Credentials); err != nil {
		return fmt.Errorf("invalid credentials: %w", err)
	}
	for i, alertManagerEndpoint := range rules.Alerting.Alertmanagers {
		if err := validateAlertManagerEndpoint(&alertManagerEndpoint); err != nil {
			return fmt.Errorf("invalid alert manager endpoint `%s` (index %d): %w", alertManagerEndpoint.Name, i, err)
		}
	}
	return nil
}

func validateAlertManagerEndpoint(alertManagerEndpoint *AlertmanagerEndpoints) error {
	if alertManagerEndpoint.Authorization != nil {
		if err := validateSecretKeySelector(alertManagerEndpoint.Authorization.Credentials); err != nil {
			return fmt.Errorf("invalid authorization credentials: %w", err)
		}
	}
	if alertManagerEndpoint.TLS != nil {
		if err := validateSecretKeySelector(alertManagerEndpoint.TLS.KeySecret); err != nil {
			return fmt.Errorf("invalid TLS key: %w", err)
		}
		if err := validateSecretOrConfigMap(alertManagerEndpoint.TLS.CA); err != nil {
			return fmt.Errorf("invalid TLS CA: %w", err)
		}
		if err := validateSecretOrConfigMap(alertManagerEndpoint.TLS.Cert); err != nil {
			return fmt.Errorf("invalid TLS Cert: %w", err)
		}
	}
	return nil
}

func validateSecretOrConfigMap(secretOrConfigMap *SecretOrConfigMap) error {
	if secretOrConfigMap == nil {
		return nil
	}
	if secretOrConfigMap.Secret != nil {
		if err := validateSecretKeySelector(secretOrConfigMap.Secret); err != nil {
			return err
		}
		if secretOrConfigMap.ConfigMap != nil {
			return errors.New("SecretOrConfigMap fields are mutually exclusive")
		}
	}
	return nil
}

func validateSecretKeySelector(secretKeySelector *corev1.SecretKeySelector) error {
	if secretKeySelector == nil {
		return nil
	}
	if secretKeySelector.Name == "" {
		return errors.New("missing secret key selector name")
	}
	return nil
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
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
}

// CollectionSpec specifies how the operator configures collection of metric data.
type CollectionSpec struct {
	// ExternalLabels specifies external labels that are attached to all scraped
	// data before being written to Google Cloud Monitoring or any other additional exports
	// specified in the OperatorConfig. The precedence behavior matches that of Prometheus.
	ExternalLabels map[string]string `json:"externalLabels,omitempty"`
	// Filter limits which metric data is sent to Cloud Monitoring (it doesn't apply to additional exports).
	Filter ExportFilters `json:"filter,omitempty"`
	// A reference to GCP service account credentials with which Prometheus collectors
	// are run. It needs to have metric write permissions for all project IDs to which
	// data is written.
	// Within GKE, this can typically be left empty if the compute default
	// service account has the required permissions.
	Credentials *corev1.SecretKeySelector `json:"credentials,omitempty"`
	// Configuration to scrape the metric endpoints of the Kubelets.
	KubeletScraping *KubeletScraping `json:"kubeletScraping,omitempty"`
	// Compression enables compression of metrics collection data
	Compression CompressionType `json:"compression,omitempty"`
}

type ExportSpec struct {
	// The URL of the endpoint that supports Prometheus Remote Write to export samples to.
	URL string `json:"url"`
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
	// Compression enables compression of the config data propagated by the operator to collectors
	// and the rule-evaluator. It is recommended to use the gzip option when using a large number of
	// ClusterPodMonitoring, PodMonitoring, GlobalRules, ClusterRules, and/or Rules.
	Compression CompressionType `json:"compression,omitempty"`
}

// TargetStatusSpec holds configuration for target status reporting.
type TargetStatusSpec struct {
	// Enable target status reporting.
	Enabled bool `json:"enabled,omitempty"`
}

// +kubebuilder:validation:Enum=none;gzip
type CompressionType string

const (
	// CompressionNone indicates that no compression should be used.
	CompressionNone CompressionType = "none"
	// CompressionGzip indicates that gzip compression should be used.
	CompressionGzip CompressionType = "gzip"
)

// KubeletScraping allows enabling scraping of the Kubelets' metric endpoints.
type KubeletScraping struct {
	// The interval at which the metric endpoints are scraped.
	Interval string `json:"interval"`
	// TLSInsecureSkipVerify disables verifying the target cert.
	// This can be useful for clusters provisioned with kubeadm.
	TLSInsecureSkipVerify bool `json:"tlsInsecureSkipVerify,omitempty"`
}

// OperatorConfigStatus holds status information of the OperatorConfig.
type OperatorConfigStatus struct {
	MonitoringStatus `json:",inline"`
}

// ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.
type ExportFilters struct {
	// A list of Prometheus time series matchers. Every time series must match at least one
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
	ConfigSecret *corev1.SecretKeySelector `json:"configSecret,omitempty"`
	// ExternalURL is the URL under which Alertmanager is externally reachable (for example, if
	// Alertmanager is served via a reverse proxy). Used for generating relative and absolute
	// links back to Alertmanager itself. If the URL has a path portion, it will be used to
	// prefix all HTTP endpoints served by Alertmanager, otherwise relevant URL components will
	// be derived automatically.
	//
	// If no URL is provided, Alertmanager will point to the Google Cloud Metric Explorer page.
	ExternalURL string `json:"externalURL,omitempty"`
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

// TLSConfig specifies TLS configuration parameters from Kubernetes resources.
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
	Secret *corev1.SecretKeySelector `json:"secret,omitempty"`
	// ConfigMap containing data to use for the targets.
	ConfigMap *corev1.ConfigMapKeySelector `json:"configMap,omitempty"`
}

// ScalingSpec defines configuration options for scaling GMP.
type ScalingSpec struct {
	VPA VPASpec `json:"vpa,omitempty"`
}

// VPASpec defines configuration options for vertical pod autoscaling.
type VPASpec struct {
	// Enabled configures whether the operator configures Vertical Pod Autoscaling for GMP workloads.
	// In GKE, installing Vertical Pod Autoscaling requires a cluster restart, and therefore it also results in an operator restart.
	// In other environments, the operator may need to be restarted to enable VPA to run the following check again and watch for the objects.
	Enabled bool `json:"enabled,omitempty"`
}
