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
			// Secrets needed per namespace.
			for _, g := range generatedSecrets {
				gClone := g.DeepCopy()
				gClone.SetNamespace(ns)
				outputs = append(outputs, gClone)
			}
		}
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
				convCtx.logger.Warn(fmt.Sprintf("Scrape timeout %q is larger than scrape interval %q. Capping timeout to %q.",
					gmpEp.Timeout, gmpEp.Interval, gmpEp.Interval))
				gmpEp.Timeout = gmpEp.Interval
			}
		}
		// TODO(M2): Inherit global scrape timeout from Prometheus CR if empty.

		// 4. Relabeling Rules (MetricRelabelings).
		if len(ep.MetricRelabelConfigs) > 0 {
			rules, err := convertMetricRelabelings(convCtx.logger, ep.MetricRelabelConfigs)
			if err != nil {
				return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
			}
			gmpEp.MetricRelabeling = rules
		}

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
			gmpEp.BasicAuth = convertBasicAuth(convCtx, ep.BasicAuth)
		}
		if ep.OAuth2 != nil {
			gmpEp.OAuth2 = convertOAuth2(convCtx, ep.OAuth2)
		}
		if ep.TLSConfig != nil {
			gmpEp.TLS = convertSafeTLSConfig(convCtx, ep.TLSConfig)
		}
		if ep.Authorization != nil {
			gmpEp.Authorization = convertAuthorization(convCtx, ep.Authorization)
		}

		// Handle deprecated BearerTokenSecret -> Authorization.
		if ep.BearerTokenSecret.Name != "" { // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			if gmpEp.Authorization != nil {
				convCtx.logger.Warn(fmt.Sprintf("Endpoint [%d] has both 'bearerTokenSecret' and 'authorization' defined. Dropping 'bearerTokenSecret'.", i))
			} else {
				gmpEp.Authorization = convertAuthorization(convCtx, &pomonitoringv1.SafeAuthorization{Credentials: &ep.BearerTokenSecret}) // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			}
		}

		// 5. Warnings for Unsupported Fields in Endpoint.
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
	endpoints, err := c.convertEndpoints(convCtx, pm.Spec.PodMetricsEndpoints)
	if err != nil {
		return nil, nil, err
	}

	gmpPM := &monitoringv1.PodMonitoring{
		TypeMeta:   BuildTypeMeta(KindPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, pm.Namespace, logger),
		Spec: monitoringv1.PodMonitoringSpec{
			Selector:  pm.Spec.Selector,
			Endpoints: endpoints,
			TargetLabels: monitoringv1.TargetLabels{
				FromPod: convertTargetLabels(logger, pm.Spec.PodTargetLabels, pm.Spec.JobLabel, "Pod"),
			},
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpPM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal PodMonitoring: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindPodMonitoring)

	return u, convCtx.generatedSecrets, nil
}

func (c *PodMonitorConverter) convertToClusterPodMonitoring(pm *pomonitoringv1.PodMonitor, logger *slog.Logger, cache *ResourceCache) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	convCtx := &conversionContext{
		logger:    logger,
		cache:     cache,
		namespace: pm.Namespace,
	}
	endpoints, err := c.convertEndpoints(convCtx, pm.Spec.PodMetricsEndpoints)
	if err != nil {
		return nil, nil, err
	}

	gmpCPM := &monitoringv1.ClusterPodMonitoring{
		TypeMeta:   BuildTypeMeta(KindClusterPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, "", logger),
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector:  pm.Spec.Selector,
			Endpoints: endpoints,
			TargetLabels: monitoringv1.ClusterTargetLabels{
				FromPod: convertTargetLabels(logger, pm.Spec.PodTargetLabels, pm.Spec.JobLabel, "Pod"),
			},
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpCPM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ClusterPodMonitoring: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindClusterPodMonitoring)

	return u, convCtx.generatedSecrets, nil
}
