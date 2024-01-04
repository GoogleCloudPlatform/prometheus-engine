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

func (p *Probe) GetStatus() *MonitoringStatus {
	return &p.Status.MonitoringStatus
}

func (p *Probe) GetKey() string {
	return fmt.Sprintf("Probe/%s", p.Name)
}

func (p *Probe) ValidateCreate() (admission.Warnings, error) {
	if len(p.Spec.Targets) == 0 {
		return nil, errors.New("at least one target is required")
	}
	if _, err := p.scrapeConfigs([]*relabel.Config{}); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *Probe) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return p.ValidateCreate()
}

func (p *Probe) ValidateDelete() (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (cm *Probe) ScrapeConfigs(projectID, location, cluster string) (res []*promconfig.ScrapeConfig, err error) {
	for i := range cm.Spec.Targets {
		relabelCfgs := []*relabel.Config{
			{
				Action:      relabel.Replace,
				Replacement: cm.Name,
				TargetLabel: "job",
			},
			{
				Action:      relabel.Replace,
				TargetLabel: "project_id",
				Replacement: projectID,
			},
			{
				Action:      relabel.Replace,
				TargetLabel: "location",
				Replacement: location,
			},
			{
				Action:      relabel.Replace,
				TargetLabel: "cluster",
				Replacement: cluster,
			},
		}

		c, err := targetScrapeConfig(
			cm.GetKey(),
			cm.Spec.Targets[i],
			relabelCfgs,
			cm.Spec.Limits,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for target at index %d: %w", i, err)
		}
		res = append(res, c...)
	}
	return res, nil
}

func (cm *Probe) scrapeConfigs(relabelCfgs []*relabel.Config) (res []*promconfig.ScrapeConfig, err error) {
	relabelCfgs = append(relabelCfgs,
		&relabel.Config{
			Action:      relabel.Replace,
			Replacement: cm.Name,
			TargetLabel: "job",
		},
	)
	for i := range cm.Spec.Targets {
		c, err := targetScrapeConfig(
			cm.GetKey(),
			cm.Spec.Targets[i],
			relabelCfgs,
			cm.Spec.Limits,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for target at index %d: %w", i, err)
		}
		res = append(res, c...)
	}
	return res, nil
}

const BLACKBOX_EXPORTER_POD_NAME = "blackbox-exporter"

func targetScrapeConfig(id string, ep ProbeTarget, relabelCfgs []*relabel.Config, limits *ScrapeLimits) ([]*promconfig.ScrapeConfig, error) {
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

	var metricRelabelCfgs []*relabel.Config
	for _, r := range ep.MetricRelabeling {
		rcfg, err := convertRelabelingRule(r)
		if err != nil {
			return nil, err
		}
		metricRelabelCfgs = append(metricRelabelCfgs, rcfg)
	}

	httpCfg, err := ep.HTTPClientConfig.ToPrometheusConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to parse HTTP client config: %w", err)
	}

	if err := httpCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Prometheus HTTP client config: %w", err)
	}

	var cfgs []*promconfig.ScrapeConfig
	for i, target := range ep.StaticTargets {
		scrapeCfg := &promconfig.ScrapeConfig{
			// Generate a job name to make it easy to track what generated the scrape configuration.
			// The actual job label attached to its metrics is overwritten via relabeling.
			JobName: fmt.Sprintf("%s/%s/%d", id, &ep.Module, i),
			ServiceDiscoveryConfigs: discovery.Configs{
				&discoverykube.SDConfig{
					HTTPClientConfig: config.DefaultHTTPClientConfig,
					Role:             discoverykube.RolePod,
					Selectors: []discoverykube.SelectorConfig{
						{
							Role:  discoverykube.RolePod,
							Field: fmt.Sprintf("spec.nodeName=$(%s),meta.name=%s,meta.namespace=%s", EnvVarNodeName, BLACKBOX_EXPORTER_POD_NAME, "TODO"),
						},
					},
				},
			},
			MetricsPath: "/probe",
			Scheme:      "http",
			Params: url.Values{
				"module": []string{ep.Module},
				"target": []string{target},
			},
			HTTPClientConfig:     httpCfg,
			ScrapeInterval:       interval,
			ScrapeTimeout:        timeout,
			RelabelConfigs:       relabelCfgs,
			MetricRelabelConfigs: metricRelabelCfgs,
		}
		if limits != nil {
			scrapeCfg.SampleLimit = uint(limits.Samples)
			scrapeCfg.LabelLimit = uint(limits.Labels)
			scrapeCfg.LabelNameLengthLimit = uint(limits.LabelNameLength)
			scrapeCfg.LabelValueLengthLimit = uint(limits.LabelValueLength)
		}
		if err := validateScrapeConfig(scrapeCfg); err != nil {
			return nil, err
		}
		cfgs = append(cfgs, scrapeCfg)
	}
	return cfgs, nil
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
