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
	"maps"
	"reflect"
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceGroup groups Services that share compatible selectors, ports, and labels.
// This allows them to be merged into a single output PodMonitoring resource.
type ServiceGroup struct {
	Selector     map[string]string
	PortMap      map[string]intstr.IntOrString
	TargetLabels map[string]string
	Services     []*corev1.Service
}

// ServiceMonitorConverter implements ResourceConverter for ServiceMonitor resources.
type ServiceMonitorConverter struct{}

// ImportKey returns the Kind of the resource this converter handles.
func (c *ServiceMonitorConverter) ImportKey() string {
	return KindServiceMonitor
}

// Convert translates a Prometheus Operator ServiceMonitor into GMP resources.
func (c *ServiceMonitorConverter) Convert(_ context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, error) {
	if unstruct == nil || unstruct.Object == nil {
		return nil, errors.New("cannot convert nil or uninitialized unstructured resource")
	}

	// 1. Decode unstructured input into typed ServiceMonitor struct.
	var serviceMonitor pomonitoringv1.ServiceMonitor
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &serviceMonitor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ServiceMonitor: %w", err)
	}

	logger.Info("Successfully decoded ServiceMonitor", slog.String("name", serviceMonitor.Name))

	// 2. Resolve target namespaces based on namespaceSelector settings.
	targetNamespaces, isClusterScoped, err := determineNamespaceScoping(serviceMonitor.Spec.NamespaceSelector, serviceMonitor.Namespace)
	if err != nil {
		return nil, err
	}

	if isClusterScoped {
		logger.Info("namespaceSelector selects 'any: true'. Translated to 'ClusterPodMonitoring'")
		u, generatedSecrets, err := c.convertToClusterPodMonitoring(&serviceMonitor, logger, cache)
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
	}

	// 3. Resolve backing Services and generate base resources (handles splitting).
	baseResources, generatedSecrets, err := c.convertToPodMonitoring(&serviceMonitor, logger, cache, targetNamespaces)
	if err != nil {
		return nil, err
	}

	// 4. Clone base resources for each target namespace.
	var outputs []*unstructured.Unstructured
	for _, base := range baseResources {
		for _, ns := range targetNamespaces {
			uClone := base.DeepCopy()
			uClone.SetNamespace(ns)
			outputs = append(outputs, uClone)
		}
	}
	for _, secret := range generatedSecrets {
		for _, ns := range targetNamespaces {
			uClone := secret.DeepCopy()
			uClone.SetNamespace(ns)
			outputs = append(outputs, uClone)
		}
	}
	return outputs, nil
}

func (c *ServiceMonitorConverter) convertToPodMonitoring(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
	targetNamespaces []string,
) (podMonitorings, generatedSecrets []*unstructured.Unstructured, err error) {
	// 1. Query the cache for all Services matching the selector.
	svcs, err := cache.findServicesBySelector(sm.Spec.Selector, targetNamespaces)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search for Services matching selector: %w", err)
	}
	if len(svcs) == 0 {
		return nil, nil, errors.New("corresponding Kubernetes Service was not found. Selector and port mappings cannot be resolved")
	}

	// 2. Group Services by configuration compatibility to detect conflicts.
	groups, err := groupServices(logger, svcs, sm.Spec.Endpoints, sm.Spec.TargetLabels)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to group Services: %w", err)
	}

	if len(groups) > 1 {
		logger.Warn("Services matched by selector have conflicts (different selectors, port mappings, or labels). Splitting into multiple PodMonitoring resources.",
			slog.Int("total_groups", len(groups)),
			slog.String("servicemonitor", sm.Name))
	}

	// 3. Construct a separate PodMonitoring resource for each compatible group.
	for _, group := range groups {
		res, err := c.buildSpecForGroup(sm, logger, cache, group, false)
		if err != nil {
			return nil, nil, err
		}

		name := sm.Name
		if len(groups) > 1 {
			// Suffix with the Service name to guarantee resource uniqueness when split.
			name = fmt.Sprintf("%s-%s", sm.Name, group.Services[0].GetName())
		}

		meta := sm.ObjectMeta.DeepCopy()
		meta.Name = name

		u, err := buildPodMonitoring(*meta, sm.Namespace, res, logger)
		if err != nil {
			return nil, nil, err
		}
		podMonitorings = append(podMonitorings, u)
		generatedSecrets = append(generatedSecrets, res.generatedSecrets...)
	}

	return podMonitorings, generatedSecrets, nil
}

func (c *ServiceMonitorConverter) convertToClusterPodMonitoring(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	// Search all namespaces for Services targeting ClusterPodMonitoring.
	svcs, err := cache.findServicesBySelector(sm.Spec.Selector, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search for Services matching selector: %w", err)
	}
	if len(svcs) == 0 {
		return nil, nil, errors.New("corresponding Kubernetes Service was not found. Selector and port mappings cannot be resolved")
	}

	groups, err := groupServices(logger, svcs, sm.Spec.Endpoints, sm.Spec.TargetLabels)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to group Services: %w", err)
	}

	// ClusterPodMonitoring is cluster-scoped and cannot be split by namespace.
	// We fall back to using the first group and ignore the others.
	if len(groups) > 1 {
		logger.Warn("Multiple incompatible Service groups found for ClusterPodMonitoring. Using the first group and ignoring others.",
			slog.Int("total_groups", len(groups)),
			slog.String("used_group_service", groups[0].Services[0].GetName()))
	}

	res, err := c.buildSpecForGroup(sm, logger, cache, groups[0], true)
	if err != nil {
		return nil, nil, err
	}

	u, err := buildClusterPodMonitoring(sm.ObjectMeta, res, logger)
	if err != nil {
		return nil, nil, err
	}

	return u, res.generatedSecrets, nil
}

func (c *ServiceMonitorConverter) buildSpecForGroup(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
	group *ServiceGroup,
	isClusterScoped bool,
) (*commonMonitorSpec, error) {
	convCtx := &conversionContext{
		logger:          logger,
		cache:           cache,
		sourceNamespace: sm.Namespace,
		targetNamespace: sm.Namespace,
		isClusterScoped: isClusterScoped,
	}

	// Extract pre-scrape relabelings.
	var relabelConfigs [][]pomonitoringv1.RelabelConfig
	for _, ep := range sm.Spec.Endpoints {
		relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
	}
	rules, err := extractPreScrapeRelabelings(logger, relabelConfigs)
	if err != nil {
		return nil, err
	}

	// Convert endpoints using group's resolved ports.
	endpoints, err := c.convertEndpointsForGroup(convCtx, sm.Spec.Endpoints, rules.PerEndpoint, group)
	if err != nil {
		return nil, err
	}

	// Apply Service targetLabels (statically resolved for this group).
	var serviceTargetLabelRules []monitoringv1.RelabelingRule
	if len(group.TargetLabels) > 0 {
		serviceTargetLabelRules = convertStaticTargetLabels(logger, group.TargetLabels)
	}

	if len(serviceTargetLabelRules) > 0 {
		for i := range endpoints {
			endpoints[i].MetricRelabeling = append(endpoints[i].MetricRelabeling, serviceTargetLabelRules...)
		}
	}

	// Merge Pod target labels and selector.
	mergedFromPod := mergeFromPod(logger, convertTargetLabels(logger, sm.Spec.PodTargetLabels, sm.Spec.JobLabel, "Pod"), rules.ResourceCombined.FromPod)

	baseSelector := metav1.LabelSelector{MatchLabels: group.Selector}
	mergedSelector, err := mergeLabelSelector(baseSelector, rules.ResourceCombined.MatchLabels, rules.ResourceCombined.MatchExpressions)
	if err != nil {
		return nil, err
	}

	// Spec-level warnings.
	resolveScrapeClass(sm.Spec.ScrapeClassName, logger)
	validateScrapeProtocols(sm.Spec.ScrapeProtocols, logger)

	filteredMetadata := filterMetadata(rules.ResourceCombined.Metadata, isClusterScoped, logger)
	metadata := resolveAttachMetadata(sm.Spec.AttachMetadata, filteredMetadata, isClusterScoped)

	var filterRunnings []*bool
	for _, ep := range sm.Spec.Endpoints {
		filterRunnings = append(filterRunnings, ep.FilterRunning)
	}
	filterRunning := resolveFilterRunning(filterRunnings, logger, isClusterScoped)

	limits := convertLimits(sm.Spec.SampleLimit, sm.Spec.LabelLimit, sm.Spec.LabelNameLengthLimit, sm.Spec.LabelValueLengthLimit)

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

func (c *ServiceMonitorConverter) convertEndpointsForGroup(
	convCtx *conversionContext,
	endpoints []pomonitoringv1.Endpoint,
	epResults []preScrapeRelabelingResult,
	group *ServiceGroup,
) ([]monitoringv1.ScrapeEndpoint, error) {
	var gmpEndpoints []monitoringv1.ScrapeEndpoint

	for i, ep := range endpoints {
		gmpEp := monitoringv1.ScrapeEndpoint{}

		// 1. Port mapping (Use pre-resolved port from group).
		if ep.Port == "" {
			return nil, fmt.Errorf("endpoint [%d]: port must be set", i)
		}

		resolvedPort, exists := group.PortMap[ep.Port]
		if !exists {
			return nil, fmt.Errorf("endpoint [%d]: port %q was not resolved for this group", i, ep.Port)
		}
		gmpEp.Port = resolvedPort

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

		// 4. Relabeling Rules.
		relabelings := combineAndConvertRelabelings(convCtx.logger, epResults[i].PromotedRules, ep.MetricRelabelConfigs)
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

		// Auth & TLS mappings.
		var safeTLS *pomonitoringv1.SafeTLSConfig
		if ep.TLSConfig != nil {
			safeTLS = &ep.TLSConfig.SafeTLSConfig
			// Warn about unsafe fields if they are set.
			if ep.TLSConfig.CAFile != "" {
				convCtx.logger.Warn("Field 'tlsConfig.caFile' is unsupported in GMP and has been dropped. Use 'tlsConfig.ca' (Secret/ConfigMap) instead.")
			}
			if ep.TLSConfig.CertFile != "" {
				convCtx.logger.Warn("Field 'tlsConfig.certFile' is unsupported in GMP and has been dropped. Use 'tlsConfig.cert' instead.")
			}
			if ep.TLSConfig.KeyFile != "" {
				convCtx.logger.Warn("Field 'tlsConfig.keyFile' is unsupported in GMP and has been dropped. Use 'tlsConfig.keySecret' instead.")
			}
		}

		var bearerTokenSecret corev1.SecretKeySelector
		// nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
		if ep.BearerTokenSecret != nil {
			// nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			bearerTokenSecret = *ep.BearerTokenSecret
		}
		err = convCtx.applyAuthAndTLS(&gmpEp, ep.BasicAuth, ep.OAuth2, safeTLS, ep.Authorization, bearerTokenSecret) // nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("endpoint [%d]: %w", i, err)
		}

		// Warnings for Unsupported Fields.
		warnUnsupportedEndpointFields(convCtx.logger, ep.FollowRedirects, ep.EnableHttp2, ep.HonorLabels, ep.HonorTimestamps, ep.TrackTimestampsStaleness, i)

		gmpEndpoints = append(gmpEndpoints, gmpEp)
	}

	return gmpEndpoints, nil
}

// groupServices groups matched Services by their resolved target selectors and port/label mappings.
func groupServices(
	logger *slog.Logger,
	svcs []*corev1.Service,
	endpoints []pomonitoringv1.Endpoint,
	targetLabels []string,
) ([]*ServiceGroup, error) {
	var groups []*ServiceGroup

	for _, svc := range svcs {
		// 1. Extract and validate selector.
		selectorString := svc.Spec.Selector
		if len(selectorString) == 0 {
			return nil, fmt.Errorf("service %q has no selector (targets external or static endpoints). GMP Managed Collection only supports scraping in-cluster Pod targets", svc.GetName())
		}

		// 2. Resolve ports for this Service.
		portMap := make(map[string]intstr.IntOrString)
		for _, ep := range endpoints {
			if ep.Port == "" {
				continue
			}
			resolvedPort, err := resolveServicePort(logger, svc, ep.Port)
			if err != nil {
				return nil, fmt.Errorf("service %q: failed to resolve port %q: %w", svc.GetName(), ep.Port, err)
			}
			portMap[ep.Port] = resolvedPort
		}

		// 3. Resolve target labels for this Service.
		resolvedLabels := make(map[string]string)
		for _, labelName := range targetLabels {
			if val, found := svc.Labels[labelName]; found {
				resolvedLabels[labelName] = val
			}
		}

		// 4. Find a compatible group.
		var matchedGroup *ServiceGroup
		for _, g := range groups {
			if g.canMergeWith(selectorString, portMap, resolvedLabels) {
				matchedGroup = g
				break
			}
		}

		// 5. Merge into existing group or create new group.
		if matchedGroup != nil {
			matchedGroup.Services = append(matchedGroup.Services, svc)
			// Merge complementary mappings.
			maps.Copy(matchedGroup.PortMap, portMap)
			maps.Copy(matchedGroup.TargetLabels, resolvedLabels)
		} else {
			groups = append(groups, &ServiceGroup{
				Selector:     selectorString,
				PortMap:      portMap,
				TargetLabels: resolvedLabels,
				Services:     []*corev1.Service{svc},
			})
		}
	}

	return groups, nil
}

// canMergeWith checks if a Service's resolved selector, ports, and labels
// are compatible with this ServiceGroup (i.e. no conflicts).
func (g *ServiceGroup) canMergeWith(
	selector map[string]string,
	portMap map[string]intstr.IntOrString,
	targetLabels map[string]string,
) bool {
	// 1. Selector must match exactly.
	if !reflect.DeepEqual(g.Selector, selector) {
		return false
	}

	// 2. Check for port mapping conflicts.
	for portName, targetPort := range portMap {
		if existingTargetPort, exists := g.PortMap[portName]; exists {
			if !reflect.DeepEqual(existingTargetPort, targetPort) {
				return false
			}
		}
	}

	// 3. Check for target label conflicts.
	for labelName, labelVal := range targetLabels {
		if existingVal, exists := g.TargetLabels[labelName]; exists {
			if existingVal != labelVal {
				return false
			}
		}
	}

	return true
}

func convertStaticTargetLabels(logger *slog.Logger, labels map[string]string) []monitoringv1.RelabelingRule {
	var rules []monitoringv1.RelabelingRule
	var keys []string
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := labels[k]
		rule := monitoringv1.RelabelingRule{
			TargetLabel: k,
			Replacement: v,
			Action:      "replace",
		}
		rules = append(rules, rule)
		logger.Info("Service label mapped statically to metricRelabeling",
			slog.String("label", fmt.Sprintf("%s: %s", k, v)))
	}
	return rules
}
