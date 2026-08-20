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
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/google/export"
	"github.com/prometheus/prometheus/model/relabel"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	labelContainer              = "container"
	labelNode                   = "node"
	labelPod                    = "pod"
	labelTopLevelController     = "top_level_controller"
	labelTopLevelControllerName = "top_level_controller_name"
	labelTopLevelControllerType = "top_level_controller_type"
	labelAddress                = "__address__"
)

const (
	AnnotationTodoPrefix = "gmp.googleapis.com/todo-"
)

// Constants representing the supported ScrapeProtocol enum values defined in upstream Prometheus Operator.
const (
	scrapeProtocolOpenMetricsText100 = pomonitoringv1.ScrapeProtocol("OpenMetricsText1.0.0")
	scrapeProtocolPrometheusProto    = pomonitoringv1.ScrapeProtocol("PrometheusProto")
)

var (
	// protectedLabels contains the list of labels that are protected by GMP and cannot
	// be overwritten by targetLabels or relabeling rules.
	protectedLabels = map[string]bool{
		export.KeyProjectID:         true,
		export.KeyLocation:          true,
		export.KeyCluster:           true,
		export.KeyNamespace:         true,
		export.KeyJob:               true,
		export.KeyInstance:          true,
		labelTopLevelController:     true,
		labelTopLevelControllerName: true,
		labelTopLevelControllerType: true,
		labelAddress:                true,
	}

	// metadataLabelMap contains the list of PO metadata labels and their GMP equivalent.
	metadataLabelMap = map[string]string{
		"__meta_kubernetes_pod_name":            labelPod,
		"__meta_kubernetes_pod_container_name":  labelContainer,
		"__meta_kubernetes_pod_node_name":       labelNode,
		"__meta_kubernetes_node_name":           labelNode,
		"__meta_kubernetes_namespace":           export.KeyNamespace,
		"__meta_kubernetes_pod_controller_name": labelTopLevelControllerName,
		"__meta_kubernetes_pod_controller_kind": labelTopLevelControllerType,
	}
)

type relabelingData struct {
	config           pomonitoringv1.RelabelConfig
	action           relabel.Action
	targetLabel      string
	podSources       []string
	metaSources      []string
	rewrittenSources []string
}

// PreScrapeRelabelingResult holds the label mappings and selector rules extracted from pre-scrape relabelings.
type preScrapeRelabelingResult struct {
	FromPod          []monitoringv1.LabelMapping
	Metadata         *[]string
	MatchLabels      map[string]string
	MatchExpressions []metav1.LabelSelectorRequirement
	PromotedRules    []monitoringv1.RelabelingRule
	Todos            []todoItem
}

// ExtractedPreScrapeRules holds all translated rules, separated by where they belong in GMP.
type extractedPreScrapeRules struct {
	// PerEndpoint contains the rules (like promoted metric relabelings) specific to each scrape endpoint.
	PerEndpoint []preScrapeRelabelingResult
	// ResourceCombined contains target labels and selectors merged across all endpoints.
	ResourceCombined preScrapeRelabelingResult
}

// BuildTypeMeta constructs standard TypeMeta for a GMP resource Kind.
func BuildTypeMeta(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: GMPAPIVersion,
		Kind:       kind,
	}
}

// CopyObjectMeta copies Name and Namespace from source to target, and strips labels and annotations.
func CopyObjectMeta(src metav1.ObjectMeta, targetNamespace string, logger *slog.Logger) metav1.ObjectMeta {
	dst := metav1.ObjectMeta{
		Name:      src.Name,
		Namespace: targetNamespace,
	}

	if len(src.Labels) > 0 || len(src.Annotations) > 0 {
		logger.Info("Stripped all metadata labels and annotations. Reconfigure them manually if needed")
	}

	return dst
}

// AddMigrationTodo appends a sequential TODO annotation to the unstructured resource.
func AddMigrationTodo(u *unstructured.Unstructured, category, reason, action string) {
	if u == nil || u.Object == nil {
		return
	}
	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	todoNumber := 1
	for {
		key := fmt.Sprintf("%s%d", AnnotationTodoPrefix, todoNumber)
		if _, exists := annotations[key]; !exists {
			annotations[key] = fmt.Sprintf("[%s] %s ACTION: %s", category, reason, action)
			break
		}
		todoNumber++
	}
	u.SetAnnotations(annotations)
}

// parseAndCleanNamespaces trims whitespace, filters out empty strings, and deduplicates namespaces.
func parseAndCleanNamespaces(namespaces []string) []string {
	unique := make(map[string]bool)
	var cleaned []string
	for _, ns := range namespaces {
		trimmed := strings.TrimSpace(ns)
		if trimmed != "" && !unique[trimmed] {
			unique[trimmed] = true
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// determineNamespaceScoping resolves the target namespaces from a NamespaceSelector.
func determineNamespaceScoping(nsSel pomonitoringv1.NamespaceSelector, defaultNS string) ([]string, bool, error) {
	if nsSel.Any {
		return nil, true, nil
	}
	if len(nsSel.MatchNames) > 0 {
		targetNamespaces := parseAndCleanNamespaces(nsSel.MatchNames)
		if len(targetNamespaces) == 0 {
			return nil, false, errors.New("namespaceSelector.matchNames contains only empty or invalid values")
		}
		return targetNamespaces, false, nil
	}
	return []string{defaultNS}, false, nil
}

// todoItem represents an actionable TODO annotation to attach to generated resources.
type todoItem struct {
	category string
	reason   string
	action   string
}

// commonMonitorSpec holds common fields extracted from Prometheus Operator monitor specs for building GMP resources.
type commonMonitorSpec struct {
	endpoints        []monitoringv1.ScrapeEndpoint
	mergedFromPod    []monitoringv1.LabelMapping
	mergedSelector   metav1.LabelSelector
	metadata         *[]string
	filterRunning    *bool
	limits           *monitoringv1.ScrapeLimits
	generatedSecrets []*unstructured.Unstructured
	todos            []todoItem
}

// conversionContext groups common parameters passed down to conversion helper functions.
type conversionContext struct {
	logger *slog.Logger
	// cache provides access to dependent resources.
	cache *ResourceCache
	// sourceNamespace is the original namespace where inputs (Secrets/ConfigMaps) live in the cache.
	sourceNamespace string
	// targetNamespace is the destination namespace for generated resources.
	targetNamespace string
	// generatedSecrets accumulates created Secrets when migrating ConfigMaps, keyed by Secret name.
	generatedSecrets map[string]*unstructured.Unstructured
	// isClusterScoped indicates if the target resource is cluster-scoped (ClusterPodMonitoring).
	isClusterScoped bool
	todos           []todoItem
}

// getGeneratedSecrets returns the generated secrets accumulated in the context as a slice.
func (c *conversionContext) getGeneratedSecrets() []*unstructured.Unstructured {
	if len(c.generatedSecrets) == 0 {
		return nil
	}
	// Extract and sort names to guarantee deterministic output order.
	names := make([]string, 0, len(c.generatedSecrets))
	for name := range c.generatedSecrets {
		names = append(names, name)
	}
	slices.Sort(names)

	secrets := make([]*unstructured.Unstructured, 0, len(c.generatedSecrets))
	for _, name := range names {
		secrets = append(secrets, c.generatedSecrets[name])
	}
	return secrets
}

// isValidLabelValues checks whether all strings in parts satisfy Kubernetes label value validation.
func isValidLabelValues(parts []string) bool {
	for _, p := range parts {
		if errs := validation.IsValidLabelValue(p); len(errs) > 0 {
			return false
		}
	}
	return true
}

// convertPreScrapeRelabelings evaluates pre-scrape relabelings on a single endpoint and extracts target label and selector rules.
func convertPreScrapeRelabelings(logger *slog.Logger, configs []pomonitoringv1.RelabelConfig, isSingleEndpoint bool) (preScrapeRelabelingResult, error) {
	var (
		res         preScrapeRelabelingResult
		rawMetadata []string
	)

	for _, config := range configs {
		action := relabel.Action(strings.ToLower(config.Action))
		if action == "" {
			action = relabel.Replace
		}

		if skip, scopeExpanded, details := shouldSkipRelabelConfig(logger, config, action); skip {
			if scopeExpanded {
				logger.Warn("Scraping scope expanded: targets previously excluded by this rule will now be scraped. Adjust pod selectors to compensate.")
				res.Todos = append(res.Todos, todoItem{
					category: "WARNING",
					reason:   fmt.Sprintf("Dropped target filtering rule (%s).", details),
					action:   "Add equivalent pod label selector in 'spec.selector.matchLabels'.",
				})
			}
			continue
		}

		// Pre-scrape labeldrop and labelkeep filter service-discovery meta labels; there is
		// no post-scrape equivalent, so they cannot be promoted.
		if action == relabel.LabelDrop || action == relabel.LabelKeep {
			logger.Warn(fmt.Sprintf("Pre-scrape relabeling rule uses 'action: %s', which filters service discovery labels and has no GMP equivalent. The rule has been dropped.", action))
			continue
		}

		// Pre-scrape hashmod is used for target sharding, which GMP automatically handles.
		if action == relabel.HashMod {
			logger.Warn("Pre-scrape relabeling rule uses 'action: hashmod'. GMP automatically handles target sharding; this rule has been dropped.")
			continue
		}

		// Resolve all source labels upfront and intercept unsupported internal discovery labels.
		podSources, metaSources, rewrittenSources, unsupported := resolveSourceLabels(config.SourceLabels)
		if unsupported {
			logger.Warn(fmt.Sprintf("Relabeling rule (action %q on %v) dropped due to unsupported internal labels (e.g. annotations).", action, config.SourceLabels))
			if action == relabel.Keep || action == relabel.Drop {
				logger.Warn("Scraping scope expanded: targets previously excluded by this rule will now be scraped. Adjust pod selectors to compensate.")
			}
			continue
		}

		data := &relabelingData{
			config:           config,
			action:           action,
			targetLabel:      config.TargetLabel,
			podSources:       podSources,
			metaSources:      metaSources,
			rewrittenSources: rewrittenSources,
		}

		// Convert target filtering ("keep" and "drop") rules on pod labels to Kubernetes label selectors.
		if converted, err := convertRelabelingToSelector(logger, data, isSingleEndpoint, &res); err != nil {
			return preScrapeRelabelingResult{}, err
		} else if converted {
			continue
		}

		// Convert simple label copy rules to targetLabels (fromPod or metadata).
		if convertRelabelingToSimpleCopy(logger, data, &res, &rawMetadata) {
			continue
		}

		// Convert complex or value-changing rules to post-scrape metricRelabeling.
		convertRelabelingToMetricRelabeling(logger, data, &res, &rawMetadata)
	}

	if len(rawMetadata) > 0 {
		res.Metadata = &rawMetadata
	}
	return res, nil
}

// extractPreScrapeRelabelings evaluates pre-scrape rules once per endpoint, returning consolidated endpoint and resource-level results.
func extractPreScrapeRelabelings(logger *slog.Logger, endpointsRelabelConfigs [][]pomonitoringv1.RelabelConfig) (extractedPreScrapeRules, error) {
	var (
		epResults   []preScrapeRelabelingResult
		combined    preScrapeRelabelingResult
		rawMetadata []string
	)
	isSingleEndpoint := len(endpointsRelabelConfigs) == 1

	for _, relabelConfigs := range endpointsRelabelConfigs {
		var r preScrapeRelabelingResult
		if len(relabelConfigs) > 0 {
			var err error
			r, err = convertPreScrapeRelabelings(logger, relabelConfigs, isSingleEndpoint)
			if err != nil {
				return extractedPreScrapeRules{}, err
			}
			combined.FromPod = append(combined.FromPod, r.FromPod...)
			if r.Metadata != nil {
				rawMetadata = append(rawMetadata, *r.Metadata...)
			}
			combined.Todos = append(combined.Todos, r.Todos...)
		}
		epResults = append(epResults, r)
	}

	if len(rawMetadata) > 0 {
		slices.Sort(rawMetadata)
		rawMetadata = slices.Compact(rawMetadata)
		combined.Metadata = &rawMetadata
	}

	// Since we only convert selectors for single-endpoint resources,
	// the combined selector is simply the selector from the first (and only) endpoint.
	if isSingleEndpoint && len(epResults) == 1 {
		combined.MatchLabels = epResults[0].MatchLabels
		combined.MatchExpressions = epResults[0].MatchExpressions
	}

	return extractedPreScrapeRules{
		PerEndpoint:      epResults,
		ResourceCombined: combined,
	}, nil
}

// mergeLabelSelector combines base selector requirements with extracted pre-scrape filtering rules.
func (c *conversionContext) mergeLabelSelector(base metav1.LabelSelector, extraLabels map[string]string, extraExprs []metav1.LabelSelectorRequirement) metav1.LabelSelector {
	if len(extraLabels) == 0 && len(extraExprs) == 0 {
		return *base.DeepCopy()
	}
	res := base.DeepCopy()
	if len(extraLabels) > 0 && res.MatchLabels == nil {
		res.MatchLabels = make(map[string]string)
	}
	for k, v := range extraLabels {
		if existing, exists := res.MatchLabels[k]; exists && existing != v {
			c.todos = append(c.todos, todoItem{
				category: "ERROR",
				reason:   fmt.Sprintf("Selector conflict: label %q has conflicting values %q (from base selector) and %q (from relabeling rule).", k, existing, v),
				action:   fmt.Sprintf("Reconcile 'spec.selector.matchLabels' for label %q with the intended target pods.", k),
			})
			continue
		}
		res.MatchLabels[k] = v
	}
	res.MatchExpressions = append(res.MatchExpressions, extraExprs...)
	return *res
}

// mergeFromPod merges target label mappings and deduplicates by target label name.
func mergeFromPod(logger *slog.Logger, base []monitoringv1.LabelMapping, extra []monitoringv1.LabelMapping) []monitoringv1.LabelMapping {
	if len(extra) == 0 {
		return base
	}
	seenTargets := make(map[string]string)
	var res []monitoringv1.LabelMapping

	// Process custom relabel configs first (higher precedence).
	for _, m := range extra {
		target := m.From
		if m.To != "" {
			target = m.To
		}
		seenTargets[target] = m.From
		res = append(res, m)
	}

	// Process static podTargetLabels second (lower precedence, overridden by extra).
	for _, m := range base {
		target := m.From
		if m.To != "" {
			target = m.To
		}
		if existingFrom, exists := seenTargets[target]; exists {
			if existingFrom == m.From {
				continue
			}
			logger.Info(fmt.Sprintf("Static podTargetLabels mapping for target %q (from %q) is overridden by custom relabel config (from %q).", target, m.From, existingFrom))
			continue
		}
		seenTargets[target] = m.From
		res = append(res, m)
	}
	return res
}

// extractResourceKey is a consolidated helper that fetches a key from a ConfigMap or Secret.
func (c *conversionContext) extractResourceKey(kind, name, key string) string {
	kindUpper := strings.ToUpper(kind)
	if name == "" && key == "" {
		return ""
	}
	if name == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced %s has an empty name for key %q.", kindUpper, key),
			action:   fmt.Sprintf("Specify a valid %s name in the configuration.", kindUpper),
		})
		return fmt.Sprintf("TODO_SET_%s_FROM_%s_EMPTY_NAME", strings.ToUpper(key), kindUpper)
	}
	if key == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced %s %q has an empty key.", kindUpper, name),
			action:   fmt.Sprintf("Specify a valid key in %s %q.", kindUpper, name),
		})
		return fmt.Sprintf("TODO_SET_EMPTY_KEY_FROM_%s_%s", kindUpper, strings.ToUpper(name))
	}

	obj, ok := c.cache.Get(kind, c.sourceNamespace, name)
	if !ok {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced %s %q for key %q was not found in migration inputs.", kindUpper, name, key),
			action:   fmt.Sprintf("Verify %s %q exists in namespace %q or provide it in migration inputs.", kindUpper, name, c.sourceNamespace),
		})
		return fmt.Sprintf("TODO_SET_%s_FROM_%s_%s", strings.ToUpper(key), kindUpper, strings.ToUpper(name))
	}

	// Secrets support unencoded stringData.
	if kind == KindSecret {
		val, found, _ := unstructured.NestedString(obj.Object, "stringData", key)
		if found {
			return val
		}
	}

	// Check standard data field (plain string for ConfigMap, base64 for Secret).
	val, found, _ := unstructured.NestedString(obj.Object, "data", key)
	if found {
		if kind == KindSecret {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
			if err != nil {
				c.todos = append(c.todos, todoItem{
					category: "ERROR",
					reason:   fmt.Sprintf("Failed to base64-decode key %q in Secret %q.", key, name),
					action:   fmt.Sprintf("Ensure Secret %q contains valid base64 data (or use stringData).", name),
				})
				return fmt.Sprintf("TODO_CORRUPT_SECRET_DATA_%s", strings.ToUpper(key))
			}
			return string(decoded)
		}
		return val
	}

	// ConfigMaps can store base64 binaryData.
	if kind == KindConfigMap {
		val, found, _ = unstructured.NestedString(obj.Object, "binaryData", key)
		if found {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
			if err != nil {
				c.todos = append(c.todos, todoItem{
					category: "ERROR",
					reason:   fmt.Sprintf("Failed to base64-decode key %q in ConfigMap %q.", key, name),
					action:   fmt.Sprintf("Ensure ConfigMap %q contains valid base64 binaryData.", name),
				})
				return fmt.Sprintf("TODO_CORRUPT_CONFIGMAP_DATA_%s", strings.ToUpper(key))
			}
			return string(decoded)
		}
	}

	c.todos = append(c.todos, todoItem{
		category: "ERROR",
		reason:   fmt.Sprintf("Key %q was not found in referenced %s %q.", key, kindUpper, name),
		action:   fmt.Sprintf("Add the %q key to %s %q.", key, kindUpper, name),
	})
	return fmt.Sprintf("TODO_MISSING_KEY_%s_IN_%s_%s", strings.ToUpper(key), kindUpper, strings.ToUpper(name))
}

// extractSecretKey extracts a string value from a Secret.
func (c *conversionContext) extractSecretKey(sel corev1.SecretKeySelector) string {
	if sel.Name == "" && sel.Key == "" {
		return ""
	}
	return c.extractResourceKey(KindSecret, sel.Name, sel.Key)
}

// extractConfigMapKey extracts a string value from a ConfigMap.
func (c *conversionContext) extractConfigMapKey(sel corev1.ConfigMapKeySelector) string {
	if sel.Name == "" && sel.Key == "" {
		return ""
	}
	return c.extractResourceKey(KindConfigMap, sel.Name, sel.Key)
}

// convertConfigMapToSecretSelector translates a ConfigMapKeySelector to a SecretSelector.
func (c *conversionContext) convertConfigMapToSecretSelector(sel *corev1.ConfigMapKeySelector) *monitoringv1.SecretSelector {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil
	}
	name := sel.Name
	key := sel.Key
	if name == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced ConfigMap has an empty name for key %q.", key),
			action:   "Specify a valid ConfigMap name in the configuration.",
		})
		name = "TODO_SET_CONFIGMAP_NAME"
	}
	if key == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced ConfigMap %q has an empty key.", name),
			action:   fmt.Sprintf("Specify a valid key in ConfigMap %q.", name),
		})
		key = "TODO_SET_CONFIGMAP_KEY"
	}

	secretName := "secret-" + name
	secretKey := key

	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn("ConfigMap reference had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.",
			slog.String("configmap", sel.Name))
	}

	if c.generatedSecrets == nil {
		c.generatedSecrets = make(map[string]*unstructured.Unstructured)
	}

	if _, exists := c.generatedSecrets[secretName]; !exists {
		obj, ok := c.cache.Get(KindConfigMap, c.sourceNamespace, sel.Name)
		if !ok {
			c.logger.Warn("TLS ConfigMap reference was not found in the inputs. Updated reference to GMP Secret, but you must manually convert your ConfigMap to a Secret with this name in GMP.",
				slog.String("configmap", sel.Name),
				slog.String("expected_secret", secretName))
		} else {
			c.logger.Info("Translated TLS ConfigMap reference to GMP Secret. Generated new Secret manifest.",
				slog.String("configmap", sel.Name),
				slog.String("generated_secret", secretName))

			newSecret := &unstructured.Unstructured{}
			newSecret.SetAPIVersion("v1")
			newSecret.SetKind(KindSecret)
			newSecret.SetName(secretName)
			newSecret.SetNamespace(c.targetNamespace)

			data, found, _ := unstructured.NestedMap(obj.Object, "data")
			if found {
				_ = unstructured.SetNestedMap(newSecret.Object, data, "stringData")
			}
			binaryData, found, _ := unstructured.NestedMap(obj.Object, "binaryData")
			if found {
				_ = unstructured.SetNestedMap(newSecret.Object, binaryData, "data")
			}
			c.generatedSecrets[secretName] = newSecret
		}
	}

	var ns string
	if c.isClusterScoped {
		ns = c.targetNamespace
	}
	secretRef := &monitoringv1.SecretKeySelector{Name: secretName, Key: secretKey, Namespace: ns}
	return &monitoringv1.SecretSelector{Secret: secretRef}
}

// convertSecretOrConfigMapToSecretSelector translates a SecretOrConfigMap to a SecretSelector.
func (c *conversionContext) convertSecretOrConfigMapToSecretSelector(sel pomonitoringv1.SecretOrConfigMap) *monitoringv1.SecretSelector {
	if sel.Secret != nil {
		return c.convertSecretSelector(sel.Secret)
	}

	if sel.ConfigMap != nil {
		return c.convertConfigMapToSecretSelector(sel.ConfigMap)
	}

	return nil
}

// convertSecretSelector translates a SecretKeySelector to a SecretSelector.
func (c *conversionContext) convertSecretSelector(sel *corev1.SecretKeySelector) *monitoringv1.SecretSelector {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil
	}
	name := sel.Name
	key := sel.Key
	if name == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced Secret has an empty name for key %q.", key),
			action:   "Specify a valid Secret name in the configuration.",
		})
		name = "TODO_SET_SECRET_NAME"
	}
	if key == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Referenced Secret %q has an empty key.", name),
			action:   fmt.Sprintf("Specify a valid key in Secret %q.", name),
		})
		key = "TODO_SET_SECRET_KEY"
	}
	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn("Secret reference had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.",
			slog.String("secret", name))
	}
	var ns string
	if c.isClusterScoped {
		ns = c.targetNamespace
	}
	secretRef := &monitoringv1.SecretKeySelector{Name: name, Key: key, Namespace: ns}
	return &monitoringv1.SecretSelector{Secret: secretRef}
}

// convertBasicAuth maps PO BasicAuth to GMP BasicAuth, extracting the username string.
func (c *conversionContext) convertBasicAuth(ba *pomonitoringv1.BasicAuth) *monitoringv1.BasicAuth {
	if ba == nil {
		return nil
	}
	username := c.extractSecretKey(ba.Username)
	password := c.convertSecretSelector(&ba.Password)
	return &monitoringv1.BasicAuth{
		Username: username,
		Password: password,
	}
}

// convertSafeTLSConfig maps PO SafeTLSConfig to GMP TLS, wrapping ConfigMaps into Secrets.
func (c *conversionContext) convertSafeTLSConfig(tls *pomonitoringv1.SafeTLSConfig) *monitoringv1.TLS {
	if tls == nil {
		return nil
	}
	gmpTLS := &monitoringv1.TLS{}
	if tls.InsecureSkipVerify != nil {
		gmpTLS.InsecureSkipVerify = *tls.InsecureSkipVerify
	}
	if tls.ServerName != nil {
		gmpTLS.ServerName = *tls.ServerName
	}
	if tls.CA.Secret != nil || tls.CA.ConfigMap != nil {
		gmpTLS.CA = c.convertSecretOrConfigMapToSecretSelector(tls.CA)
	}
	if tls.Cert.Secret != nil || tls.Cert.ConfigMap != nil {
		gmpTLS.Cert = c.convertSecretOrConfigMapToSecretSelector(tls.Cert)
	}
	if tls.KeySecret != nil {
		gmpTLS.Key = c.convertSecretSelector(tls.KeySecret)
	}
	return gmpTLS
}

// convertOAuth2 maps PO OAuth2 to GMP OAuth2, extracting the clientID string.
func (c *conversionContext) convertOAuth2(oa *pomonitoringv1.OAuth2) *monitoringv1.OAuth2 {
	if oa == nil {
		return nil
	}
	clientID := ""
	if oa.ClientID.Secret != nil {
		clientID = c.extractSecretKey(*oa.ClientID.Secret)
	} else if oa.ClientID.ConfigMap != nil {
		clientID = c.extractConfigMapKey(*oa.ClientID.ConfigMap)
	} else {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   "OAuth2 clientID must be defined as either Secret or ConfigMap.",
			action:   "Specify a valid Secret or ConfigMap reference for 'clientID'.",
		})
		clientID = "TODO_SET_OAUTH2_CLIENT_ID"
	}

	clientSecret := c.convertSecretSelector(&oa.ClientSecret)

	tokenURL := oa.TokenURL
	if tokenURL == "" {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   "OAuth2 tokenURL is empty.",
			action:   "Specify a valid token endpoint URL in 'spec.endpoints[].oauth2.tokenUrl'.",
		})
		tokenURL = "TODO_SET_OAUTH2_TOKEN_URL"
	}

	return &monitoringv1.OAuth2{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		TokenURL:       tokenURL,
		Scopes:         oa.Scopes,
		EndpointParams: oa.EndpointParams,
	}
}

// convertAuthorization maps PO SafeAuthorization to GMP Auth.
func (c *conversionContext) convertAuthorization(auth *pomonitoringv1.SafeAuthorization) *monitoringv1.Auth {
	if auth == nil {
		return nil
	}
	var credentials *monitoringv1.SecretSelector
	if auth.Credentials != nil {
		credentials = c.convertSecretSelector(auth.Credentials)
	}
	return &monitoringv1.Auth{
		Type:        auth.Type,
		Credentials: credentials,
	}
}

// applyAuthAndTLS converts credentials and TLS settings for a generic endpoint.
func (c *conversionContext) applyAuthAndTLS(
	gmpEp *monitoringv1.ScrapeEndpoint,
	basicAuth *pomonitoringv1.BasicAuth,
	oAuth2 *pomonitoringv1.OAuth2,
	tlsConfig *pomonitoringv1.SafeTLSConfig,
	authorization *pomonitoringv1.SafeAuthorization,
	bearerTokenSecret corev1.SecretKeySelector,
) {
	if basicAuth != nil {
		gmpEp.BasicAuth = c.convertBasicAuth(basicAuth)
	}
	if oAuth2 != nil {
		gmpEp.OAuth2 = c.convertOAuth2(oAuth2)
	}
	if tlsConfig != nil {
		gmpEp.TLS = c.convertSafeTLSConfig(tlsConfig)
	}
	if authorization != nil {
		gmpEp.Authorization = c.convertAuthorization(authorization)
	}

	// Handle deprecated BearerTokenSecret -> Authorization.
	if bearerTokenSecret.Name != "" || bearerTokenSecret.Key != "" { // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
		if gmpEp.Authorization != nil {
			c.logger.Warn("Endpoint has both 'bearerTokenSecret' and 'authorization' defined. Dropping 'bearerTokenSecret'.")
		} else {
			tokenSecret := bearerTokenSecret // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			gmpEp.Authorization = c.convertAuthorization(&pomonitoringv1.SafeAuthorization{Credentials: &tokenSecret})
		}
	}
}

func convertMetricRelabelings(
	logger *slog.Logger,
	configs []pomonitoringv1.RelabelConfig,
) []monitoringv1.RelabelingRule {
	var rules []monitoringv1.RelabelingRule

	for _, config := range configs {
		action := relabel.Action(strings.ToLower(config.Action))
		if action == "" {
			action = relabel.Replace
		}

		if skip, _, _ := shouldSkipRelabelConfig(logger, config, action); skip {
			continue
		}

		targetLabel := config.TargetLabel
		switch action {
		case relabel.Replace, relabel.HashMod:
			if protectedLabels[config.TargetLabel] {
				targetLabel = "exported_" + config.TargetLabel
				logger.Warn("Relabeling rule attempts to write to protected target label. Renamed target.",
					slog.String("protected_label", config.TargetLabel),
					slog.String("renamed_target", targetLabel))
			}
		}

		rule := monitoringv1.RelabelingRule{
			TargetLabel: targetLabel,
			Regex:       config.Regex,
			Modulus:     config.Modulus,
			Action:      string(action),
		}

		if len(config.SourceLabels) > 0 {
			rule.SourceLabels = make([]string, len(config.SourceLabels))
			for i, sl := range config.SourceLabels {
				rule.SourceLabels[i] = string(sl)
			}
		}

		if config.Separator != nil {
			rule.Separator = *config.Separator
		}
		if config.Replacement != nil {
			rule.Replacement = *config.Replacement
		}

		rules = append(rules, rule)
	}

	return rules
}

func convertTargetLabels(logger *slog.Logger, sourceLabels []string, jobLabel string, labelKind string) []monitoringv1.LabelMapping {
	var fromPod []monitoringv1.LabelMapping
	seenTargets := make(map[string]bool)

	for _, l := range sourceLabels {
		target := l
		if protectedLabels[l] {
			target = "exported_" + l
		}

		if seenTargets[target] {
			logger.Warn("Target label mapping collision. Skipping.",
				slog.String("label_kind", labelKind),
				slog.String("source_label", l),
				slog.String("target_label", target))
			continue
		}

		seenTargets[target] = true
		mapping := monitoringv1.LabelMapping{From: l}

		if target != l {
			mapping.To = target
			logger.Warn("Target label is protected in GMP. Renamed target.",
				slog.String("label_kind", labelKind),
				slog.String("source_label", l),
				slog.String("renamed_target", target))
		}

		fromPod = append(fromPod, mapping)
	}

	if jobLabel != "" {
		target := "exported_job"
		if !seenTargets[target] {
			logger.Warn("GMP does not support overriding the protected 'job' label. Value has been copied into the target label 'exported_job'.",
				slog.String("source_label", jobLabel))
			fromPod = append(fromPod, monitoringv1.LabelMapping{
				From: jobLabel,
				To:   target,
			})
			seenTargets[target] = true
		} else {
			logger.Warn("Job label could not be mapped to 'exported_job' because 'exported_job' is already taken.",
				slog.String("source_label", jobLabel))
		}
	}

	return fromPod
}

// shouldSkipRelabelConfig checks if the relabel config uses unsupported actions or references annotations.
func shouldSkipRelabelConfig(logger *slog.Logger, config pomonitoringv1.RelabelConfig, action relabel.Action) (skip bool, scopeExpanded bool, dropDetails string) {
	switch action {
	case relabel.Replace, relabel.HashMod, "":
		if config.TargetLabel == "" {
			logger.Warn(fmt.Sprintf("Relabeling rule uses 'action: %s' with an empty 'targetLabel', which is invalid in Prometheus and has been dropped.", action))
			return true, false, ""
		}
	case relabel.Keep, relabel.Drop, relabel.LabelDrop, relabel.LabelKeep:
		// Supported actions that do not require targetLabel.
	case relabel.LabelMap, relabel.Lowercase, relabel.Uppercase, relabel.KeepEqual, relabel.DropEqual:
		logger.Warn(fmt.Sprintf("Relabeling rule uses 'action: %s' which is not supported by GMP and has been dropped.", action))
		return true, false, ""
	default:
		logger.Warn(fmt.Sprintf("Relabeling rule uses unknown 'action: %s' which is not supported by GMP and has been dropped.", action))
		return true, false, ""
	}

	for _, sl := range config.SourceLabels {
		s := string(sl)
		if strings.HasPrefix(s, "__meta_kubernetes_pod_annotation_") {
			logger.Warn(fmt.Sprintf("Relabeling rule referencing pod annotation %q is unsupported in GMP. The rule has been dropped.", s))
			if action == relabel.Keep || action == relabel.Drop {
				return true, true, fmt.Sprintf("'%s' on '%s'", action, s)
			}
			return true, false, ""
		}
		if strings.HasPrefix(s, "__meta_kubernetes_node_") && s != "__meta_kubernetes_node_name" {
			logger.Warn(fmt.Sprintf("Relabeling rule referencing node metadata %q is unsupported in GMP (only node name is supported). The rule has been dropped.", s))
			if action == relabel.Keep || action == relabel.Drop {
				return true, true, fmt.Sprintf("'%s' on '%s'", action, s)
			}
			return true, false, ""
		}
	}
	return false, false, ""
}

// resolveSourceLabels resolves source labels to pod labels, metadata labels, and rewritten labels.
// Returns unsupported=true if it encounters an unsupported internal label.
func resolveSourceLabels(sourceLabels []pomonitoringv1.LabelName) (podSources []string, metaSources []string, rewrittenSources []string, unsupported bool) {
	for _, sl := range sourceLabels {
		s := string(sl)
		if labelName, found := strings.CutPrefix(s, "__meta_kubernetes_pod_label_"); found {
			podSources = append(podSources, labelName)
			if protectedLabels[labelName] {
				labelName = "exported_" + labelName
			}
			rewrittenSources = append(rewrittenSources, labelName)
			continue
		}
		if gmpMeta, ok := metadataLabelMap[s]; ok {
			metaSources = append(metaSources, gmpMeta)
			rewrittenSources = append(rewrittenSources, gmpMeta)
			continue
		}
		// TODO(kunnikrishnan): Support __meta_kubernetes_pod_labelpresent_<labelname> in the future.
		if strings.HasPrefix(s, "__") {
			return nil, nil, nil, true
		}
		rewrittenSources = append(rewrittenSources, s)
	}
	return podSources, metaSources, rewrittenSources, false
}

// convertRelabelingToSelector attempts to convert target filtering (keep/drop) rules to pod selectors.
func convertRelabelingToSelector(logger *slog.Logger, data *relabelingData, isSingleEndpoint bool, res *preScrapeRelabelingResult) (bool, error) {
	if !isSingleEndpoint || data.action != relabel.Keep || len(data.podSources) != 1 || len(data.config.SourceLabels) != 1 {
		return false, nil
	}
	source := string(data.config.SourceLabels[0])
	labelName := data.podSources[0]

	if errs := validation.IsQualifiedName(labelName); len(errs) > 0 {
		return false, nil
	}

	clean := strings.TrimPrefix(strings.TrimSuffix(data.config.Regex, "$"), "^")
	if strings.HasPrefix(clean, "(") && strings.HasSuffix(clean, ")") {
		clean = clean[1 : len(clean)-1]
	}
	parts := strings.Split(clean, "|")
	if strings.ContainsAny(clean, "*+?[]{}()\\^$.") || slices.Contains(parts, "") || !isValidLabelValues(parts) {
		return false, nil
	}

	if len(parts) == 1 {
		if res.MatchLabels == nil {
			res.MatchLabels = make(map[string]string)
		}
		if existing, exists := res.MatchLabels[labelName]; exists && existing != parts[0] {
			res.Todos = append(res.Todos, todoItem{
				category: "ERROR",
				reason:   fmt.Sprintf("Conflicting relabeling keep rules for label %q: cannot require both %q and %q simultaneously.", labelName, existing, parts[0]),
				action:   fmt.Sprintf("Define the intended label value for %q in 'spec.selector.matchLabels'.", labelName),
			})
			return true, nil
		}
		res.MatchLabels[labelName] = parts[0]
		logger.Info(fmt.Sprintf("Converted target filtering relabeling rule (%q -> %q) to Pod Selector (matchLabels).", source, parts[0]))
		return true, nil
	}

	res.MatchExpressions = append(res.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      labelName,
		Operator: metav1.LabelSelectorOpIn,
		Values:   parts,
	})
	logger.Info(fmt.Sprintf("Converted target filtering relabeling rule (%q -> In) to Pod Selector (matchExpressions).", source))
	return true, nil
}

// convertRelabelingToSimpleCopy attempts to convert simple label copy rules to targetLabels (fromPod or metadata).
func convertRelabelingToSimpleCopy(logger *slog.Logger, data *relabelingData, res *preScrapeRelabelingResult, rawMetadata *[]string) bool {
	isDefaultRegex := data.config.Regex == "" ||
		data.config.Regex == "(.*)" ||
		data.config.Regex == "^(.*)$" ||
		data.config.Regex == "^.*$"
	isSimpleCopy := len(data.config.SourceLabels) == 1 &&
		isDefaultRegex &&
		(data.config.Replacement == nil || *data.config.Replacement == "$1") &&
		data.action == relabel.Replace

	if !isSimpleCopy {
		return false
	}

	target := data.targetLabel

	if len(data.podSources) == 1 {
		source := data.podSources[0]

		if protectedLabels[target] {
			oldTarget := target
			target = "exported_" + oldTarget
			logger.Warn(fmt.Sprintf("Relabeling rule attempts to write to protected target label %q. Renamed target to %q.", oldTarget, target))
		}

		mapping := monitoringv1.LabelMapping{From: source}
		if target != source {
			mapping.To = target
		}
		res.FromPod = append(res.FromPod, mapping)
		logger.Info(fmt.Sprintf("Converted simple label copy relabeling rule (%q -> %q) to 'targetLabels.fromPod'.", source, target))
		return true
	}

	if len(data.metaSources) == 1 && target == data.metaSources[0] {
		source := string(data.config.SourceLabels[0])
		*rawMetadata = append(*rawMetadata, data.metaSources[0])
		logger.Info(fmt.Sprintf("Converted metadata label copy (%q) to 'targetLabels.metadata' (as label: %q).", source, data.metaSources[0]))
		return true
	}

	return false
}

// convertRelabelingToMetricRelabeling converts a rule to post-scrape metricRelabeling.
func convertRelabelingToMetricRelabeling(logger *slog.Logger, data *relabelingData, res *preScrapeRelabelingResult, rawMetadata *[]string) {
	if data.action == relabel.Keep || data.action == relabel.Drop {
		logger.Warn(fmt.Sprintf("Target filtering rule (action %q on %v) promoted to post-scrape metricRelabeling; target is still scraped but metrics are dropped. Use labelSelector instead to avoid scraping overhead.", data.action, data.config.SourceLabels))
	}

	for _, p := range data.podSources {
		mapping := monitoringv1.LabelMapping{From: p}
		if protectedLabels[p] {
			mapping.To = "exported_" + p
		}
		res.FromPod = append(res.FromPod, mapping)
		logger.Warn(fmt.Sprintf("Promoted relabeling rule requires pod label %q, which is added to 'targetLabels.fromPod'. This label will be attached to all metrics scraped by this resource across all endpoints.", p))
	}
	for _, m := range data.metaSources {
		*rawMetadata = append(*rawMetadata, m)
		logger.Warn(fmt.Sprintf("Promoted relabeling rule requires metadata label %q, which is added to 'targetLabels.metadata'. This label will be attached to all metrics scraped by this resource across all endpoints.", m))
	}

	targetLabel := data.targetLabel
	if protectedLabels[targetLabel] {
		oldTarget := targetLabel
		targetLabel = "exported_" + oldTarget
		logger.Warn(fmt.Sprintf("Relabeling rule attempts to write to protected target label %q. Renamed target to %q.", oldTarget, targetLabel))
	}

	promoted := monitoringv1.RelabelingRule{
		SourceLabels: data.rewrittenSources,
		TargetLabel:  targetLabel,
		Regex:        data.config.Regex,
		Modulus:      data.config.Modulus,
		Action:       string(data.action),
	}
	if data.config.Separator != nil {
		promoted.Separator = *data.config.Separator
	}
	if data.config.Replacement != nil {
		promoted.Replacement = *data.config.Replacement
	}
	res.PromotedRules = append(res.PromotedRules, promoted)
	logger.Info(fmt.Sprintf("Complex relabeling rule (target: %q) promoted from pre-scrape 'relabelings' to post-scrape 'metricRelabeling'.", data.targetLabel))
}

// warnUnsupportedMonitorSpecFields logs warnings for spec-level fields that GMP does not support or need.
// TODO: Once Prometheus Operator Go structs are upgraded, add warning checks for unsupported native histogram fields ('scrapeNativeHistograms', 'scrapeClassicHistograms', 'nativeHistogramBucketLimit', 'nativeHistogramMinBucketFactor', 'fallbackScrapeProtocols').
func warnUnsupportedMonitorSpecFields(logger *slog.Logger, targetLimit *uint64, keepDroppedTargets *uint64, bodySizeLimit *pomonitoringv1.ByteSize) {
	if targetLimit != nil {
		logger.Warn("Field 'targetLimit' is unnecessary in GMP Managed Collection and has been dropped. Target discovery and scaling are managed automatically by GKE.")
	}
	if keepDroppedTargets != nil {
		logger.Warn("Field 'keepDroppedTargets' is unnecessary in GMP Managed Collection and has been dropped.")
	}
	if bodySizeLimit != nil {
		logger.Warn("Field 'bodySizeLimit' is unsupported by GMP Managed Collection and has been dropped. Scrape response buffer limits are managed automatically by GMP.")
	}
}

// convertLimits maps PodMonitor limit settings to GMP ScrapeLimits.
func convertLimits(sampleLimit, labelLimit, labelNameLengthLimit, labelValueLengthLimit *uint64) *monitoringv1.ScrapeLimits {
	if sampleLimit == nil && labelLimit == nil && labelNameLengthLimit == nil && labelValueLengthLimit == nil {
		return nil
	}
	limits := &monitoringv1.ScrapeLimits{}
	if sampleLimit != nil {
		limits.Samples = *sampleLimit
	}
	if labelLimit != nil {
		limits.Labels = *labelLimit
	}
	if labelNameLengthLimit != nil {
		limits.LabelNameLength = *labelNameLengthLimit
	}
	if labelValueLengthLimit != nil {
		limits.LabelValueLength = *labelValueLengthLimit
	}
	// Return nil if all fields in limits remain zero since zero values are stripped by omitempty.
	if *limits == (monitoringv1.ScrapeLimits{}) {
		return nil
	}
	return limits
}

// toStrictUnstructured converts a struct to an unstructured map and normalizes uint64 fields to int64.
// This prevents unstructured.DeepCopy from panicking without losing integer precision.
func toStrictUnstructured(obj any) (map[string]any, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return sanitizeUInt64(u).(map[string]any), nil
}

// sanitizeUInt64 recursively converts uint64 primitives to int64 for unstructured compatibility.
func sanitizeUInt64(val any) any {
	switch v := val.(type) {
	case uint64:
		return int64(v)
	case map[string]any:
		for k, child := range v {
			v[k] = sanitizeUInt64(child)
		}
	case []any:
		for i, child := range v {
			v[i] = sanitizeUInt64(child)
		}
	}
	return val
}

// buildPodMonitoring constructs a GMP PodMonitoring resource from common spec.
func buildPodMonitoring(
	srcMeta metav1.ObjectMeta,
	targetNamespace string,
	spec *commonMonitorSpec,
	logger *slog.Logger,
) (*unstructured.Unstructured, error) {
	if spec == nil {
		return nil, errors.New("spec cannot be nil")
	}
	gmpPM := &monitoringv1.PodMonitoring{
		TypeMeta:   BuildTypeMeta(KindPodMonitoring),
		ObjectMeta: CopyObjectMeta(srcMeta, targetNamespace, logger),
		Spec: monitoringv1.PodMonitoringSpec{
			Selector:  spec.mergedSelector,
			Endpoints: spec.endpoints,
			TargetLabels: monitoringv1.TargetLabels{
				FromPod:  spec.mergedFromPod,
				Metadata: spec.metadata,
			},
			Limits:        spec.limits,
			FilterRunning: spec.filterRunning,
		},
	}

	unstructuredMap, err := toStrictUnstructured(gmpPM)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PodMonitoring: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindPodMonitoring)

	for _, td := range spec.todos {
		AddMigrationTodo(u, td.category, td.reason, td.action)
		if logger != nil {
			logger.Warn(td.reason, slog.String("action", td.action), slog.String("migration_status", "action_items"))
		}
	}

	return u, nil
}

// buildClusterPodMonitoring constructs a GMP ClusterPodMonitoring resource from common spec.
func buildClusterPodMonitoring(
	srcMeta metav1.ObjectMeta,
	spec *commonMonitorSpec,
	logger *slog.Logger,
) (*unstructured.Unstructured, error) {
	if spec == nil {
		return nil, errors.New("spec cannot be nil")
	}

	gmpCPM := &monitoringv1.ClusterPodMonitoring{
		TypeMeta:   BuildTypeMeta(KindClusterPodMonitoring),
		ObjectMeta: CopyObjectMeta(srcMeta, "", logger),
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector:  spec.mergedSelector,
			Endpoints: spec.endpoints,
			TargetLabels: monitoringv1.ClusterTargetLabels{
				FromPod:  spec.mergedFromPod,
				Metadata: spec.metadata,
			},
			Limits:        spec.limits,
			FilterRunning: spec.filterRunning,
		},
	}

	unstructuredMap, err := toStrictUnstructured(gmpCPM)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ClusterPodMonitoring: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindClusterPodMonitoring)

	for _, td := range spec.todos {
		AddMigrationTodo(u, td.category, td.reason, td.action)
		if logger != nil {
			logger.Warn(td.reason, slog.String("action", td.action), slog.String("migration_status", "action_items"))
		}
	}

	return u, nil
}

// resolveScrapeClass handles ScrapeClass resolution.
// Logs a warning that ScrapeClass settings will be lost.
// TODO(M2): Lookup ScrapeClass from Prometheus CR and merge its settings.
func resolveScrapeClass(name *string, logger *slog.Logger) {
	if name != nil && *name != "" {
		logger.Warn(fmt.Sprintf("ScrapeClass %q was not found in the inputs. The 'scrapeClassName' field has been dropped and inherited settings will be lost.", *name))
	}
}

// validateScrapeProtocols logs a warning if any scrape protocol requires Protobuf.
func validateScrapeProtocols(protocols []pomonitoringv1.ScrapeProtocol, logger *slog.Logger) {
	for _, sp := range protocols {
		if sp == scrapeProtocolPrometheusProto || strings.Contains(strings.ToLower(string(sp)), "proto") {
			logger.Warn("Scrape protocol settings (scrapeProtocols) requiring Protobuf are unsupported. Scrapes may fail if target lacks text fallback.")
			break
		}
	}
}

// unionMetadata merges two slices of metadata labels, removing duplicates and returning a sorted slice.
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

// resolveMetadata applies default metadata unioning, namespace label stripping, and attachMetadata node resolution.
func resolveMetadata(baseMetadata *[]string, attachMetadata *pomonitoringv1.AttachMetadata, isCluster bool, logger *slog.Logger) *[]string {
	filtered := filterMetadata(baseMetadata, isCluster, logger)
	return resolveAttachMetadata(attachMetadata, filtered, isCluster)
}

// filterMetadata applies namespaced or cluster metadata defaults and strips namespace metadata in namespaced resources.
func filterMetadata(metadata *[]string, isCluster bool, logger *slog.Logger) *[]string {
	if metadata == nil {
		return nil
	}
	if isCluster {
		union := unionMetadata(*metadata, clusterMetadataDefaults)
		return &union
	}
	union := unionMetadata(*metadata, namespacedMetadataDefaults)
	var md []string
	for _, m := range union {
		if m != export.KeyNamespace {
			md = append(md, m)
		} else {
			logger.Warn("Relabeling rule referencing namespace metadata is unsupported in namespaced PodMonitoring (it is only allowed in ClusterPodMonitoring). The metadata entry has been omitted.")
		}
	}
	if len(md) > 0 {
		return &md
	}
	return nil
}

// resolveAttachMetadata appends "node" to metadata if attachMetadata.node is enabled.
func resolveAttachMetadata(attachMetadata *pomonitoringv1.AttachMetadata, baseMetadata *[]string, isCluster bool) *[]string {
	if attachMetadata != nil && attachMetadata.Node != nil && *attachMetadata.Node {
		if baseMetadata == nil {
			defaults := namespacedMetadataDefaults
			if isCluster {
				defaults = clusterMetadataDefaults
			}
			union := unionMetadata([]string{labelNode}, defaults)
			return &union
		}
		union := unionMetadata([]string{labelNode}, *baseMetadata)
		return &union
	}
	return baseMetadata
}

// resolveFilterRunning evaluates filterRunning settings across endpoints and resolves them to a single resource-level setting.
// TODO: Potential better optimization would be to split into separate PodMonitorings based on filterRunning value.
func resolveFilterRunning(filterRunnings []*bool, logger *slog.Logger, isCluster bool) *bool {
	var hasFalse, hasTrue bool
	for _, fr := range filterRunnings {
		if fr != nil && !*fr {
			hasFalse = true
		} else {
			hasTrue = true
		}
	}
	if hasFalse {
		falseVal := false
		if hasTrue {
			if isCluster {
				logger.Warn("Endpoint-level configuration conflict detected: some endpoints are configured with 'filterRunning: false' and others with 'true' (or default), but GMP only supports 'filterRunning' at the resource level. Setting 'filterRunning: false' globally on the ClusterPodMonitoring resource.")
			} else {
				logger.Warn("Endpoint-level configuration conflict detected: some endpoints are configured with 'filterRunning: false' and others with 'true' (or default), but GMP only supports 'filterRunning' at the resource level. Setting 'filterRunning: false' globally.")
			}
		}
		return &falseVal
	}
	return nil
}

// resolveScrapeIntervalAndTimeout validates and caps timeout to interval if needed.
func (c *conversionContext) resolveScrapeIntervalAndTimeout(interval, timeout string) (resolvedInterval, resolvedTimeout string) {
	// TODO(M2): Inherit global scrape interval from Prometheus CR if empty.
	if interval == "" {
		c.logger.Warn("Scrape interval is empty. Defaulting to '30s' as GMP requires this field.")
		interval = "30s"
	}

	intDur, err := prommodel.ParseDuration(interval)
	if err != nil {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Invalid scrape interval %q.", interval),
			action:   "Specify a valid Prometheus duration string (e.g. '30s', '1m') in 'spec.endpoints[].interval'.",
		})
		interval = "30s"
		intDur, _ = prommodel.ParseDuration("30s")
	}

	// TODO(M2): Inherit global scrape timeout from Prometheus CR if empty.
	if timeout != "" {
		toDur, err := prommodel.ParseDuration(timeout)
		if err != nil {
			c.todos = append(c.todos, todoItem{
				category: "ERROR",
				reason:   fmt.Sprintf("Invalid scrape timeout %q.", timeout),
				action:   "Specify a valid Prometheus duration string (e.g. '10s') in 'spec.endpoints[].timeout'.",
			})
			timeout = ""
		} else if toDur > intDur {
			c.logger.Warn("Scrape timeout is larger than scrape interval. Capping timeout to interval.",
				slog.String("timeout", timeout),
				slog.String("interval", interval))
			timeout = interval
		}
	}
	return interval, timeout
}

// convertProxyURL verifies proxy URL credentials and attaches a TODO if credentials are present or malformed.
func (c *conversionContext) convertProxyURL(proxyURL *string) string {
	if proxyURL == nil {
		return ""
	}
	parsed, err := url.Parse(*proxyURL)
	if err != nil {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   fmt.Sprintf("Proxy URL %q is invalid or malformed.", *proxyURL),
			action:   "Specify a valid proxy URL (e.g. 'http://proxy.example.com:8080').",
		})
		return "TODO_SET_VALID_PROXY_URL"
	}
	if parsed.User != nil {
		c.todos = append(c.todos, todoItem{
			category: "ERROR",
			reason:   "Proxy URL contains embedded plaintext credentials. Credentials were removed.",
			action:   "Configure proxy authentication via proxy server configuration or network allowlist.",
		})
		parsed.User = nil
		return parsed.String()
	}
	return *proxyURL
}

// warnUnsupportedEndpointFields logs warnings for fields that GMP does not support.
// TODO: Once Prometheus Operator Go structs are upgraded, add warning checks for unsupported endpoint proxy fields ('noProxy', 'proxyConnectHeader', 'proxyFromEnvironment').
func warnUnsupportedEndpointFields(logger *slog.Logger, followRedirects *bool, enableHTTP2 *bool, honorLabels bool, honorTimestamps *bool, trackTimestampsStaleness *bool, i int) {
	if followRedirects != nil && !*followRedirects {
		logger.Warn(fmt.Sprintf("endpoint [%d]: field 'followRedirects: false' is unsupported by GMP Managed Collection and has been dropped. The collector will always follow redirects.", i))
	}
	if enableHTTP2 != nil && !*enableHTTP2 {
		logger.Warn(fmt.Sprintf("endpoint [%d]: field 'enableHttp2: false' is unsupported by GMP Managed Collection and has been dropped. The collector will always negotiate HTTP/2 for TLS connections.", i))
	}
	if honorLabels {
		logger.Warn(fmt.Sprintf("endpoint [%d]: field 'honorLabels: true' is unsupported and dropped. GMP always overrides conflicting labels. Clashing metric labels will be renamed with the 'exported_' prefix.", i))
	}
	if honorTimestamps != nil && *honorTimestamps {
		logger.Warn(fmt.Sprintf("endpoint [%d]: field 'honorTimestamps: true' is unsupported and dropped. GMP always uses the scrape ingestion timestamp. Target metric timestamps will be ignored.", i))
	}
	if trackTimestampsStaleness != nil {
		logger.Warn(fmt.Sprintf("endpoint [%d]: field 'trackTimestampsStaleness' is unsupported in GMP and has been dropped.", i))
	}
}

// combineAndConvertRelabelings combines promoted pre-scrape rules and converts metricRelabelings.
func combineAndConvertRelabelings(logger *slog.Logger, promoted []monitoringv1.RelabelingRule, configs []pomonitoringv1.RelabelConfig) []monitoringv1.RelabelingRule {
	totalRules := len(promoted) + len(configs)
	if totalRules == 0 {
		return nil
	}

	allRules := make([]monitoringv1.RelabelingRule, 0, totalRules)
	allRules = append(allRules, promoted...)

	if len(configs) > 0 {
		rules := convertMetricRelabelings(logger, configs)
		allRules = append(allRules, rules...)
	}

	if len(allRules) == 0 {
		return nil
	}
	return allRules
}

// endpointPortKey returns a string representation of the endpoint's Port or TargetPort for port resolution and map lookups.
func endpointPortKey(ep pomonitoringv1.Endpoint) string {
	if ep.Port != "" {
		return ep.Port
	}
	// nolint:staticcheck // Support deprecated TargetPort fallback for backwards compatibility.
	if ep.TargetPort != nil {
		return ep.TargetPort.String()
	}
	return ""
}

// resolveServicePort resolves a Service port to the backing Pod's target port.
func resolveServicePort(logger *slog.Logger, svc *corev1.Service, portStr string) (intstr.IntOrString, *todoItem) {
	if portStr == "" {
		return intstr.FromString("TODO_SET_PORT"), nil
	}
	if svc == nil {
		return intstr.FromString("TODO_RESOLVE_PORT"), nil
	}
	if len(svc.Spec.Ports) == 0 {
		return intstr.FromString(fmt.Sprintf("TODO_RESOLVE_PORT_%s", strings.ToUpper(portStr))), &todoItem{
			category: "WARNING",
			reason:   fmt.Sprintf("Port %q could not be resolved because Service %q defines no ports in its spec.", portStr, svc.Name),
			action:   fmt.Sprintf("Verify that target pods expose port %q.", portStr),
		}
	}

	for _, p := range svc.Spec.Ports {
		if p.Port < 1 || p.Port > 65535 {
			logger.Warn("Service port entry has an invalid or out-of-range port number. Skipping malformed entry.",
				slog.String("service", svc.Name),
				slog.String("port_name", p.Name),
				slog.Int("port_value", int(p.Port)))
			continue
		}

		if p.Name == portStr || fmt.Sprintf("%d", p.Port) == portStr || p.TargetPort.String() == portStr {
			if p.TargetPort.IntVal == 0 && p.TargetPort.StrVal == "" {
				return intstr.FromInt32(p.Port), nil
			}
			return p.TargetPort, nil
		}
	}

	return intstr.FromString(fmt.Sprintf("TODO_RESOLVE_PORT_%s", strings.ToUpper(portStr))), &todoItem{
		category: "WARNING",
		reason:   fmt.Sprintf("Port %q was not found in Service %q spec.", portStr, svc.Name),
		action:   fmt.Sprintf("Verify that target pods expose port %q.", portStr),
	}
}

const (
	// resourceNameHashLength is the number of hexadecimal characters used for the deterministic suffix hash.
	resourceNameHashLength = 6
	// resourceNameSuffixLength is the total length of the hyphen (1 character) plus the hash appended to truncated resource names.
	resourceNameSuffixLength = 1 + resourceNameHashLength
)

// makeUniqueResourceName joins a base name and suffix with a hyphen.
// If the resulting name exceeds validation.DNS1123LabelMaxLength (63 characters),
// it truncates the base name and appends a 6-character deterministic hash of the combined name.
func makeUniqueResourceName(base, suffix string) string {
	name := fmt.Sprintf("%s-%s", base, suffix)
	if len(name) <= validation.DNS1123LabelMaxLength {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	hashStr := fmt.Sprintf("%08x", h.Sum32())
	hashSuffix := hashStr[:resourceNameHashLength]

	// Prevent out-of-bounds slicing when base is shorter than 56 characters but suffix is very long.
	maxBase := min(len(base), validation.DNS1123LabelMaxLength-resourceNameSuffixLength)
	trimmedBase := strings.TrimRight(base[:maxBase], "-")
	return fmt.Sprintf("%s-%s", trimmedBase, hashSuffix)
}
