// Copyright 2024 Google LLC
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
	"maps"
	"slices"
	"strings"

	"github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/google/export"
	"github.com/prometheus/prometheus/model/relabel"
)

const (
	labelCluster   = "cluster"
	labelLocation  = "location"
	labelProjectID = "project_id"

	labelContainer              = "container"
	labelNamespace              = "namespace"
	labelNode                   = "node"
	labelPod                    = "pod"
	labelTopLevelControllerName = "top_level_controller_name"
	labelTopLevelControllerType = "top_level_controller_type"
)

var (
	allowedClusterPodMonitoringLabel = map[string]bool{
		labelContainer:              true,
		labelNamespace:              true,
		labelNode:                   true,
		labelPod:                    true,
		labelTopLevelControllerName: true,
		labelTopLevelControllerType: true,
	}
	allowedClusterPodMonitoringLabels = slices.Sorted(maps.Keys(allowedClusterPodMonitoringLabel))

	allowedPodMonitoringLabel = map[string]bool{
		labelContainer:              true,
		labelNode:                   true,
		labelPod:                    true,
		labelTopLevelControllerName: true,
		labelTopLevelControllerType: true,
	}
	allowedPodMonitoringLabels = slices.Sorted(maps.Keys(allowedClusterPodMonitoringLabel))

	topLevelControllerNameRules = []*relabel.Config{
		// First, capture the controller name from the pod manifest.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_name"},
			TargetLabel:  labelTopLevelControllerName,
		},
		// If the controller kind is a ReplicaSet and it has a pod template hash, it belongs to a deployment.
		// The name of the deployment is the name of the ReplicaSet with the hash truncated.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind", "__meta_kubernetes_pod_labelpresent_pod_template_hash", "__meta_kubernetes_pod_controller_name"},
			Regex:        relabel.MustNewRegexp("ReplicaSet;true;(.+)-[a-z0-9]+"),
			TargetLabel:  labelTopLevelControllerName,
		},
		// If the controller kind is Job and it has a 8-digit numeric suffix (i.e. timestamp), assume the Job was created by a CronJob.
		// The name of the deployment is the name of the Job with the timestamp truncated.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind", "__meta_kubernetes_pod_controller_name"},
			Regex:        relabel.MustNewRegexp("Job;(.+)-\\d{8}$"),
			TargetLabel:  labelTopLevelControllerName,
		},
	}

	topLevelControllerTypeRules = []*relabel.Config{
		// First, capture the controller name from the pod manifest.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind"},
			TargetLabel:  labelTopLevelControllerType,
		},
		// If the controller kind is a ReplicaSet and it has a pod template hash, it belongs to a deployment.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind", "__meta_kubernetes_pod_labelpresent_pod_template_hash", "__meta_kubernetes_pod_controller_name"},
			Regex:        relabel.MustNewRegexp("ReplicaSet;true;(.+)-[a-z0-9]+"),
			TargetLabel:  labelTopLevelControllerType,
			Replacement:  "Deployment",
		},
		// If the controller kind is Job and it has a 8-digit numeric suffix (i.e. timestamp), assume the Job was created by a CronJob.
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_controller_kind", "__meta_kubernetes_pod_controller_name"},
			Regex:        relabel.MustNewRegexp("Job;(.+)-\\d{8}$"),
			TargetLabel:  labelTopLevelControllerType,
			Replacement:  "CronJob",
		},
	}
)

// ScrapeConfigs generates Prometheus scrape configs for the PodMonitoring.
func (p *PodMonitoring) ScrapeConfigs(projectID, location, cluster string, pool PrometheusSecretConfigs, globalMetricRelabelCfg []*relabel.Config) (res []*promconfig.ScrapeConfig, err error) {
	relabelCfgs := []*relabel.Config{
		// Force target labels, so they cannot be overwritten by metric labels.
		{
			Action:      relabel.Replace,
			TargetLabel: labelProjectID,
			Replacement: projectID,
		},
		{
			Action:      relabel.Replace,
			TargetLabel: labelLocation,
			Replacement: location,
		},
		{
			Action:      relabel.Replace,
			TargetLabel: labelCluster,
			Replacement: cluster,
		},
	}
	return p.scrapeConfigs(relabelCfgs, globalMetricRelabelCfg, pool)
}

// ScrapeConfigs generates Prometheus scrape configs for the PodMonitoring.
//
// The relabelCfgs, globalMetricRelabelCfg slices are read only.
func (p *PodMonitoring) scrapeConfigs(relabelCfgs, globalMetricRelabelCfg []*relabel.Config, pool PrometheusSecretConfigs) (res []*promconfig.ScrapeConfig, err error) {
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		// Filter targets by namespace of the PodMonitoring configuration.
		Action:       relabel.Keep,
		SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
		Regex:        relabel.MustNewRegexp(p.Namespace),
	})
	for i := range p.Spec.Endpoints {
		// Each scrape endpoint has its own relabel config so make sure we copy the array.
		c, err := p.endpointScrapeConfig(i, append([]*relabel.Config(nil), relabelCfgs...), globalMetricRelabelCfg, pool)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, validateDistinctJobNames(res)
}

func (p *PodMonitoring) endpointScrapeConfig(index int, relabelCfgs, globalMetricRelabelCfg []*relabel.Config, pool PrometheusSecretConfigs) (*promconfig.ScrapeConfig, error) {
	// Filter targets that belong to selected pods.
	selectors, err := relabelingsForSelector(p.Spec.Selector, p)
	if err != nil {
		return nil, err
	}
	relabelCfgs = append(relabelCfgs, selectors...)

	metadataLabels := make(map[string]bool)
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility and won't add any labels in that case.
	if p.Spec.TargetLabels.Metadata != nil {
		for _, l := range *p.Spec.TargetLabels.Metadata {
			if !allowedPodMonitoringLabel[l] {
				return nil, fmt.Errorf("metadata label %q not allowed, must be one of %v", l, allowedPodMonitoringLabels)
			}
			metadataLabels[l] = true
		}
	}
	relabelCfgs = append(relabelCfgs, relabelingsForMetadata(metadataLabels)...)

	// The namespace label is always set for PodMonitorings.
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:       relabel.Replace,
		SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
		TargetLabel:  labelNamespace,
	})
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:      relabel.Replace,
		Replacement: p.Name,
		TargetLabel: "job",
	})

	// Drop any non-running pods if left unspecified or explicitly enabled.
	if p.Spec.FilterRunning == nil || *p.Spec.FilterRunning {
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Drop,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_phase"},
			Regex:        relabel.MustNewRegexp("(Failed|Succeeded)"),
		})
	}

	return endpointScrapeConfig(
		p,
		p.Spec.Endpoints[index],
		relabelCfgs, globalMetricRelabelCfg,
		p.Spec.TargetLabels.FromPod,
		p.Spec.Limits,
		pool,
	)
}

// ScrapeConfigs generates Prometheus scrape configs for the PodMonitoring.
func (c *ClusterPodMonitoring) ScrapeConfigs(projectID, location, cluster string, pool PrometheusSecretConfigs, globalMetricRelabelCfg []*relabel.Config) (res []*promconfig.ScrapeConfig, err error) {
	relabelCfgs := []*relabel.Config{
		// Force target labels, so they cannot be overwritten by metric labels.
		{
			Action:      relabel.Replace,
			TargetLabel: labelProjectID,
			Replacement: projectID,
		},
		{
			Action:      relabel.Replace,
			TargetLabel: labelLocation,
			Replacement: location,
		},
		{
			Action:      relabel.Replace,
			TargetLabel: labelCluster,
			Replacement: cluster,
		},
	}
	return c.scrapeConfigs(relabelCfgs, globalMetricRelabelCfg, pool)
}

func (c *ClusterPodMonitoring) scrapeConfigs(relabelCfgs, globalMetricRelabelCfg []*relabel.Config, pool PrometheusSecretConfigs) (res []*promconfig.ScrapeConfig, err error) {
	for i := range c.Spec.Endpoints {
		// Each scrape endpoint has its own relabel config so make sure we copy the array.
		c, err := c.endpointScrapeConfig(i, append([]*relabel.Config(nil), relabelCfgs...), globalMetricRelabelCfg, pool)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, validateDistinctJobNames(res)
}

func (c *ClusterPodMonitoring) endpointScrapeConfig(index int, relabelCfgs, globalMetricRelabelCfg []*relabel.Config, pool PrometheusSecretConfigs) (*promconfig.ScrapeConfig, error) {
	// Filter targets that belong to selected pods.
	selectors, err := relabelingsForSelector(c.Spec.Selector, c)
	if err != nil {
		return nil, err
	}
	relabelCfgs = append(relabelCfgs, selectors...)

	metadataLabels := make(map[string]bool)
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility. In that case we must always add the namespace label.
	if c.Spec.TargetLabels.Metadata == nil {
		metadataLabels = map[string]bool{
			labelNamespace: true,
		}
	} else {
		for _, l := range *c.Spec.TargetLabels.Metadata {
			if !allowedClusterPodMonitoringLabel[l] {
				return nil, fmt.Errorf("metadata label %q not allowed, must be one of %v", l, allowedClusterPodMonitoringLabels)
			}
			metadataLabels[l] = true
		}
	}
	relabelCfgs = append(relabelCfgs, relabelingsForMetadata(metadataLabels)...)

	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:      relabel.Replace,
		Replacement: c.Name,
		TargetLabel: "job",
	})

	// Drop any non-running pods if left unspecified or explicitly enabled.
	if c.Spec.FilterRunning == nil || *c.Spec.FilterRunning {
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Drop,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_phase"},
			Regex:        relabel.MustNewRegexp("(Failed|Succeeded)"),
		})
	}

	return endpointScrapeConfig(
		c,
		c.Spec.Endpoints[index],
		relabelCfgs, globalMetricRelabelCfg,
		c.Spec.TargetLabels.FromPod,
		c.Spec.Limits,
		pool,
	)
}

func endpointScrapeConfig(
	m PodMonitoringCRD,
	ep ScrapeEndpoint,
	relabelCfgs, globalMetricRelabelCfg []*relabel.Config,
	podLabels []LabelMapping,
	limits *ScrapeLimits,
	pool PrometheusSecretConfigs,
) (*promconfig.ScrapeConfig, error) {
	id := m.GetKey()
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
		// requested as an output label via .targetLabels.metadata. This aligns with the Pod specification,
		// which requires port names in a Pod to be unique but not port numbers. Thus, the container is
		// potentially ambiguous for numerical ports in any case.

		// First, drop the container label even if it was added before.
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action: relabel.LabelDrop,
			Regex:  relabel.MustNewRegexp(labelContainer),
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
	pCfgs, err := labelMappingRelabelConfigs(podLabels, "__meta_kubernetes_pod_label_")
	if err != nil {
		return nil, fmt.Errorf("invalid pod label mapping: %w", err)
	}
	relabelCfgs = append(relabelCfgs, pCfgs...)

	httpCfg, err := ep.ToPrometheusConfig(m, pool)
	if err != nil {
		return nil, fmt.Errorf("unable to parse or invalid Prometheus HTTP client config: %w", err)
	}
	if err := httpCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Prometheus HTTP client config: %w", err)
	}

	return buildPrometheusScrapeConfig(fmt.Sprintf("%s/%s", id, &ep.Port), discoveryCfgs, httpCfg, relabelCfgs, globalMetricRelabelCfg, limits, ep)
}

func relabelingsForMetadata(keys map[string]bool) (res []*relabel.Config) {
	if keys[labelNamespace] {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
			TargetLabel:  labelNamespace,
		})
	}
	if keys[labelPod] {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_name"},
			TargetLabel:  labelPod,
		})
	}
	if keys[labelContainer] {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_container_name"},
			TargetLabel:  labelContainer,
		})
	}
	if keys[labelNode] {
		res = append(res, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_node_name"},
			TargetLabel:  labelNode,
		})
	}
	if keys[labelTopLevelControllerName] {
		res = append(res, topLevelControllerNameRules...)
	}
	if keys[labelTopLevelControllerType] {
		res = append(res, topLevelControllerTypeRules...)
	}
	return res
}

// ToPrometheusRelabel converts the rule to a Prometheus relabel configuration.
// An error is returned if the rule would modify one of the protected labels.
//
// GoMixedReceiverTypes rationales: purposefully make a copy to avoid accidental changes.
//
//goland:noinspection GoMixedReceiverTypes
func (r RelabelingRule) ToPrometheusRelabel() (*relabel.Config, error) {
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
		if protectedLabel[r.TargetLabel] {
			return nil, fmt.Errorf("cannot relabel with action %q onto protected label %q", r.Action, r.TargetLabel)
		}
	case relabel.LabelDrop:
		if matchesAnyProtectedLabel(re) {
			return nil, fmt.Errorf("regex %s would drop at least one of the protected labels %v", r.Regex, protectedLabels)
		}
	case relabel.LabelKeep:
		// Keep drops all labels that don't match the regex. So all protected labels must
		// match keep.
		if !matchesAllProtectedLabels(re) {
			return nil, fmt.Errorf("regex %s would drop at least one of the protected labels %s", r.Regex, protectedLabels)
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

var (
	protectedLabel = map[string]bool{
		export.KeyProjectID: true,
		export.KeyLocation:  true,
		export.KeyCluster:   true,
		export.KeyNamespace: true,
		export.KeyJob:       true,
		export.KeyInstance:  true,
		"__address__":       true,
	}
	protectedLabels = slices.Sorted(maps.Keys(protectedLabel))
)

func matchesAnyProtectedLabel(re relabel.Regexp) bool {
	for pl := range protectedLabel {
		if re.MatchString(pl) {
			return true
		}
	}
	return false
}

func matchesAllProtectedLabels(re relabel.Regexp) bool {
	for pl := range protectedLabel {
		if !re.MatchString(pl) {
			return false
		}
	}
	return true
}

// labelMappingRelabelConfigs generates relabel configs using a provided mapping and resource prefix.
func labelMappingRelabelConfigs(mappings []LabelMapping, prefix string) ([]*relabel.Config, error) {
	var relabelCfgs []*relabel.Config
	for _, m := range mappings {
		// `To` can be unset, default to `From`.
		if m.To == "" {
			m.To = m.From
		}
		rcfg, err := RelabelingRule{
			Action:       "replace",
			SourceLabels: []string{prefix + string(sanitizeLabelName(m.From))},
			TargetLabel:  m.To,
		}.ToPrometheusRelabel()
		if err != nil {
			return nil, err
		}
		relabelCfgs = append(relabelCfgs, rcfg)
	}
	return relabelCfgs, nil
}
