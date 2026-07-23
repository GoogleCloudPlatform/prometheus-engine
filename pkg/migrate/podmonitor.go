// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/google/export"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	namespacedMetadataDefaults = []string{labelContainer, labelPod, labelTopLevelControllerName, labelTopLevelControllerType}
	clusterMetadataDefaults    = []string{labelContainer, export.KeyNamespace, labelPod, labelTopLevelControllerName, labelTopLevelControllerType}
)

// PodMonitorConverter implements ResourceConverter for PodMonitor resources.
type PodMonitorConverter struct{}

// ImportKey returns the Kind of the resource this converter handles.
func (c *PodMonitorConverter) ImportKey() string {
	return KindPodMonitor
}

// Convert translates a Prometheus Operator PodMonitor into GMP resources.
func (c *PodMonitorConverter) Convert(_ context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, error) {
	if unstruct == nil || unstruct.Object == nil {
		return nil, errors.New("cannot convert nil or uninitialized unstructured resource")
	}

	// 1. Unmarshal unstructured to typed PodMonitor.
	var podMonitor pomonitoringv1.PodMonitor
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &podMonitor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PodMonitor: %w", err)
	}

	logger.Info("Successfully decoded PodMonitor", slog.String("name", podMonitor.Name))

	// TODO(M2): Override local namespace scoping if Prometheus CR specifies ignoreNamespaceSelectors.

	// 2. Determine Scoping based on namespaceSelector.
	nsSel := podMonitor.Spec.NamespaceSelector

	if nsSel.Any {
		// Case A: namespaceSelector.any = true -> Single ClusterPodMonitoring.
		logger.Info("namespaceSelector selects 'any: true'. Translated to 'ClusterPodMonitoring'")
		u, generatedSecrets, err := c.convertToClusterPodMonitoring(&podMonitor, logger, cache)
		if err != nil {
			return nil, err
		}
		outputs := []*unstructured.Unstructured{u}
		outputs = append(outputs, generatedSecrets...)
		return outputs, nil
	}

	if len(nsSel.MatchNames) > 0 {
		// Case B: namespaceSelector.matchNames listed -> Multiple PodMonitoring resources (one per namespace).
		targetNamespaces := ParseAndCleanNamespaces(nsSel.MatchNames)

		// 2.1 Fail if all provided names were empty/whitespace (broken config).
		if len(targetNamespaces) == 0 {
			return nil, errors.New("namespaceSelector.matchNames contains only empty or invalid values")
		}

		if len(targetNamespaces) > 1 {
			logger.Info("namespaceSelector targets multiple namespaces. Generating separate PodMonitoring resources for each namespace",
				slog.Any("namespaces", targetNamespaces),
			)
		}

		// 2.2 Convert to a base namespaced PodMonitoring.
		baseU, generatedSecrets, err := c.convertToPodMonitoring(&podMonitor, logger, cache)
		if err != nil {
			return nil, err
		}

		// 2.3 Clone and apply target namespaces.
		var outputs []*unstructured.Unstructured
		for _, ns := range targetNamespaces {
			uClone := baseU.DeepCopy()
			uClone.SetNamespace(ns)
			outputs = append(outputs, uClone)
		}
		outputs = append(outputs, generatedSecrets...)
		return outputs, nil
	}

	// Case C: namespaceSelector is empty/omitted -> Single PodMonitoring in local namespace.
	u, generatedSecrets, err := c.convertToPodMonitoring(&podMonitor, logger, cache)
	if err != nil {
		return nil, err
	}
	outputs := []*unstructured.Unstructured{u}
	outputs = append(outputs, generatedSecrets...)
	return outputs, nil
}

func (c *PodMonitorConverter) convertEndpoints(
	convCtx *conversionContext,
	endpoints []pomonitoringv1.PodMetricsEndpoint,
	epResults []preScrapeRelabelingResult,
) ([]monitoringv1.ScrapeEndpoint, error) {
	if len(epResults) != len(endpoints) {
		return nil, fmt.Errorf("internal error: pre-scrape relabeling results length (%d) does not match endpoints length (%d)", len(epResults), len(endpoints))
	}

	var gmpEndpoints []monitoringv1.ScrapeEndpoint

	for i, ep := range endpoints {
		gmpEp := monitoringv1.ScrapeEndpoint{}

		// 1. Port mapping.
		if ep.Port != "" {
			gmpEp.Port = intstr.FromString(ep.Port)
		} else if ep.TargetPort != nil { // nolint:staticcheck // Map deprecated TargetPort for backwards compatibility.
			gmpEp.Port = *ep.TargetPort // nolint:staticcheck // Map deprecated TargetPort for backwards compatibility.
		} else {
			return nil, fmt.Errorf("endpoint [%d]: port or targetPort must be set", i)
		}

		// 2. Basic Fields.
		gmpEp.Path = ep.Path
		gmpEp.Scheme = strings.ToLower(ep.Scheme)
		gmpEp.Params = ep.Params

		// 3. Scrape Intervals & Timeouts.
		interval, timeout, err := resolveScrapeIntervalAndTimeout(convCtx.logger, string(ep.Interval), string(ep.ScrapeTimeout))
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}
		gmpEp.Interval = interval
		gmpEp.Timeout = timeout

		// 4. Relabeling Rules (Promoted Pre-Scrape + MetricRelabelings).
		relabelings, err := combineAndConvertRelabelings(convCtx.logger, epResults[i].PromotedRules, ep.MetricRelabelConfigs)
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}
		gmpEp.MetricRelabeling = relabelings

		// Proxy Settings.
		proxyURL, err := convertProxyURL(ep.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}
		gmpEp.ProxyURL = proxyURL

		// noProxy, proxyConnectHeader, and proxyFromEnvironment fields are silently dropped.
		// The pinned Prometheus Operator version lacks these fields, and GMP does not support them anyway.

		// Auth & TLS mappings.
		err = convCtx.applyAuthAndTLS(i, &gmpEp, ep.BasicAuth, ep.OAuth2, ep.TLSConfig, ep.Authorization, ep.BearerTokenSecret)
		if err != nil {
			return nil, err
		}

		// 5. Warnings for Unsupported Fields in Endpoint.
		warnUnsupportedEndpointFields(convCtx.logger, ep.FollowRedirects, ep.EnableHttp2, ep.HonorLabels, ep.HonorTimestamps, ep.TrackTimestampsStaleness, i)

		gmpEndpoints = append(gmpEndpoints, gmpEp)
	}

	return gmpEndpoints, nil
}

func (c *PodMonitorConverter) convertMonitorSpec(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache, isCluster bool) (*commonMonitorSpec, error) {
	convCtx := &conversionContext{
		logger:    logger,
		cache:     cache,
		namespace: pm.Namespace,
	}
	rules, err := extractPreScrapeRelabelings(logger, pm.Spec.PodMetricsEndpoints)
	if err != nil {
		return nil, err
	}
	endpoints, err := c.convertEndpoints(convCtx, pm.Spec.PodMetricsEndpoints, rules.PerEndpoint)
	if err != nil {
		return nil, err
	}

	mergedFromPod := mergeFromPod(logger, convertTargetLabels(logger, pm.Spec.PodTargetLabels, pm.Spec.JobLabel, "Pod"), rules.ResourceCombined.FromPod)
	mergedSelector, err := mergeLabelSelector(pm.Spec.Selector, rules.ResourceCombined.MatchLabels, rules.ResourceCombined.MatchExpressions)
	if err != nil {
		return nil, err
	}
	if len(mergedSelector.MatchLabels) == 0 && len(mergedSelector.MatchExpressions) == 0 {
		logger.Warn("Resulting PodMonitoring selector is empty. It will select and scrape all pods in this namespace. Verify if this is intended.")
	}

	// Spec-level warnings for unsupported fields.
	warnUnsupportedSpecFields(logger, &pm.Spec)
	resolveScrapeClass(pm.Spec.ScrapeClassName, logger)
	validateScrapeProtocols(pm.Spec.ScrapeProtocols, logger)

	var filteredMetadata *[]string
	if isCluster {
		if rules.ResourceCombined.Metadata != nil {
			union := unionMetadata(*rules.ResourceCombined.Metadata, clusterMetadataDefaults)
			filteredMetadata = &union
		}
	} else {
		if rules.ResourceCombined.Metadata != nil {
			union := unionMetadata(*rules.ResourceCombined.Metadata, namespacedMetadataDefaults)
			var md []string
			for _, m := range union {
				if m != export.KeyNamespace {
					md = append(md, m)
				} else {
					logger.Warn("Relabeling rule referencing namespace metadata is unsupported in namespaced PodMonitoring (it is only allowed in ClusterPodMonitoring). The metadata entry has been omitted .")
				}
			}
			if len(md) > 0 {
				filteredMetadata = &md
			}
		}
	}

	// In GMP, Metadata: nil on a PodMonitoring defaults to emitting namespaced defaults (container, pod, etc.).
	// When setting Metadata explicitly for AttachMetadata.Node, we must merge namespacedMetadataDefaults so that default metadata is not dropped.
	if pm.Spec.AttachMetadata != nil && pm.Spec.AttachMetadata.Node != nil && *pm.Spec.AttachMetadata.Node {
		if filteredMetadata == nil {
			union := unionMetadata([]string{labelNode}, namespacedMetadataDefaults)
			filteredMetadata = &union
		} else {
			union := unionMetadata([]string{labelNode}, *filteredMetadata)
			filteredMetadata = &union
		}
	}
	filteredMetadata = resolveAttachMetadata(pm.Spec.AttachMetadata, filteredMetadata, isCluster)

	var filterRunnings []*bool
	for _, ep := range pm.Spec.PodMetricsEndpoints {
		filterRunnings = append(filterRunnings, ep.FilterRunning)
	}
	filterRunning := resolveFilterRunning(filterRunnings, logger, isCluster)

	limits := convertLimits(pm.Spec.SampleLimit, pm.Spec.LabelLimit, pm.Spec.LabelNameLengthLimit, pm.Spec.LabelValueLengthLimit)

	return &commonMonitorSpec{
		endpoints:        endpoints,
		mergedFromPod:    mergedFromPod,
		mergedSelector:   mergedSelector,
		metadata:         filteredMetadata,
		filterRunning:    filterRunning,
		limits:           limits,
		generatedSecrets: convCtx.getGeneratedSecrets(),
	}, nil
}

func (c *PodMonitorConverter) convertToPodMonitoring(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	res, err := c.convertMonitorSpec(pm, logger, cache, false)
	if err != nil {
		return nil, nil, err
	}

	u, err := buildPodMonitoring(pm.ObjectMeta, pm.Namespace, res, logger)
	if err != nil {
		return nil, nil, err
	}

	return u, res.generatedSecrets, nil
}

func (c *PodMonitorConverter) convertToClusterPodMonitoring(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	res, err := c.convertMonitorSpec(pm, logger, cache, true)
	if err != nil {
		return nil, nil, err
	}

	u, err := buildClusterPodMonitoring(pm.ObjectMeta, res, logger)
	if err != nil {
		return nil, nil, err
	}

	return u, res.generatedSecrets, nil
}

func unionMetadata(extracted []string, defaults []string) []string {
	unique := make(map[string]bool)
	for _, m := range defaults {
		unique[m] = true
	}
	for _, m := range extracted {
		unique[m] = true
	}
	var res []string
	for k := range unique {
		res = append(res, k)
	}
	slices.Sort(res)
	return res
}
