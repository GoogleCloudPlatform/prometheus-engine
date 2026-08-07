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
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/strutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// serviceGroup groups Services that share compatible selectors, ports, and labels.
// This allows them to be merged into a single output PodMonitoring resource.
type serviceGroup struct {
	selector     map[string]string
	portMap      map[string]intstr.IntOrString
	targetLabels map[string]string
	services     []*corev1.Service
	todos        []todoItem
}

// namespaces returns a sorted slice of unique namespaces where Services in this group reside.
func (g *serviceGroup) namespaces() []string {
	unique := make(map[string]bool)
	var ns []string
	for _, svc := range g.services {
		n := svc.Namespace
		if !unique[n] {
			unique[n] = true
			ns = append(ns, n)
		}
	}
	slices.Sort(ns)
	return ns
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
		clusterPodMonitorings, generatedSecrets, err := c.convertToClusterPodMonitoring(&serviceMonitor, logger, cache)
		if err != nil {
			return nil, err
		}
		var outputs []*unstructured.Unstructured
		outputs = append(outputs, clusterPodMonitorings...)
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

	// 3. Resolve backing Services and generate PodMonitoring resources per group and namespace.
	podMonitorings, generatedSecrets, err := c.convertToPodMonitoring(&serviceMonitor, logger, cache, targetNamespaces)
	if err != nil {
		return nil, err
	}

	var outputs []*unstructured.Unstructured
	outputs = append(outputs, podMonitorings...)
	outputs = append(outputs, generatedSecrets...)
	return outputs, nil
}

func (c *ServiceMonitorConverter) convertToMonitoringResources(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
	targetNamespaces []string,
	isClusterScoped bool,
) (outputs, generatedSecrets []*unstructured.Unstructured, err error) {
	groups, err := c.findAndGroupServices(sm, targetNamespaces, logger, cache)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search for Services matching selector: %w", err)
	}
	if len(groups) == 0 {
		return nil, nil, nil
	}

	if len(groups) > 1 {
		targetKind := "PodMonitoring"
		if isClusterScoped {
			targetKind = "ClusterPodMonitoring"
		}
		logger.Warn(fmt.Sprintf("Services matched by selector have conflicts (different selectors, port mappings, or labels). Splitting into multiple %s resources.", targetKind),
			slog.Int("total_groups", len(groups)),
			slog.String("servicemonitor", sm.Name))
	}

	for _, group := range groups {
		res, err := c.buildSpecForGroup(sm, logger, cache, group, isClusterScoped)
		if err != nil {
			return nil, nil, err
		}

		name := sm.Name
		if len(groups) > 1 {
			// Suffix with Service name for PodMonitoring, or namespace and Service name for ClusterPodMonitoring to prevent cluster-scoped collisions.
			suffix := group.services[0].Name
			if isClusterScoped {
				suffix = fmt.Sprintf("%s-%s", group.services[0].Namespace, group.services[0].Name)
			}
			name = makeUniqueResourceName(sm.Name, suffix)
		}

		meta := sm.ObjectMeta.DeepCopy()
		meta.Name = name

		if isClusterScoped {
			u, err := buildClusterPodMonitoring(*meta, res, logger)
			if err != nil {
				return nil, nil, err
			}
			outputs = append(outputs, u)
			generatedSecrets = append(generatedSecrets, res.generatedSecrets...)
		} else {
			for _, ns := range group.namespaces() {
				u, err := buildPodMonitoring(*meta, ns, res, logger)
				if err != nil {
					return nil, nil, err
				}
				outputs = append(outputs, u)
				for _, secret := range res.generatedSecrets {
					sClone := secret.DeepCopy()
					sClone.SetNamespace(ns)
					generatedSecrets = append(generatedSecrets, sClone)
				}
			}
		}
	}

	return outputs, generatedSecrets, nil
}

func (c *ServiceMonitorConverter) convertToPodMonitoring(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
	targetNamespaces []string,
) (podMonitorings, generatedSecrets []*unstructured.Unstructured, err error) {
	return c.convertToMonitoringResources(sm, logger, cache, targetNamespaces, false)
}

// findAndGroupServices queries the cache for Services matching the selector and groups them by configuration compatibility.
func (c *ServiceMonitorConverter) findAndGroupServices(
	sm *pomonitoringv1.ServiceMonitor,
	targetNamespaces []string,
	logger *slog.Logger,
	cache *ResourceCache,
) ([]*serviceGroup, error) {
	svcs, err := cache.findServicesBySelector(sm.Spec.Selector, targetNamespaces)
	if err != nil {
		return nil, fmt.Errorf("failed to search for Services matching selector: %w", err)
	}
	if len(svcs) == 0 {
		logger.Warn("Corresponding Kubernetes Service was not found. Emitting draft PodMonitoring with placeholder selector and ports.",
			slog.String("servicemonitor", sm.Name))
		portMap := make(map[string]intstr.IntOrString)
		for _, ep := range sm.Spec.Endpoints {
			k := endpointPortKey(ep)
			if k == "" {
				k = "TODO_SET_PORT"
			}
			portMap[k] = intstr.FromString("TODO_RESOLVE_PORT")
		}
		dummySvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sm.Name,
				Namespace: sm.Namespace,
			},
		}
		return []*serviceGroup{
			{
				selector: map[string]string{
					"app": "TODO_SET_POD_SELECTOR",
				},
				portMap:  portMap,
				services: []*corev1.Service{dummySvc},
				todos: []todoItem{
					{
						category: "ERROR",
						reason:   "Corresponding Kubernetes Service was not found. Selector and port mappings could not be resolved.",
						action:   "Define target pod selector in 'spec.selector.matchLabels' and verify endpoint ports.",
					},
				},
			},
		}, nil
	}

	groups, err := groupServices(logger, sm.Spec.TargetLabels, sm, svcs)
	if err != nil {
		return nil, fmt.Errorf("failed to group Services: %w", err)
	}
	return groups, nil
}

func (c *ServiceMonitorConverter) convertToClusterPodMonitoring(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
) (clusterPodMonitorings, generatedSecrets []*unstructured.Unstructured, err error) {
	return c.convertToMonitoringResources(sm, logger, cache, nil, true)
}

func (c *ServiceMonitorConverter) buildSpecForGroup(
	sm *pomonitoringv1.ServiceMonitor,
	logger *slog.Logger,
	cache *ResourceCache,
	group *serviceGroup,
	isClusterScoped bool,
) (*commonMonitorSpec, error) {
	convCtx := &conversionContext{
		logger:          logger,
		cache:           cache,
		sourceNamespace: sm.Namespace,
		targetNamespace: sm.Namespace,
		isClusterScoped: isClusterScoped,
	}
	convCtx.todos = append(convCtx.todos, group.todos...)

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
	if len(group.targetLabels) > 0 {
		serviceTargetLabelRules = convertStaticTargetLabels(logger, group.targetLabels)
	}

	if len(serviceTargetLabelRules) > 0 {
		for i := range endpoints {
			endpoints[i].MetricRelabeling = append(slices.Clone(serviceTargetLabelRules), endpoints[i].MetricRelabeling...)
		}
	}

	// Merge Pod target labels and selector.
	mergedFromPod := mergeFromPod(logger, convertTargetLabels(logger, sm.Spec.PodTargetLabels, "", "Pod"), rules.ResourceCombined.FromPod)

	baseSelector := metav1.LabelSelector{MatchLabels: group.selector}
	mergedSelector := convCtx.mergeLabelSelector(baseSelector, rules.ResourceCombined.MatchLabels, rules.ResourceCombined.MatchExpressions)
	var todos []todoItem
	todos = append(todos, rules.ResourceCombined.Todos...)

	if len(mergedSelector.MatchLabels) == 0 && len(mergedSelector.MatchExpressions) == 0 {
		if isClusterScoped {
			logger.Warn("Resulting ClusterPodMonitoring selector is empty. It will select and scrape all pods across all namespaces. Verify if this is intended.")
			todos = append(todos, todoItem{
				category: "WARNING",
				reason:   "Resulting ClusterPodMonitoring selector is empty and matches all pods across all namespaces.",
				action:   "Define explicit 'matchLabels' in 'spec.selector'.",
			})
		} else {
			logger.Warn("Resulting PodMonitoring selector is empty. It will select and scrape all pods in this namespace. Verify if this is intended.")
			todos = append(todos, todoItem{
				category: "WARNING",
				reason:   "Resulting PodMonitoring selector is empty and matches all pods in this namespace.",
				action:   "Define explicit 'matchLabels' in 'spec.selector'.",
			})
		}
	}

	// Spec-level warnings for unsupported fields.
	warnUnsupportedMonitorSpecFields(logger, sm.Spec.TargetLimit, sm.Spec.KeepDroppedTargets, sm.Spec.BodySizeLimit)
	resolveScrapeClass(sm.Spec.ScrapeClassName, logger)
	validateScrapeProtocols(sm.Spec.ScrapeProtocols, logger)

	metadata := resolveMetadata(rules.ResourceCombined.Metadata, sm.Spec.AttachMetadata, isClusterScoped, logger)

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
		todos:            append(todos, convCtx.todos...),
	}, nil
}

func (c *ServiceMonitorConverter) convertEndpointsForGroup(
	convCtx *conversionContext,
	endpoints []pomonitoringv1.Endpoint,
	epResults []preScrapeRelabelingResult,
	group *serviceGroup,
) ([]monitoringv1.ScrapeEndpoint, error) {
	if len(epResults) != len(endpoints) {
		return nil, fmt.Errorf("internal error: pre-scrape relabeling results length (%d) does not match endpoints length (%d)", len(epResults), len(endpoints))
	}

	var gmpEndpoints []monitoringv1.ScrapeEndpoint

	for i, ep := range endpoints {
		gmpEp := monitoringv1.ScrapeEndpoint{}

		// 1. Port mapping (Use pre-resolved port from group).
		portKey := endpointPortKey(ep)
		if portKey == "" {
			portKey = "TODO_SET_PORT"
		}
		resolvedPort, exists := group.portMap[portKey]
		if !exists {
			resolvedPort = intstr.FromString("TODO_RESOLVE_PORT")
		}
		gmpEp.Port = resolvedPort

		// 2. Basic Fields.
		gmpEp.Path = ep.Path
		gmpEp.Scheme = strings.ToLower(ep.Scheme)
		gmpEp.Params = ep.Params

		// 3. Scrape Intervals & Timeouts.
		interval, timeout := convCtx.resolveScrapeIntervalAndTimeout(string(ep.Interval), string(ep.ScrapeTimeout))
		gmpEp.Interval = interval
		gmpEp.Timeout = timeout

		// 4. Relabeling Rules.
		gmpEp.MetricRelabeling = combineAndConvertRelabelings(convCtx.logger, epResults[i].PromotedRules, ep.MetricRelabelConfigs)

		// Proxy Settings.
		gmpEp.ProxyURL = convCtx.convertProxyURL(ep.ProxyURL)

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
		convCtx.applyAuthAndTLS(&gmpEp, ep.BasicAuth, ep.OAuth2, safeTLS, ep.Authorization, bearerTokenSecret) // nolint:staticcheck

		// Warnings for Unsupported Fields.
		warnUnsupportedEndpointFields(convCtx.logger, ep.FollowRedirects, ep.EnableHttp2, ep.HonorLabels, ep.HonorTimestamps, ep.TrackTimestampsStaleness, i)

		gmpEndpoints = append(gmpEndpoints, gmpEp)
	}

	return gmpEndpoints, nil
}

// groupServices groups matched Services by their resolved target selectors and port/label mappings.
func groupServices(
	logger *slog.Logger,
	targetLabels []string,
	sm *pomonitoringv1.ServiceMonitor,
	svcs []*corev1.Service,
) ([]*serviceGroup, error) {
	var groups []*serviceGroup

	for _, svc := range svcs {
		// 1. Extract and validate selector.
		selectorString := svc.Spec.Selector
		if len(selectorString) == 0 {
			logger.Info("Service targets external endpoints without a pod selector. GMP Managed Collection only supports in-cluster Pods. Skipping resource.",
				slog.String("migration_status", "skipped"),
				slog.String("service", svc.GetName()),
			)
			continue
		}

		// 2. Resolve ports for this Service.
		var groupTodos []todoItem
		portMap := make(map[string]intstr.IntOrString)
		for i, ep := range sm.Spec.Endpoints {
			portKey := endpointPortKey(ep)
			if portKey == "" {
				portKey = "TODO_SET_PORT"
				groupTodos = append(groupTodos, todoItem{
					category: "ERROR",
					reason:   fmt.Sprintf("Endpoint [%d] does not specify a 'port' or 'targetPort'.", i),
					action:   "Specify a valid port name or number in 'spec.endpoints[].port'.",
				})
				portMap[portKey] = intstr.FromString("TODO_SET_PORT")
			} else {
				resolvedPort, todo := resolveServicePort(logger, svc, portKey)
				if todo != nil {
					groupTodos = append(groupTodos, *todo)
				}
				portMap[portKey] = resolvedPort
			}
		}

		// 3. Resolve target labels for this Service.
		resolvedLabels := make(map[string]string)
		for _, labelName := range targetLabels {
			if val, found := svc.Labels[labelName]; found {
				resolvedLabels[labelName] = val
			} else {
				logger.Warn("Service-level targetLabel was not found on Service. Skipping mapping.",
					slog.String("label", labelName),
					slog.String("service", svc.Name))
			}
		}

		if sm.Spec.JobLabel != "" {
			if val, found := svc.Labels[sm.Spec.JobLabel]; found {
				resolvedLabels["job"] = val
			} else {
				logger.Warn("Service-level jobLabel was not found on Service. Skipping job mapping.",
					slog.String("job_label", sm.Spec.JobLabel),
					slog.String("service", svc.Name))
			}
		}

		// 4. Find a compatible group.
		var matchedGroup *serviceGroup
		for _, g := range groups {
			if g.canMergeWith(svc.Spec.Selector, portMap, resolvedLabels) {
				matchedGroup = g
				break
			}
		}

		// 5. Merge into existing group or create new group.
		if matchedGroup != nil {
			matchedGroup.services = append(matchedGroup.services, svc)
			// Merge complementary mappings.
			maps.Copy(matchedGroup.portMap, portMap)
			maps.Copy(matchedGroup.targetLabels, resolvedLabels)
			matchedGroup.todos = append(matchedGroup.todos, groupTodos...)
		} else {
			groups = append(groups, &serviceGroup{
				selector:     svc.Spec.Selector,
				portMap:      portMap,
				targetLabels: resolvedLabels,
				services:     []*corev1.Service{svc},
				todos:        groupTodos,
			})
		}
	}

	return groups, nil
}

// canMergeWith checks if a Service's resolved selector, ports, and labels
// are compatible with this ServiceGroup (i.e. no conflicts).
func (g *serviceGroup) canMergeWith(
	selector map[string]string,
	portMap map[string]intstr.IntOrString,
	targetLabels map[string]string,
) bool {
	// 1. Selector must match exactly.
	if !maps.Equal(g.selector, selector) {
		return false
	}

	// 2. Check for port mapping conflicts.
	for portName, targetPort := range portMap {
		if existingTargetPort, exists := g.portMap[portName]; exists {
			if existingTargetPort != targetPort {
				return false
			}
		}
	}

	// 3. Target label mappings must match exactly across merged Services.
	// Otherwise, unlabeled or partially labeled Services would incorrectly inherit static target label values from other Services in the group.
	if !maps.Equal(g.targetLabels, targetLabels) {
		return false
	}

	return true
}

// convertStaticTargetLabels maps Service target labels to static metricRelabeling rules.
func convertStaticTargetLabels(logger *slog.Logger, labels map[string]string) []monitoringv1.RelabelingRule {
	var rules []monitoringv1.RelabelingRule
	seenTargets := make(map[string]bool)
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		v := labels[k]
		target := strutil.SanitizeLabelName(k)
		if protectedLabels[target] {
			target = "exported_" + target
			logger.Warn("Service targetLabel matches protected label. Renamed target.",
				slog.String("label", k),
				slog.String("renamed_target", target))
		}
		if seenTargets[target] {
			logger.Warn("Service targetLabel mapping collision. Skipping.",
				slog.String("source_label", k),
				slog.String("target_label", target))
			continue
		}
		seenTargets[target] = true
		rule := monitoringv1.RelabelingRule{
			TargetLabel: target,
			Replacement: v,
			Action:      string(relabel.Replace),
		}
		rules = append(rules, rule)
		logger.Info("Service label mapped statically to metricRelabeling. Note: Changes to the Service label will not be dynamically reflected on metrics unless this configuration is redeployed with the respective changes.",
			slog.String("label", fmt.Sprintf("%s: %s", k, v)))
	}
	return rules
}
