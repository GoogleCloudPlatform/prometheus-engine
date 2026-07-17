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
	prommodel "github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	epResults []PreScrapeRelabelingResult,
) ([]monitoringv1.ScrapeEndpoint, error) {
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
		gmpEp.Interval = string(ep.Interval)
		gmpEp.Timeout = string(ep.ScrapeTimeout)

		// TODO(M2): Inherit global scrape interval from Prometheus CR if empty.
		if gmpEp.Interval == "" {
			convCtx.logger.Warn("Scrape interval is empty. Defaulting to '30s' as GMP requires this field.")
			gmpEp.Interval = "30s"
		}

		intDur, err := prommodel.ParseDuration(gmpEp.Interval)
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: invalid interval %q: %w", i, gmpEp.Interval, err)
		}

		if gmpEp.Timeout != "" {
			toDur, err := prommodel.ParseDuration(gmpEp.Timeout)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: invalid scrapeTimeout %q: %w", i, gmpEp.Timeout, err)
			}
			if toDur > intDur {
				convCtx.logger.Warn("Scrape timeout is larger than scrape interval. Capping timeout to interval.",
					slog.String("timeout", gmpEp.Timeout),
					slog.String("interval", gmpEp.Interval))
				gmpEp.Timeout = gmpEp.Interval
			}
		}
		// TODO(M2): Inherit global scrape timeout from Prometheus CR if empty.

		// 4. Relabeling Rules (Promoted Pre-Scrape + MetricRelabelings).
		totalRules := len(epResults[i].PromotedRules) + len(ep.MetricRelabelConfigs)
		var allRules []monitoringv1.RelabelingRule
		if totalRules > 0 {
			allRules = make([]monitoringv1.RelabelingRule, 0, totalRules)
			allRules = append(allRules, epResults[i].PromotedRules...)
			if len(ep.MetricRelabelConfigs) > 0 {
				rules, err := convertMetricRelabelings(convCtx.logger, ep.MetricRelabelConfigs)
				if err != nil {
					return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
				}
				allRules = append(allRules, rules...)
			}
		}
		gmpEp.MetricRelabeling = allRules

		// Proxy Settings.
		if ep.ProxyURL != nil {
			if strings.Contains(*ep.ProxyURL, "@") {
				return nil, fmt.Errorf("endpoint [%d]: proxyUrl contains credentials (matches '@'), which is blocked by GMP API validation", i)
			}
			gmpEp.ProxyURL = *ep.ProxyURL
		}

		// noProxy, proxyConnectHeader, and proxyFromEnvironment fields are silently dropped.
		// The pinned Prometheus Operator version lacks these fields, and GMP does not support them anyway.

		// Auth & TLS mappings.
		if ep.BasicAuth != nil {
			ba, err := convCtx.convertBasicAuth(ep.BasicAuth)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: basicAuth: %w", i, err)
			}
			gmpEp.BasicAuth = ba
		}
		if ep.OAuth2 != nil {
			oa, err := convCtx.convertOAuth2(ep.OAuth2)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: oAuth2: %w", i, err)
			}
			gmpEp.OAuth2 = oa
		}
		if ep.TLSConfig != nil {
			tls, err := convCtx.convertSafeTLSConfig(ep.TLSConfig)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: tlsConfig: %w", i, err)
			}
			gmpEp.TLS = tls
		}
		if ep.Authorization != nil {
			auth, err := convCtx.convertAuthorization(ep.Authorization)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: authorization: %w", i, err)
			}
			gmpEp.Authorization = auth
		}

		// Handle deprecated BearerTokenSecret -> Authorization.
		if ep.BearerTokenSecret.Name != "" { // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			if gmpEp.Authorization != nil {
				convCtx.logger.Warn("Endpoint has both 'bearerTokenSecret' and 'authorization' defined. Dropping 'bearerTokenSecret'.",
					slog.Int("endpoint_index", i))
			} else {
				tokenSecret := ep.BearerTokenSecret // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
				auth, err := convCtx.convertAuthorization(&pomonitoringv1.SafeAuthorization{Credentials: &tokenSecret})
				if err != nil {
					return nil, fmt.Errorf("endpoint [%d]: bearerTokenSecret: %w", i, err)
				}
				gmpEp.Authorization = auth
			}
		}

		// 5. Warnings for Unsupported Fields in Endpoint.
		if ep.FollowRedirects != nil && !*ep.FollowRedirects {
			convCtx.logger.Warn("Field 'followRedirects: false' is unsupported by GMP Managed Collection and has been dropped. The collector will always follow redirects.")
		}
		if ep.EnableHttp2 != nil && !*ep.EnableHttp2 {
			convCtx.logger.Warn("Field 'enableHttp2: false' is unsupported by GMP Managed Collection and has been dropped. The collector will always negotiate HTTP/2 for TLS connections.")
		}

		if ep.HonorLabels {
			convCtx.logger.Warn("Field 'honorLabels: true' is unsupported and dropped. GMP always overrides conflicting labels. Clashing metric labels will be renamed with the 'exported_' prefix.")
		}
		if ep.HonorTimestamps != nil && *ep.HonorTimestamps {
			convCtx.logger.Warn("Field 'honorTimestamps: true' is unsupported and dropped. GMP always uses the scrape ingestion timestamp. Target metric timestamps will be ignored.")
		}
		if ep.TrackTimestampsStaleness != nil {
			convCtx.logger.Warn("Field 'trackTimestampsStaleness' is unsupported in GMP and has been dropped.")
		}

		gmpEndpoints = append(gmpEndpoints, gmpEp)
	}

	return gmpEndpoints, nil
}

func (c *PodMonitorConverter) convertToPodMonitoring(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	convCtx := &conversionContext{
		logger:    logger,
		cache:     cache,
		namespace: pm.Namespace,
	}
	rules := extractPreScrapeRelabelings(logger, pm.Spec.PodMetricsEndpoints)
	endpoints, err := c.convertEndpoints(convCtx, pm.Spec.PodMetricsEndpoints, rules.PerEndpoint)
	if err != nil {
		return nil, nil, err
	}

	mergedFromPod := mergeFromPod(logger, convertTargetLabels(logger, pm.Spec.PodTargetLabels, pm.Spec.JobLabel, "Pod"), rules.ResourceCombined.FromPod)
	mergedSelector := mergeLabelSelector(logger, pm.Spec.Selector, rules.ResourceCombined.MatchLabels, rules.ResourceCombined.MatchExpressions)

	// Spec-level warnings for unsupported fields.
	// TODO(M2): Resolve and merge ScrapeClass configurations from Prometheus CR if scrapeClassName is specified.
	if pm.Spec.ScrapeClassName != nil && *pm.Spec.ScrapeClassName != "" {
		logger.Warn(fmt.Sprintf("ScrapeClass %q was not found in the inputs. The 'scrapeClassName' field has been dropped and inherited settings will be lost.", *pm.Spec.ScrapeClassName))
	}
	for _, sp := range pm.Spec.ScrapeProtocols {
		if strings.Contains(strings.ToLower(string(sp)), "protobuf") {
			logger.Warn("Scrape protocol settings (scrapeProtocols) requiring Protobuf are unsupported. Scrapes may fail if target lacks text fallback.")
			break
		}
	}

	metadata := rules.ResourceCombined.Metadata
	var metadataCopy []string
	if pm.Spec.AttachMetadata != nil && pm.Spec.AttachMetadata.Node != nil && *pm.Spec.AttachMetadata.Node {
		if metadata == nil {
			metadata = &[]string{"node"}
		} else if !slices.Contains(*metadata, "node") {
			metadataCopy = append(slices.Clone(*metadata), "node")
			metadata = &metadataCopy
		}
	}

	var hasFalse, hasTrue bool
	for _, ep := range pm.Spec.PodMetricsEndpoints {
		if ep.FilterRunning != nil && !*ep.FilterRunning {
			hasFalse = true
		} else {
			hasTrue = true
		}
	}
	var filterRunning *bool
	if hasFalse {
		falseVal := false
		filterRunning = &falseVal
		if hasTrue {
			logger.Warn("Endpoint-level configuration conflict detected: some endpoints are configured with 'filterRunning: false' and others with 'true' (or default), but GMP only supports 'filterRunning' at the resource level. Setting 'filterRunning: false' globally on the PodMonitoring resource.")
		}
	}

	limits := convertLimits(pm.Spec.SampleLimit, pm.Spec.LabelLimit, pm.Spec.LabelNameLengthLimit, pm.Spec.LabelValueLengthLimit)

	gmpPM := &monitoringv1.PodMonitoring{
		TypeMeta:   BuildTypeMeta(KindPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, pm.Namespace, logger),
		Spec: monitoringv1.PodMonitoringSpec{
			Selector:  mergedSelector,
			Endpoints: endpoints,
			TargetLabels: monitoringv1.TargetLabels{
				FromPod:  mergedFromPod,
				Metadata: metadata,
			},
			Limits:        limits,
			FilterRunning: filterRunning,
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpPM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal PodMonitoring: %w", err)
	}
	removeNilFields(unstructuredMap)

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindPodMonitoring)

	return u, convCtx.getGeneratedSecrets(), nil
}

func (c *PodMonitorConverter) convertToClusterPodMonitoring(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	convCtx := &conversionContext{
		logger:    logger,
		cache:     cache,
		namespace: pm.Namespace,
	}
	rules := extractPreScrapeRelabelings(logger, pm.Spec.PodMetricsEndpoints)
	endpoints, err := c.convertEndpoints(convCtx, pm.Spec.PodMetricsEndpoints, rules.PerEndpoint)
	if err != nil {
		return nil, nil, err
	}

	mergedFromPod := mergeFromPod(logger, convertTargetLabels(logger, pm.Spec.PodTargetLabels, pm.Spec.JobLabel, "Pod"), rules.ResourceCombined.FromPod)
	mergedSelector := mergeLabelSelector(logger, pm.Spec.Selector, rules.ResourceCombined.MatchLabels, rules.ResourceCombined.MatchExpressions)

	// Spec-level warnings for unsupported fields.
	// TODO(M2): Resolve and merge ScrapeClass configurations from Prometheus CR if scrapeClassName is specified.
	if pm.Spec.ScrapeClassName != nil && *pm.Spec.ScrapeClassName != "" {
		logger.Warn(fmt.Sprintf("ScrapeClass %q was not found in the pre-scanned inputs. The 'scrapeClassName' field has been dropped and inherited settings will be lost.", *pm.Spec.ScrapeClassName))
	}
	for _, sp := range pm.Spec.ScrapeProtocols {
		if strings.Contains(strings.ToLower(string(sp)), "protobuf") {
			logger.Warn("Scrape protocol settings (scrapeProtocols) requiring Protobuf are unsupported. Scrapes may fail if target lacks text fallback.")
			break
		}
	}

	metadata := rules.ResourceCombined.Metadata
	var metadataCopy []string
	if pm.Spec.AttachMetadata != nil && pm.Spec.AttachMetadata.Node != nil && *pm.Spec.AttachMetadata.Node {
		if metadata == nil {
			metadata = &[]string{"node"}
		} else if !slices.Contains(*metadata, "node") {
			metadataCopy = append(slices.Clone(*metadata), "node")
			metadata = &metadataCopy
		}
	}

	var hasFalse, hasTrue bool
	for _, ep := range pm.Spec.PodMetricsEndpoints {
		if ep.FilterRunning != nil && !*ep.FilterRunning {
			hasFalse = true
		} else {
			hasTrue = true
		}
	}
	var filterRunning *bool
	if hasFalse {
		falseVal := false
		filterRunning = &falseVal
		if hasTrue {
			logger.Warn("Endpoint-level configuration conflict detected: some endpoints are configured with 'filterRunning: false' and others with 'true' (or default), but GMP only supports 'filterRunning' at the resource level. Setting 'filterRunning: false' globally on the ClusterPodMonitoring resource.")
		}
	}

	limits := convertLimits(pm.Spec.SampleLimit, pm.Spec.LabelLimit, pm.Spec.LabelNameLengthLimit, pm.Spec.LabelValueLengthLimit)

	gmpCPM := &monitoringv1.ClusterPodMonitoring{
		TypeMeta:   BuildTypeMeta(KindClusterPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, "", logger),
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector:  mergedSelector,
			Endpoints: endpoints,
			TargetLabels: monitoringv1.ClusterTargetLabels{
				FromPod:  mergedFromPod,
				Metadata: metadata,
			},
			Limits:        limits,
			FilterRunning: filterRunning,
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpCPM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ClusterPodMonitoring: %w", err)
	}
	removeNilFields(unstructuredMap)

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindClusterPodMonitoring)

	return u, convCtx.getGeneratedSecrets(), nil
}
