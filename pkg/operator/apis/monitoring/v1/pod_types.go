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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
)

var (
	errInvalidCond = fmt.Errorf("condition needs both 'Type' and 'Status' fields set")
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
	Items           []ClusterPodMonitoring `json:"items"`
}

func (c *ClusterPodMonitoring) ValidateCreate() (admission.Warnings, error) {
	if len(c.Spec.Endpoints) == 0 {
		return nil, errors.New("at least one endpoint is required")
	}
	// TODO(freinartz): extract validator into dedicated object (like defaulter). For now using
	// example values has no adverse effects.
	_, err := c.ScrapeConfigs("test_project", "test_location", "test_cluster", nil)
	return nil, err
}

func (c *ClusterPodMonitoring) ValidateUpdate(runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return c.ValidateCreate()
}

func (*ClusterPodMonitoring) ValidateDelete() (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (c *ClusterPodMonitoring) ScrapeConfigs(projectID, location, cluster string, pool PrometheusSecretConfigs) (res []*promconfig.ScrapeConfig, err error) {
	for i := range c.Spec.Endpoints {
		c, err := c.endpointScrapeConfig(i, projectID, location, cluster, pool)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, validateDistinctJobNames(res)
}

func (p *PodMonitoring) ValidateCreate() (admission.Warnings, error) {
	if len(p.Spec.Endpoints) == 0 {
		return nil, errors.New("at least one endpoint is required")
	}
	// TODO(freinartz): extract validator into dedicated object (like defaulter). For now using
	// example values has no adverse effects.
	_, err := p.ScrapeConfigs("test_project", "test_location", "test_cluster", nil)
	return nil, err
}

func (p *PodMonitoring) ValidateUpdate(runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return p.ValidateCreate()
}

func (p *PodMonitoring) ValidateDelete() (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

// ScrapeConfigs generates Prometheus scrape configs for the PodMonitoring.
func (p *PodMonitoring) ScrapeConfigs(projectID, location, cluster string, pool PrometheusSecretConfigs) (res []*promconfig.ScrapeConfig, err error) {
	for i := range p.Spec.Endpoints {
		c, err := p.endpointScrapeConfig(i, projectID, location, cluster, pool)
		if err != nil {
			return nil, fmt.Errorf("invalid definition for endpoint with index %d: %w", i, err)
		}
		res = append(res, c)
	}
	return res, validateDistinctJobNames(res)
}

func (p *PodMonitoring) endpointScrapeConfig(index int, projectID, location, cluster string, pool PrometheusSecretConfigs) (*promconfig.ScrapeConfig, error) {
	relabelCfgs := []*relabel.Config{
		// Force target labels, so they cannot be overwritten by metric labels.
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
		// Filter targets by namespace of the PodMonitoring configuration.
		{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
			Regex:        relabel.MustNewRegexp(p.Namespace),
		},
	}

	// Filter targets that belong to selected pods.
	selectors, err := relabelingsForSelector(p.Spec.Selector, p)
	if err != nil {
		return nil, err
	}
	relabelCfgs = append(relabelCfgs, selectors...)

	metadataLabels := map[string]struct{}{}
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility and won't add any labels in that case.
	if p.Spec.TargetLabels.Metadata != nil {
		for _, l := range *p.Spec.TargetLabels.Metadata {
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
		relabelCfgs,
		p.Spec.TargetLabels.FromPod,
		p.Spec.Limits,
		pool,
	)
}

func endpointScrapeConfig(
	m PodMonitoringCRD,
	ep ScrapeEndpoint,
	relabelCfgs []*relabel.Config,
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
	pCfgs, err := labelMappingRelabelConfigs(podLabels, "__meta_kubernetes_pod_label_")
	if err != nil {
		return nil, fmt.Errorf("invalid pod label mapping: %w", err)
	}
	relabelCfgs = append(relabelCfgs, pCfgs...)

	httpCfg, err := ep.HTTPClientConfig.ToPrometheusConfig(m, pool)
	if err != nil {
		return nil, fmt.Errorf("unable to parse or invalid Prometheus HTTP client config: %w", err)
	}
	if err := httpCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Prometheus HTTP client config: %w", err)
	}

	return buildPrometheusScrapeConfig(fmt.Sprintf("%s/%s", id, &ep.Port), discoveryCfgs, httpCfg, relabelCfgs, limits, ep)
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

func (c *ClusterPodMonitoring) endpointScrapeConfig(index int, projectID, location, cluster string, pool PrometheusSecretConfigs) (*promconfig.ScrapeConfig, error) {
	relabelCfgs := []*relabel.Config{
		// Force target labels, so they cannot be overwritten by metric labels.
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

	// Filter targets that belong to selected pods.
	selectors, err := relabelingsForSelector(c.Spec.Selector, c)
	if err != nil {
		return nil, err
	}
	relabelCfgs = append(relabelCfgs, selectors...)

	metadataLabels := map[string]struct{}{}
	// The metadata list must be always set in general but we allow the null case
	// for backwards compatibility. In that case we must always add the namespace label.
	if c.Spec.TargetLabels.Metadata == nil {
		metadataLabels = map[string]struct{}{
			"namespace": {},
		}
	} else {
		for _, l := range *c.Spec.TargetLabels.Metadata {
			if allowed := []string{"namespace", "pod", "container", "node"}; !containsString(allowed, l) {
				return nil, fmt.Errorf("metadata label %q not allowed, must be one of %v", l, allowed)
			}
			metadataLabels[l] = struct{}{}
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
		relabelCfgs,
		c.Spec.TargetLabels.FromPod,
		c.Spec.Limits,
		pool,
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
	Endpoints []ScrapeEndpoint `json:"endpoints"`
	// Labels to add to the Prometheus target for discovered endpoints.
	// The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`
	// if the scraped pod is controlled by a DaemonSet.
	TargetLabels TargetLabels `json:"targetLabels,omitempty"`
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
	// Interval at which to scrape metrics. Must be a valid Prometheus duration.
	// +kubebuilder:validation:Pattern="^((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)$"
	// +kubebuilder:default="1m"
	Interval string `json:"interval,omitempty"`
	// Timeout for metrics scrapes. Must be a valid Prometheus duration.
	// Must not be larger than the scrape interval.
	Timeout string `json:"timeout,omitempty"`
	// Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
	// override protected target labels (project_id, location, cluster, namespace, job,
	// instance, or __address__) are not permitted. The labelmap action is not permitted
	// in general.
	MetricRelabeling []RelabelingRule `json:"metricRelabeling,omitempty"`
	// Prometheus HTTP client configuration.
	HTTPClientConfig `json:",inline"`
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
	// Kubernetes resource label to remap.
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
	MonitoringStatus `json:",inline"`
	// Represents the latest available observations of target state for each ScrapeEndpoint.
	EndpointStatuses []ScrapeEndpointStatus `json:"endpointStatuses,omitempty"`
}
