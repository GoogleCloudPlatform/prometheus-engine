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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// preScrapeRelabelingResult holds the label mappings and selector rules extracted from pre-scrape relabelings.
type preScrapeRelabelingResult struct {
	FromPod          []monitoringv1.LabelMapping
	Metadata         *[]string
	MatchLabels      map[string]string
	MatchExpressions []metav1.LabelSelectorRequirement
	PromotedRules    []monitoringv1.RelabelingRule
}

// extractedPreScrapeRules holds all translated rules, separated by where they belong in GMP.
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
		logger.Warn("Stripped all metadata labels and annotations. Reconfigure them manually if needed")
	}

	return dst
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

		if shouldSkipRelabelConfig(logger, config, action) {
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
func extractPreScrapeRelabelings(logger *slog.Logger, endpoints []pomonitoringv1.PodMetricsEndpoint) (extractedPreScrapeRules, error) {
	var (
		epResults   []preScrapeRelabelingResult
		combined    preScrapeRelabelingResult
		rawMetadata []string
	)
	isSingleEndpoint := len(endpoints) == 1

	for _, ep := range endpoints {
		var r preScrapeRelabelingResult
		if len(ep.RelabelConfigs) > 0 {
			var err error
			r, err = convertPreScrapeRelabelings(logger, ep.RelabelConfigs, isSingleEndpoint)
			if err != nil {
				return extractedPreScrapeRules{}, err
			}
			combined.FromPod = append(combined.FromPod, r.FromPod...)
			if r.Metadata != nil {
				rawMetadata = append(rawMetadata, *r.Metadata...)
			}
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
func mergeLabelSelector(base metav1.LabelSelector, extraLabels map[string]string, extraExprs []metav1.LabelSelectorRequirement) (metav1.LabelSelector, error) {
	if len(extraLabels) == 0 && len(extraExprs) == 0 {
		return *base.DeepCopy(), nil
	}
	res := base.DeepCopy()
	if len(extraLabels) > 0 && res.MatchLabels == nil {
		res.MatchLabels = make(map[string]string)
	}
	for k, v := range extraLabels {
		if existing, exists := res.MatchLabels[k]; exists && existing != v {
			return metav1.LabelSelector{}, fmt.Errorf("selector conflict: label %q has conflicting values %q (base selector) and %q (relabeling rule)", k, existing, v)
		}
		res.MatchLabels[k] = v
	}
	res.MatchExpressions = append(res.MatchExpressions, extraExprs...)
	return *res, nil
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
	var (
		credentials *monitoringv1.SecretSelector
		err         error
	)
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

// applyAuthAndTLS converts credentials and TLS settings for a generic endpoint.
func (c *conversionContext) applyAuthAndTLS(
	i int,
	gmpEp *monitoringv1.ScrapeEndpoint,
	basicAuth *pomonitoringv1.BasicAuth,
	oAuth2 *pomonitoringv1.OAuth2,
	tlsConfig *pomonitoringv1.SafeTLSConfig,
	authorization *pomonitoringv1.SafeAuthorization,
	bearerTokenSecret corev1.SecretKeySelector,
) error {
	if basicAuth != nil {
		ba, err := c.convertBasicAuth(basicAuth)
		if err != nil {
			return fmt.Errorf("endpoint [%d]: basicAuth: %w", i, err)
		}
		gmpEp.BasicAuth = ba
	}
	if oAuth2 != nil {
		oa, err := c.convertOAuth2(oAuth2)
		if err != nil {
			return fmt.Errorf("endpoint [%d]: oAuth2: %w", i, err)
		}
		gmpEp.OAuth2 = oa
	}
	if tlsConfig != nil {
		tls, err := c.convertSafeTLSConfig(tlsConfig)
		if err != nil {
			return fmt.Errorf("endpoint [%d]: tlsConfig: %w", i, err)
		}
		gmpEp.TLS = tls
	}
	if authorization != nil {
		auth, err := c.convertAuthorization(authorization)
		if err != nil {
			return fmt.Errorf("endpoint [%d]: authorization: %w", i, err)
		}
		gmpEp.Authorization = auth
	}

	// Handle deprecated BearerTokenSecret -> Authorization.
	if bearerTokenSecret.Name != "" { // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
		if gmpEp.Authorization != nil {
			c.logger.Warn("Endpoint has both 'bearerTokenSecret' and 'authorization' defined. Dropping 'bearerTokenSecret'.",
				slog.Int("endpoint_index", i))
		} else {
			tokenSecret := bearerTokenSecret // nolint:staticcheck // Map deprecated BearerTokenSecret for backwards compatibility.
			auth, err := c.convertAuthorization(&pomonitoringv1.SafeAuthorization{Credentials: &tokenSecret})
			if err != nil {
				return fmt.Errorf("endpoint [%d]: bearerTokenSecret: %w", i, err)
			}
			gmpEp.Authorization = auth
		}
	}
	return nil
}

func convertMetricRelabelings(
	logger *slog.Logger,
	configs []pomonitoringv1.RelabelConfig,
) ([]monitoringv1.RelabelingRule, error) {
	var rules []monitoringv1.RelabelingRule

	for _, config := range configs {
		action := relabel.Action(strings.ToLower(config.Action))
		if action == "" {
			action = relabel.Replace
		}

		if shouldSkipRelabelConfig(logger, config, action) {
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

// shouldSkipRelabelConfig checks if the relabel config uses unsupported actions or references annotations.
func shouldSkipRelabelConfig(logger *slog.Logger, config pomonitoringv1.RelabelConfig, action relabel.Action) bool {
	switch action {
	case relabel.Replace, relabel.HashMod, "":
		if config.TargetLabel == "" {
			logger.Warn(fmt.Sprintf("Relabeling rule uses 'action: %s' with an empty 'targetLabel', which is invalid in Prometheus and has been dropped.", action))
			return true
		}
	case relabel.Keep, relabel.Drop, relabel.LabelDrop, relabel.LabelKeep:
		// Supported actions that do not require targetLabel.
	case relabel.LabelMap, relabel.Lowercase, relabel.Uppercase, relabel.KeepEqual, relabel.DropEqual:
		logger.Warn(fmt.Sprintf("Relabeling rule uses 'action: %s' which is not supported by GMP and has been dropped.", action))
		return true
	default:
		logger.Warn(fmt.Sprintf("Relabeling rule uses unknown 'action: %s' which is not supported by GMP and has been dropped.", action))
		return true
	}

	for _, sl := range config.SourceLabels {
		if strings.HasPrefix(string(sl), "__meta_kubernetes_pod_annotation_") {
			logger.Warn(fmt.Sprintf("Relabeling rule referencing pod annotation %q is unsupported in GMP. The rule has been dropped.", string(sl)))
			return true
		}
		if strings.HasPrefix(string(sl), "__meta_kubernetes_node_") && string(sl) != "__meta_kubernetes_node_name" {
			logger.Warn(fmt.Sprintf("Relabeling rule referencing node metadata %q is unsupported in GMP (only node name is supported). The rule has been dropped.", string(sl)))
			return true
		}
	}
	return false
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
			return false, fmt.Errorf("conflicting keep rules for label %q: cannot require both %q and %q simultaneously", labelName, existing, parts[0])
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

// warnUnsupportedSpecFields logs warnings for spec-level fields that GMP does not support or need.
func warnUnsupportedSpecFields(logger *slog.Logger, spec *pomonitoringv1.PodMonitorSpec) {
	if spec == nil {
		return
	}
	if spec.TargetLimit != nil {
		logger.Warn("Field 'targetLimit' is unnecessary in GMP Managed Collection and has been dropped. Target discovery and scaling are managed automatically by GKE.")
	}
	if spec.KeepDroppedTargets != nil {
		logger.Warn("Field 'keepDroppedTargets' is unnecessary in GMP Managed Collection and has been dropped.")
	}
	if spec.BodySizeLimit != nil {
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

// toStrictUnstructured converts a struct to a strictly JSON-compatible unstructured map.
// uses JSON to silently convert unsupported Go primitives (like uint64) into safe float64 numbers.
// ensures the resulting map will not panic on DeepCopy.
func toStrictUnstructured(obj interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var u map[string]interface{}
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, err
	}
	return u, nil
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

// resolveAttachMetadata appends "node" to metadata if attachMetadata.node is enabled.
func resolveAttachMetadata(attachMetadata *pomonitoringv1.AttachMetadata, baseMetadata *[]string, isCluster bool) *[]string {
	if attachMetadata != nil && attachMetadata.Node != nil && *attachMetadata.Node {
		if baseMetadata == nil {
			if isCluster {
				union := unionMetadata([]string{labelNode}, clusterMetadataDefaults)
				return &union
			}
			return &[]string{labelNode}
		}
		if !slices.Contains(*baseMetadata, labelNode) {
			metadataCopy := append(slices.Clone(*baseMetadata), labelNode)
			return &metadataCopy
		}
	}
	return baseMetadata
}

// resolveFilterRunning evaluates filterRunning settings across endpoints and resolves them to a single resource-level setting.
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
func resolveScrapeIntervalAndTimeout(logger *slog.Logger, interval, timeout string) (resolvedInterval, resolvedTimeout string, err error) {
	// TODO(M2): Inherit global scrape interval from Prometheus CR if empty.
	if interval == "" {
		logger.Warn("Scrape interval is empty. Defaulting to '30s' as GMP requires this field.")
		interval = "30s"
	}

	intDur, err := prommodel.ParseDuration(interval)
	if err != nil {
		return "", "", fmt.Errorf("invalid interval %q: %w", interval, err)
	}

	// TODO(M2): Inherit global scrape timeout from Prometheus CR if empty.
	if timeout != "" {
		toDur, err := prommodel.ParseDuration(timeout)
		if err != nil {
			return "", "", fmt.Errorf("invalid scrapeTimeout %q: %w", timeout, err)
		}
		if toDur > intDur {
			logger.Warn("Scrape timeout is larger than scrape interval. Capping timeout to interval.",
				slog.String("timeout", timeout),
				slog.String("interval", interval))
			timeout = interval
		}
	}
	return interval, timeout, nil
}

// convertProxyURL verifies proxy URL credentials.
func convertProxyURL(proxyURL *string) (string, error) {
	if proxyURL == nil {
		return "", nil
	}
	if strings.Contains(*proxyURL, "@") {
		return "", errors.New("proxyUrl contains credentials (matches '@'), which is blocked by GMP API validation")
	}
	return *proxyURL, nil
}

// warnUnsupportedEndpointFields logs warnings for fields that GMP does not support.
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
		logger.Warn(fmt.Sprintf("endpoint [%d]: fField 'trackTimestampsStaleness' is unsupported in GMP and has been dropped.", i))
	}
}

// combineAndConvertRelabelings combines promoted pre-scrape rules and converts metricRelabelings.
func combineAndConvertRelabelings(logger *slog.Logger, promoted []monitoringv1.RelabelingRule, configs []pomonitoringv1.RelabelConfig) ([]monitoringv1.RelabelingRule, error) {
	totalRules := len(promoted) + len(configs)
	if totalRules == 0 {
		return nil, nil
	}

	allRules := make([]monitoringv1.RelabelingRule, 0, totalRules)
	allRules = append(allRules, promoted...)

	if len(configs) > 0 {
		rules, err := convertMetricRelabelings(logger, configs)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}
