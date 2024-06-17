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
	"strings"

	"github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/relabel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ScrapeNodeEndpoint specifies a Prometheus metrics endpoint on a node to scrape.
// It contains all the fields used in the ScrapeEndpoint except for port and HTTPClientConfig.
type ScrapeNodeEndpoint struct {
	// Protocol scheme to use to scrape.
	Scheme string `json:"scheme,omitempty"`
	// HTTP path to scrape metrics from. Defaults to "/metrics".
	Path string `json:"path,omitempty"`
	// HTTP GET params to use when scraping.
	Params map[string][]string `json:"params,omitempty"`
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
	// TLS configures the scrape request's TLS settings.
	// +optional
	TLS *ClusterNodeTLS `json:"tls,omitempty"`
}

type ClusterNodeTLS struct {
	// InsecureSkipVerify disables target certificate validation.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// ClusterNodeMonitoringSpec contains specification parameters for ClusterNodeMonitoring.
type ClusterNodeMonitoringSpec struct {
	// Label selector that specifies which nodes are selected for this monitoring
	// configuration. If left empty all nodes are selected.
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// The endpoints to scrape on the selected nodes.
	Endpoints []ScrapeNodeEndpoint `json:"endpoints"`
	// Limits to apply at scrape time.
	Limits *ScrapeLimits `json:"limits,omitempty"`
}

// ClusterNodeMonitoringList is a list of ClusterNodeMonitorings.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterNodeMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterNodeMonitoring `json:"items"`
}

// ClusterNodeMonitoring defines monitoring for a set of nodes.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type ClusterNodeMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of desired node selection for target discovery by
	// Prometheus.
	Spec ClusterNodeMonitoringSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status MonitoringStatus `json:"status,omitempty"`
}

func (c *ClusterNodeMonitoring) GetKey() string {
	return fmt.Sprintf("ClusterNodeMonitoring/%s", c.Name)
}

func (c *ClusterNodeMonitoring) GetEndpoints() []ScrapeNodeEndpoint {
	return c.Spec.Endpoints
}

func (c *ClusterNodeMonitoring) GetMonitoringStatus() *MonitoringStatus {
	return &c.Status
}

func (c *ClusterNodeMonitoring) ValidateCreate() (admission.Warnings, error) {
	if len(c.Spec.Endpoints) == 0 {
		return nil, errors.New("at least one endpoint is required")
	}
	// TODO(freinartz): extract validator into dedicated object (like defaulter). For now using
	// example values has no adverse effects.
	_, err := c.ScrapeConfigs("test_project", "test_location", "test_cluster")
	return nil, err
}

func (c *ClusterNodeMonitoring) ValidateUpdate(runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return c.ValidateCreate()
}

func (*ClusterNodeMonitoring) ValidateDelete() (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (c *ClusterNodeMonitoring) ScrapeConfigs(projectID, location, cluster string) (res []*promconfig.ScrapeConfig, err error) {
	for i, ep := range c.Spec.Endpoints {
		sc, err := c.endpointScrapeConfig(&ep, projectID, location, cluster)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, sc)
	}
	return res, validateDistinctJobNames(res)
}

func (c *ClusterNodeMonitoring) endpointScrapeConfig(ep *ScrapeNodeEndpoint, projectID, location, cluster string) (*promconfig.ScrapeConfig, error) {
	// Filter targets that belong to selected nodes.
	relabelCfgs, err := relabelingsForSelector(c.Spec.Selector, c)
	if err != nil {
		return nil, err
	}

	metricsPath := "/metrics"
	if ep.Path != "" {
		metricsPath = ep.Path
	}

	relabelCfgs = append(relabelCfgs,
		&relabel.Config{
			Action:      relabel.Replace,
			Replacement: c.Name,
			TargetLabel: "job",
		},
		&relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_node_name"},
			TargetLabel:  "node",
		},
		&relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_node_name"},
			Replacement:  fmt.Sprintf(`$1:%s`, strings.TrimPrefix(metricsPath, "/")),
			TargetLabel:  "instance",
		},
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
	)

	discoveryCfgs := discovery.Configs{
		&discoverykube.SDConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			Role:             discoverykube.RoleNode,
			// Drop all potential targets not the same node as the collector. The $(NODE_NAME) variable
			// is interpolated by the config reloader sidecar before the config reaches the Prometheus collector.
			// Doing it through selectors rather than relabeling should substantially reduce the client and
			// server side load.
			Selectors: []discoverykube.SelectorConfig{
				{
					Role:  discoverykube.RoleNode,
					Field: fmt.Sprintf("metadata.name=$(%s)", EnvVarNodeName),
				},
			},
		},
	}

	httpCfg := config.HTTPClientConfig{
		Authorization: &config.Authorization{
			CredentialsFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		},
		TLSConfig: config.TLSConfig{
			CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		},
	}
	if tls := ep.TLS; tls != nil {
		httpCfg.TLSConfig.InsecureSkipVerify = tls.InsecureSkipVerify
	}

	return buildPrometheusScrapeConfig(fmt.Sprintf("%s%s", c.GetKey(), metricsPath), discoveryCfgs, httpCfg, relabelCfgs, c.Spec.Limits,
		ScrapeEndpoint{Interval: ep.Interval,
			Timeout:          ep.Timeout,
			Path:             metricsPath,
			MetricRelabeling: ep.MetricRelabeling,
			Scheme:           ep.Scheme,
			Params:           ep.Params})
}
