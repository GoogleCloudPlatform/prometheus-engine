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
	targetNamespaces, isClusterScoped, err := determineNamespaceScoping(podMonitor.Spec.NamespaceSelector, podMonitor.Namespace)
	if err != nil {
		return nil, err
	}

	if isClusterScoped {
		logger.Info("namespaceSelector selects 'any: true'. Translated to 'ClusterPodMonitoring'")
		u, generatedSecrets, err := c.convertToClusterPodMonitoring(&podMonitor, podMonitor.Namespace, logger, cache)
		if err != nil {
			return nil, err
		}
		outputs := []*unstructured.Unstructured{u}
		outputs = append(outputs, generatedSecrets...)
		return outputs, nil
	}

	if len(targetNamespaces) > 1 {
		logger.Info("namespaceSelector targets multiple namespaces. Generating separate PodMonitoring resources for each namespace",
			slog.Any("namespaces", targetNamespaces),
		)
		logger.Warn("Multi-namespace conversion does not copy existing referenced Kubernetes Secrets. Ensure any referenced Secrets are manually replicated into all target namespaces.",
			slog.Any("target_namespaces", targetNamespaces),
		)
	}

	var outputs []*unstructured.Unstructured
	for _, ns := range targetNamespaces {
		u, generatedSecrets, err := c.convertToPodMonitoring(&podMonitor, ns, logger, cache)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, u)
		outputs = append(outputs, generatedSecrets...)
	}
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
		gmpEp.MetricRelabeling = combineAndConvertRelabelings(convCtx.logger, epResults[i].PromotedRules, ep.MetricRelabelConfigs)

		// Proxy Settings.
		proxyURL, err := convertProxyURL(ep.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}
		gmpEp.ProxyURL = proxyURL

		// noProxy, proxyConnectHeader, and proxyFromEnvironment fields are silently dropped.
		// The pinned Prometheus Operator version lacks these fields, and GMP does not support them anyway.

		// Auth & TLS mappings.
		err = convCtx.applyAuthAndTLS(&gmpEp, ep.BasicAuth, ep.OAuth2, ep.TLSConfig, ep.Authorization, ep.BearerTokenSecret) // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}

		// 5. Warnings for Unsupported Fields in Endpoint.
		warnUnsupportedEndpointFields(convCtx.logger, ep.FollowRedirects, ep.EnableHttp2, ep.HonorLabels, ep.HonorTimestamps, ep.TrackTimestampsStaleness, i)

		gmpEndpoints = append(gmpEndpoints, gmpEp)
	}

	return gmpEndpoints, nil
}

func (c *PodMonitorConverter) convertMonitorSpec(pm *pomonitoringv1.PodMonitor, targetNamespace string, logger *slog.Logger, cache *ResourceCache, isCluster bool) (*commonMonitorSpec, error) {
	convCtx := &conversionContext{
		logger:          logger,
		cache:           cache,
		sourceNamespace: pm.Namespace,
		targetNamespace: targetNamespace,
	}
	var relabelConfigs [][]pomonitoringv1.RelabelConfig
	for _, ep := range pm.Spec.PodMetricsEndpoints {
		relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
	}
	rules, err := extractPreScrapeRelabelings(logger, relabelConfigs)
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
		if isCluster {
			logger.Warn("Resulting ClusterPodMonitoring selector is empty. It will select and scrape all pods across all namespaces. Verify if this is intended.")
		} else {
			logger.Warn("Resulting PodMonitoring selector is empty. It will select and scrape all pods in this namespace. Verify if this is intended.")
		}
	}

	// Spec-level warnings for unsupported fields.
	warnUnsupportedSpecFields(logger, &pm.Spec)
	resolveScrapeClass(pm.Spec.ScrapeClassName, logger)
	validateScrapeProtocols(pm.Spec.ScrapeProtocols, logger)

	metadata := resolveMetadata(rules.ResourceCombined.Metadata, pm.Spec.AttachMetadata, isCluster, logger)

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
		metadata:         metadata,
		filterRunning:    filterRunning,
		limits:           limits,
		generatedSecrets: convCtx.getGeneratedSecrets(),
	}, nil
}

// convertToMonitoringResource is a parameterized helper that converts a PodMonitor to either a PodMonitoring or ClusterPodMonitoring resource.
func (c *PodMonitorConverter) convertToMonitoringResource(
	pm *pomonitoringv1.PodMonitor,
	targetNamespace string,
	logger *slog.Logger,
	cache *ResourceCache,
	isCluster bool,
) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	res, err := c.convertMonitorSpec(pm, targetNamespace, logger, cache, isCluster)
	if err != nil {
		return nil, nil, err
	}

	var u *unstructured.Unstructured
	if isCluster {
		u, err = buildClusterPodMonitoring(pm.ObjectMeta, res, logger)
	} else {
		u, err = buildPodMonitoring(pm.ObjectMeta, targetNamespace, res, logger)
	}
	if err != nil {
		return nil, nil, err
	}

	return u, res.generatedSecrets, nil
}

func (c *PodMonitorConverter) convertToPodMonitoring(pm *pomonitoringv1.PodMonitor, targetNamespace string, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	return c.convertToMonitoringResource(pm, targetNamespace, logger, cache, false)
}

func (c *PodMonitorConverter) convertToClusterPodMonitoring(pm *pomonitoringv1.PodMonitor, targetNamespace string, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	return c.convertToMonitoringResource(pm, targetNamespace, logger, cache, true)
}
