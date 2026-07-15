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
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	actionReplace   = "replace"
	actionKeep      = "keep"
	actionDrop      = "drop"
	actionLabelMap  = "labelmap"
	actionLabelKeep = "labelkeep"
	actionLabelDrop = "labeldrop"
	actionHashMod   = "hashmod"
	actionLowercase = "lowercase"
	actionUppercase = "uppercase"

	labelCluster                = "cluster"
	labelLocation               = "location"
	labelProjectID              = "project_id"
	labelNamespace              = "namespace"
	labelJob                    = "job"
	labelInstance               = "instance"
	labelContainer              = "container"
	labelNode                   = "node"
	labelPod                    = "pod"
	labelTopLevelController     = "top_level_controller"
	labelTopLevelControllerName = "top_level_controller_name"
	labelTopLevelControllerType = "top_level_controller_type"
	labelAddress                = "__address__"
)

var (
	// protectedLabels contains the list of labels that are protected by GMP and cannot
	// be overwritten by targetLabels or relabeling rules.
	protectedLabels = map[string]bool{
		labelProjectID:              true,
		labelLocation:               true,
		labelCluster:                true,
		labelNamespace:              true,
		labelJob:                    true,
		labelInstance:               true,
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
		"__meta_kubernetes_namespace":           labelNamespace,
		"__meta_kubernetes_pod_controller_name": labelTopLevelControllerName,
		"__meta_kubernetes_pod_controller_kind": labelTopLevelControllerType,
	}
)

// PreScrapeRelabelingResult holds the label mappings and selector rules extracted from pre-scrape relabelings.
type PreScrapeRelabelingResult struct {
	FromPod          []monitoringv1.LabelMapping
	Metadata         *[]string
	MatchLabels      map[string]string
	MatchExpressions []metav1.LabelSelectorRequirement
	PromotedRules    []monitoringv1.RelabelingRule
}

// ExtractedPreScrapeRules holds all translated rules, separated by where they belong in GMP.
type ExtractedPreScrapeRules struct {
	// PerEndpoint contains the rules (like promoted metric relabelings) specific to each scrape endpoint.
	PerEndpoint []PreScrapeRelabelingResult
	// ResourceCombined contains target labels and selectors merged across all endpoints.
	ResourceCombined PreScrapeRelabelingResult
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
		logger.Warn("Stripped all metadata labels and annotations. Reconfigure them manually if needed")
	}

	return dst
}

// ParseAndCleanNamespaces trims whitespace, filters out empty strings, and deduplicates namespaces.
func ParseAndCleanNamespaces(namespaces []string) []string {
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

// conversionContext groups common parameters passed down to conversion helper functions.
type conversionContext struct {
	logger *slog.Logger
	// cache provides access to dependent resources.
	cache *ResourceCache
	// namespace is the source namespace of the primary resource.
	namespace string
	// generatedSecrets accumulates created Secrets when migrating ConfigMaps, keyed by Secret name.
	generatedSecrets map[string]*unstructured.Unstructured
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
func convertPreScrapeRelabelings(logger *slog.Logger, configs []pomonitoringv1.RelabelConfig, isSingleEndpoint bool) PreScrapeRelabelingResult {
	var res PreScrapeRelabelingResult
	var rawMetadata []string

	for _, config := range configs {
		action := strings.ToLower(config.Action)
		if action == "" {
			action = actionReplace
		}

		switch action {
		case actionLabelMap, actionLabelKeep, actionLabelDrop:
			logger.Warn(fmt.Sprintf("Relabeling rule uses 'action: %s' which is not supported by GMP and has been dropped.", action))
			continue
		}

		// Relabeling rules on annotations cannot be migrated.
		var anno string
		for _, sl := range config.SourceLabels {
			if strings.HasPrefix(string(sl), "__meta_kubernetes_pod_annotation_") {
				anno = string(sl)
				break
			}
		}
		if anno != "" {
			logger.Warn(fmt.Sprintf("Relabeling rule referencing pod annotation %q is unsupported in GMP. The rule has been dropped.", anno))
			continue
		}

		// Change protected labels to exported_<label>.
		targetLabel := config.TargetLabel
		if protectedLabels[targetLabel] {
			oldTarget := targetLabel
			targetLabel = "exported_" + oldTarget
			logger.Warn(fmt.Sprintf("Relabeling rule attempts to write to protected target label %q. Renamed target to %q.", oldTarget, targetLabel))
		}

		// Resolve all source labels upfront and intercept unsupported internal discovery labels.
		var podSources []string
		var metaSources []string
		var rewrittenSources []string
		var unsupportedInternal bool

		for _, sl := range config.SourceLabels {
			s := string(sl)
			if labelName, found := strings.CutPrefix(s, "__meta_kubernetes_pod_label_"); found {
				podSources = append(podSources, labelName)
				rewrittenSources = append(rewrittenSources, labelName)
			} else if gmpMeta, ok := metadataLabelMap[s]; ok {
				metaSources = append(metaSources, gmpMeta)
				rewrittenSources = append(rewrittenSources, gmpMeta)
			} else if strings.HasPrefix(s, "__") {
				logger.Warn(fmt.Sprintf("Relabeling rule references internal label %q which is not available in post-scrape metricRelabeling. The rule cannot be migrated and has been dropped.", s))
				unsupportedInternal = true
				break
			} else {
				rewrittenSources = append(rewrittenSources, s)
			}
		}
		if unsupportedInternal {
			continue
		}

		// Translate target filtering ("keep" and "drop") rules on pod labels to Kubernetes label selectors.
		if isSingleEndpoint && (action == actionKeep || action == actionDrop) && len(podSources) == 1 && len(config.SourceLabels) == 1 {
			source := string(config.SourceLabels[0])
			labelName := podSources[0]
			// Strip optional regex start (^) and end ($) anchors (ex. "^production$" -> "production").
			clean := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(config.Regex), "$"), "^")
			// Strip outer grouping parentheses around literal lists (ex. "(test|staging)" -> "test|staging").
			if strings.HasPrefix(clean, "(") && strings.HasSuffix(clean, ")") {
				clean = clean[1 : len(clean)-1]
			}
			parts := strings.Split(clean, "|")
			// Verify that the remaining string contains no regex metacharacters (*, +, ?, [, ], etc.) and valid K8s label values.
			if !strings.ContainsAny(clean, "*+?[]{}()\\^$.") && !slices.Contains(parts, "") && isValidLabelValues(parts) {
				// A single value with "action: keep" translates to matchLabels.
				if action == actionKeep && len(parts) == 1 {
					if res.MatchLabels == nil {
						res.MatchLabels = make(map[string]string)
					}
					res.MatchLabels[labelName] = parts[0]
					logger.Info(fmt.Sprintf("Translated target filtering relabeling rule (%q -> %q) to Pod Selector (matchLabels).", source, parts[0]))
					continue
				}

				// Multiple values (or any "action: drop" set) translate to matchExpressions (In / NotIn).
				op := metav1.LabelSelectorOpIn
				if action == actionDrop {
					op = metav1.LabelSelectorOpNotIn
				}
				res.MatchExpressions = append(res.MatchExpressions, metav1.LabelSelectorRequirement{
					Key:      labelName,
					Operator: op,
					Values:   parts,
				})
				logger.Info(fmt.Sprintf("Translated target filtering relabeling rule (%q -> %s) to Pod Selector (matchExpressions).", source, op))
				continue
			}
		}

		// Relabeling rule equivalent of simply copying over a label.
		isSimpleCopy := len(config.SourceLabels) == 1 &&
			(config.Regex == "" || config.Regex == "(.*)") &&
			(config.Replacement == nil || *config.Replacement == "$1") &&
			action == actionReplace

		if isSimpleCopy {
			source := string(config.SourceLabels[0])
			target := targetLabel

			// Simple pod target label transfer.
			if len(podSources) == 1 {
				mapping := monitoringv1.LabelMapping{From: podSources[0]}
				if target != podSources[0] {
					mapping.To = target
				}
				res.FromPod = append(res.FromPod, mapping)
				logger.Info(fmt.Sprintf("Translated simple label copy relabeling rule (%q -> %q) to 'targetLabels.fromPod'.", source, target))
				continue
			}

			// Simple metadata label transfer.
			if len(metaSources) == 1 && target == metaSources[0] {
				rawMetadata = append(rawMetadata, metaSources[0])
				logger.Info(fmt.Sprintf("Translated metadata label copy (%q) to 'targetLabels.metadata' (as label: %q).", source, metaSources[0]))
				continue
			}
		}

		// Phase 3: Promote complex or value-changing rules to post-scrape metricRelabeling.
		for _, p := range podSources {
			res.FromPod = append(res.FromPod, monitoringv1.LabelMapping{From: p})
		}
		rawMetadata = append(rawMetadata, metaSources...)

		promoted := monitoringv1.RelabelingRule{
			SourceLabels: rewrittenSources,
			TargetLabel:  targetLabel,
			Regex:        config.Regex,
			Modulus:      config.Modulus,
			Action:       action,
		}
		if config.Separator != nil {
			promoted.Separator = *config.Separator
		}
		if config.Replacement != nil {
			promoted.Replacement = *config.Replacement
		}
		res.PromotedRules = append(res.PromotedRules, promoted)
		logger.Info(fmt.Sprintf("Complex relabeling rule (target: %q) promoted from pre-scrape 'relabelings' to post-scrape 'metricRelabeling'.", targetLabel))
	}

	if len(rawMetadata) > 0 {
		res.Metadata = &rawMetadata
	}
	return res
}

// extractPreScrapeRelabelings evaluates pre-scrape rules once per endpoint, returning consolidated endpoint and resource-level results.
func extractPreScrapeRelabelings(logger *slog.Logger, endpoints []pomonitoringv1.PodMetricsEndpoint) ExtractedPreScrapeRules {
	var epResults []PreScrapeRelabelingResult
	var combined PreScrapeRelabelingResult
	var rawMetadata []string
	for _, ep := range endpoints {
		var r PreScrapeRelabelingResult
		if len(ep.RelabelConfigs) > 0 {
			r = convertPreScrapeRelabelings(logger, ep.RelabelConfigs, len(endpoints) == 1)
			combined.FromPod = append(combined.FromPod, r.FromPod...)
			if r.Metadata != nil {
				rawMetadata = append(rawMetadata, *r.Metadata...)
			}
			if len(r.MatchLabels) > 0 {
				if combined.MatchLabels == nil {
					combined.MatchLabels = make(map[string]string)
				}
				maps.Copy(combined.MatchLabels, r.MatchLabels)
			}
			combined.MatchExpressions = append(combined.MatchExpressions, r.MatchExpressions...)
		}
		epResults = append(epResults, r)
	}

	if len(rawMetadata) > 0 {
		unique := make(map[string]bool)
		var sortedMd []string
		for _, m := range rawMetadata {
			if !unique[m] {
				unique[m] = true
				sortedMd = append(sortedMd, m)
			}
		}
		slices.Sort(sortedMd)
		combined.Metadata = &sortedMd
	}
	return ExtractedPreScrapeRules{
		PerEndpoint:      epResults,
		ResourceCombined: combined,
	}
}

// mergeLabelSelector combines base selector requirements with extracted pre-scrape filtering rules.
func mergeLabelSelector(logger *slog.Logger, base metav1.LabelSelector, extraLabels map[string]string, extraExprs []metav1.LabelSelectorRequirement) metav1.LabelSelector {
	res := base.DeepCopy()
	if len(extraLabels) > 0 && res.MatchLabels == nil {
		res.MatchLabels = make(map[string]string)
	}
	for k, v := range extraLabels {
		if existing, exists := res.MatchLabels[k]; exists && existing != v {
			logger.Warn(fmt.Sprintf("Relabeling rule target filter for label %q (%q) conflicts with existing selector matchLabel (%q). Skipping rule.", k, v, existing))
			continue
		}
		res.MatchLabels[k] = v
	}
	res.MatchExpressions = append(res.MatchExpressions, extraExprs...)
	return *res
}

// mergeFromPod merges target label mappings and deduplicates by target label name.
func mergeFromPod(logger *slog.Logger, base []monitoringv1.LabelMapping, extra []monitoringv1.LabelMapping) []monitoringv1.LabelMapping {
	seenTargets := make(map[string]string)
	var res []monitoringv1.LabelMapping

	for _, m := range base {
		target := m.From
		if m.To != "" {
			target = m.To
		}
		seenTargets[target] = m.From
		res = append(res, m)
	}

	for _, m := range extra {
		target := m.From
		if m.To != "" {
			target = m.To
		}
		if existingFrom, exists := seenTargets[target]; exists {
			if existingFrom == m.From {
				continue
			}
			logger.Warn(fmt.Sprintf("Target label %q is already mapped from %q. Skipping conflicting mapping from %q.", target, existingFrom, m.From))
			continue
		}
		seenTargets[target] = m.From
		res = append(res, m)
	}
	return res
}

// extractResourceKey is a consolidated helper that fetches a key from a ConfigMap or Secret.
// It returns an error if the reference is malformed or if data is corrupt.
// It returns a placeholder and logs a warning if the resource itself is not found in the cache.
func (c *conversionContext) extractResourceKey(kind, name, key string) (string, error) {
	kindUpper := strings.ToUpper(kind)
	if name == "" && key == "" {
		return "", nil
	}
	if name == "" {
		return "", fmt.Errorf("%s reference has an empty name for key %q", kindUpper, key)
	}
	if key == "" {
		return "", fmt.Errorf("%s reference has an empty key for name %q", kindUpper, name)
	}

	obj, ok := c.cache.Get(kind, c.namespace, name)
	if !ok {
		c.logger.Warn("Resource not found in cache. Cannot extract key. Hardcoding placeholder.",
			slog.String("referenced_kind", kind),
			slog.String("referenced_name", name),
			slog.String("key", key))
		return fmt.Sprintf("<MISSING_%s_%s_KEY_%s>", kindUpper, name, key), nil
	}

	// Secrets support unencoded stringData.
	if kind == KindSecret {
		val, found, _ := unstructured.NestedString(obj.Object, "stringData", key)
		if found {
			return val, nil
		}
	}

	// Check standard data field (plain string for ConfigMap, base64 for Secret).
	val, found, _ := unstructured.NestedString(obj.Object, "data", key)
	if found {
		if kind == KindSecret {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
			if err != nil {
				return "", fmt.Errorf("failed to decode base64 data for key %q in secret %q: %w", key, name, err)
			}
			return string(decoded), nil
		}
		return val, nil
	}

	// ConfigMaps can store base64 binaryData.
	if kind == KindConfigMap {
		val, found, _ = unstructured.NestedString(obj.Object, "binaryData", key)
		if found {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
			if err != nil {
				return "", fmt.Errorf("failed to decode base64 binaryData for key %q in configmap %q: %w", key, name, err)
			}
			return string(decoded), nil
		}
	}

	return "", fmt.Errorf("key %q not found in %s %q", key, kindUpper, name)
}

// extractSecretKey extracts a string value from a Secret.
// It returns an error if the reference or data is malformed, or a placeholder if the resource is not found.
func (c *conversionContext) extractSecretKey(sel corev1.SecretKeySelector) (string, error) {
	if sel.Name == "" && sel.Key == "" {
		return "", nil
	}
	return c.extractResourceKey(KindSecret, sel.Name, sel.Key)
}

// extractConfigMapKey extracts a string value from a ConfigMap.
// It returns an error if the reference or data is malformed, or a placeholder if the resource is not found.
func (c *conversionContext) extractConfigMapKey(sel corev1.ConfigMapKeySelector) (string, error) {
	if sel.Name == "" && sel.Key == "" {
		return "", nil
	}
	return c.extractResourceKey(KindConfigMap, sel.Name, sel.Key)
}

// convertConfigMapToSecretSelector translates a ConfigMapKeySelector to a SecretSelector.
// It returns an error if the reference is malformed.
func (c *conversionContext) convertConfigMapToSecretSelector(sel *corev1.ConfigMapKeySelector) (*monitoringv1.SecretSelector, error) {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil, nil
	}
	if sel.Name == "" {
		return nil, fmt.Errorf("configmap reference has an empty name for key %q", sel.Key)
	}
	if sel.Key == "" {
		return nil, fmt.Errorf("configmap reference has an empty key for name %q", sel.Name)
	}

	secretName := "secret-" + sel.Name
	secretKey := sel.Key

	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn("ConfigMap reference had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.",
			slog.String("configmap", sel.Name))
	}

	if c.generatedSecrets == nil {
		c.generatedSecrets = make(map[string]*unstructured.Unstructured)
	}

	if _, exists := c.generatedSecrets[secretName]; !exists {
		obj, ok := c.cache.Get(KindConfigMap, c.namespace, sel.Name)
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
			newSecret.SetNamespace(c.namespace)

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

	secretRef := &monitoringv1.SecretKeySelector{Name: secretName, Key: secretKey, Namespace: c.namespace}
	return &monitoringv1.SecretSelector{Secret: secretRef}, nil
}

// convertSecretOrConfigMapToSecretSelector translates a SecretOrConfigMap to a SecretSelector.
// It returns an error if the selected configuration reference is malformed.
func (c *conversionContext) convertSecretOrConfigMapToSecretSelector(sel pomonitoringv1.SecretOrConfigMap) (*monitoringv1.SecretSelector, error) {
	if sel.Secret != nil {
		return c.convertSecretSelector(sel.Secret)
	}

	if sel.ConfigMap != nil {
		return c.convertConfigMapToSecretSelector(sel.ConfigMap)
	}

	return nil, nil
}

// convertSecretSelector translates a SecretKeySelector to a SecretSelector.
// It returns an error if the reference is malformed.
func (c *conversionContext) convertSecretSelector(sel *corev1.SecretKeySelector) (*monitoringv1.SecretSelector, error) {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil, nil
	}
	if sel.Name == "" {
		return nil, fmt.Errorf("secret reference has an empty name for key %q", sel.Key)
	}
	if sel.Key == "" {
		return nil, fmt.Errorf("secret reference has an empty key for name %q", sel.Name)
	}
	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn("Secret reference had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.",
			slog.String("secret", sel.Name))
	}
	secretRef := &monitoringv1.SecretKeySelector{Name: sel.Name, Key: sel.Key, Namespace: c.namespace}
	return &monitoringv1.SecretSelector{Secret: secretRef}, nil
}

// convertBasicAuth maps PO BasicAuth to GMP BasicAuth, extracting the username string.
// It returns an error if either the username or password secret reference is malformed or invalid.
func (c *conversionContext) convertBasicAuth(ba *pomonitoringv1.BasicAuth) (*monitoringv1.BasicAuth, error) {
	if ba == nil {
		return nil, nil
	}
	username, err := c.extractSecretKey(ba.Username)
	if err != nil {
		return nil, err
	}
	password, err := c.convertSecretSelector(&ba.Password)
	if err != nil {
		return nil, err
	}
	return &monitoringv1.BasicAuth{
		Username: username,
		Password: password,
	}, nil
}

// convertSafeTLSConfig maps PO SafeTLSConfig to GMP TLS, wrapping ConfigMaps into Secrets.
// It returns an error if any referenced certificate secret or configmap is malformed.
func (c *conversionContext) convertSafeTLSConfig(tls *pomonitoringv1.SafeTLSConfig) (*monitoringv1.TLS, error) {
	if tls == nil {
		return nil, nil
	}
	gmpTLS := &monitoringv1.TLS{}
	if tls.InsecureSkipVerify != nil {
		gmpTLS.InsecureSkipVerify = *tls.InsecureSkipVerify
	}
	if tls.ServerName != nil {
		gmpTLS.ServerName = *tls.ServerName
	}
	if tls.CA.Secret != nil || tls.CA.ConfigMap != nil {
		ca, err := c.convertSecretOrConfigMapToSecretSelector(tls.CA)
		if err != nil {
			return nil, err
		}
		gmpTLS.CA = ca
	}
	if tls.Cert.Secret != nil || tls.Cert.ConfigMap != nil {
		cert, err := c.convertSecretOrConfigMapToSecretSelector(tls.Cert)
		if err != nil {
			return nil, err
		}
		gmpTLS.Cert = cert
	}
	if tls.KeySecret != nil {
		key, err := c.convertSecretSelector(tls.KeySecret)
		if err != nil {
			return nil, err
		}
		gmpTLS.Key = key
	}
	return gmpTLS, nil
}

// convertOAuth2 maps PO OAuth2 to GMP OAuth2, extracting the clientID string.
// It returns an error if any secret or configmap reference is malformed or invalid.
func (c *conversionContext) convertOAuth2(oa *pomonitoringv1.OAuth2) (*monitoringv1.OAuth2, error) {
	if oa == nil {
		return nil, nil
	}
	clientID := ""
	var err error
	if oa.ClientID.Secret != nil {
		clientID, err = c.extractSecretKey(*oa.ClientID.Secret)
	} else if oa.ClientID.ConfigMap != nil {
		clientID, err = c.extractConfigMapKey(*oa.ClientID.ConfigMap)
	} else {
		c.logger.Warn("OAuth2 clientID neither defined as Secret nor ConfigMap. Hardcoding placeholder.")
		clientID = "<MISSING_OAUTH2_CLIENT_ID>"
	}
	if err != nil {
		return nil, err
	}

	clientSecret, err := c.convertSecretSelector(&oa.ClientSecret)
	if err != nil {
		return nil, err
	}

	return &monitoringv1.OAuth2{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		TokenURL:       oa.TokenURL,
		Scopes:         oa.Scopes,
		EndpointParams: oa.EndpointParams,
	}, nil
}

// convertAuthorization maps PO SafeAuthorization to GMP Auth.
// It returns an error if the credentials secret reference is malformed.
func (c *conversionContext) convertAuthorization(auth *pomonitoringv1.SafeAuthorization) (*monitoringv1.Auth, error) {
	if auth == nil {
		return nil, nil
	}
	var credentials *monitoringv1.SecretSelector
	var err error
	if auth.Credentials != nil {
		credentials, err = c.convertSecretSelector(auth.Credentials)
		if err != nil {
			return nil, err
		}
	}
	return &monitoringv1.Auth{
		Type:        auth.Type,
		Credentials: credentials,
	}, nil
}

func convertMetricRelabelings(
	logger *slog.Logger,
	configs []pomonitoringv1.RelabelConfig,
) ([]monitoringv1.RelabelingRule, error) {
	var rules []monitoringv1.RelabelingRule

	for _, config := range configs {
		action := strings.ToLower(config.Action)
		if action == "" {
			action = actionReplace
		}

		targetLabel := config.TargetLabel
		switch action {
		case actionLabelMap:
			logger.Warn("metricRelabelings rule uses 'action: labelmap' which is not supported by GMP and has been dropped.")
			continue
		case actionReplace, actionHashMod, actionLowercase, actionUppercase:
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
			Action:      action,
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

	return rules, nil
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
