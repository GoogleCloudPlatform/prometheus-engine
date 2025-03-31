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
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/relabel"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Environment variable for the current node that needs to be interpolated in generated
// scrape configurations for a PodMonitoring resource.
const EnvVarNodeName = "NODE_NAME"

// relabelingsForSelector generates a sequence of relabeling rules that implement
// the label selector for the meta labels produced by the Kubernetes service discovery.
func relabelingsForSelector(selector metav1.LabelSelector, crd interface{}) ([]*relabel.Config, error) {
	// Pick the correct labels based on the CRD type.
	var objectLabelPresent, objectLabel prommodel.LabelName
	switch crd.(type) {
	case *PodMonitoring, *ClusterPodMonitoring:
		objectLabel = "__meta_kubernetes_pod_label_"
		objectLabelPresent = "__meta_kubernetes_pod_labelpresent_"
	case *ClusterNodeMonitoring:
		objectLabel = "__meta_kubernetes_node_label_"
		objectLabelPresent = "__meta_kubernetes_node_labelpresent_"
	default:
		return nil, fmt.Errorf("invalid CRD type %T", crd)
	}
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
			SourceLabels: prommodel.LabelNames{objectLabel + sanitizeLabelName(k)},
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
				SourceLabels: prommodel.LabelNames{objectLabel + sanitizeLabelName(exp.Key)},
				Regex:        re,
			})
		case metav1.LabelSelectorOpNotIn:
			re, err := relabel.NewRegexp(strings.Join(exp.Values, "|"))
			if err != nil {
				return nil, err
			}
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{objectLabel + sanitizeLabelName(exp.Key)},
				Regex:        re,
			})
		case metav1.LabelSelectorOpExists:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{objectLabelPresent + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		case metav1.LabelSelectorOpDoesNotExist:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{objectLabelPresent + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		}
	}

	return relabelCfgs, nil
}

// buildPrometheusScrapeConfig builds a Prometheus scrape configuration for a given endpoint.
func buildPrometheusScrapeConfig(jobName string, discoverCfgs discovery.Configs, httpCfg config.HTTPClientConfig, relabelCfgs []*relabel.Config, limits *ScrapeLimits, ep ScrapeEndpoint) (*ScrapeConfig, error) {
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

	scrapeCfg := &ScrapeConfig{
		// Generate a job name to make it easy to track what generated the scrape configuration.
		// The actual job label attached to its metrics is overwritten via relabeling.
		ScrapeConfig: promconfig.ScrapeConfig{
			JobName:                 jobName,
			ServiceDiscoveryConfigs: discoverCfgs,
			MetricsPath:             metricsPath,
			Scheme:                  ep.Scheme,
			Params:                  ep.Params,
			HTTPClientConfig:        httpCfg,
			ScrapeInterval:          interval,
			ScrapeTimeout:           timeout,
			RelabelConfigs:          relabelCfgs,
			MetricRelabelConfigs:    metricRelabelCfgs,
		},
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

var invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeLabelName reproduces the label name cleanup Prometheus's service discovery applies.
func sanitizeLabelName(name string) prommodel.LabelName {
	return prommodel.LabelName(invalidLabelCharRE.ReplaceAllString(name, "_"))
}

// validateDistinctJobNames returns error if any job name duplicates.
// This would also result in an error on Prometheus server config parsing.
func validateDistinctJobNames(jobs []*ScrapeConfig) error {
	jobToIndex := map[string]int{}
	for i, job := range jobs {
		if dup, ok := jobToIndex[job.JobName]; ok {
			// TODO(bwplotka): Consider adding JobSuffix field or so for https://github.com/GoogleCloudPlatform/prometheus-engine/issues/479
			return fmt.Errorf("duplicated job name %v for endpoints with index %d and %d;"+
				"consider creating a separate custom resource (PodMonitoring, etc.) for endpoints that share the same resource name, namespace and port name", job.JobName, dup, i)
		}
		jobToIndex[job.JobName] = i
	}
	return nil
}
