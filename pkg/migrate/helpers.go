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
	"slices"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// protectedLabels contains the list of labels that are protected by GMP and cannot
// be overwritten by targetLabels or relabeling rules.
var protectedLabels = map[string]bool{
	"project_id":                true,
	"location":                  true,
	"cluster":                   true,
	"namespace":                 true,
	"job":                       true,
	"instance":                  true,
	"top_level_controller":      true,
	"top_level_controller_type": true,
	"__address__":               true,
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

// extractResourceKey is a consolidated helper that fetches a key from a ConfigMap or Secret.
func (c *conversionContext) extractResourceKey(kind, name, key string) string {
	kindUpper := strings.ToUpper(kind)
	if name == "" {
		c.logger.Warn(fmt.Sprintf("%sKeySelector has empty name for key %q. Hardcoding placeholder.", kind, key))
		return fmt.Sprintf("<MISSING_%s_NAME_KEY_%s>", kindUpper, key)
	}

	obj, ok := c.cache.Get(kind, c.namespace, name)
	if !ok {
		c.logger.Warn(fmt.Sprintf("%s %q not found in cache. Cannot extract key %q. Hardcoding placeholder.", kind, name, key))
		return fmt.Sprintf("<MISSING_%s_%s_KEY_%s>", kindUpper, name, key)
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
				c.logger.Warn(fmt.Sprintf("Failed to decode base64 data for key %q in secret %q. Hardcoding placeholder.", key, name))
				return fmt.Sprintf("<MALFORMED_SECRET_%s_KEY_%s>", name, key)
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
				c.logger.Warn(fmt.Sprintf("Failed to decode base64 data for key %q in configmap %q. Hardcoding placeholder.", key, name))
				return fmt.Sprintf("<MALFORMED_CONFIGMAP_%s_KEY_%s>", name, key)
			}
			return string(decoded)
		}
	}

	c.logger.Warn(fmt.Sprintf("Key %q not found in %s %q. Hardcoding placeholder.", key, strings.ToLower(kind), name))
	return fmt.Sprintf("<MISSING_KEY_%s_IN_%s_%s>", key, kindUpper, name)
}

// extractSecretKey extracts a string value from a Secret, returning a placeholder and warning if not found.
func (c *conversionContext) extractSecretKey(sel corev1.SecretKeySelector) string {
	return c.extractResourceKey(KindSecret, sel.Name, sel.Key)
}

// extractConfigMapKey extracts a string value from a ConfigMap, returning a placeholder and warning if not found.
func (c *conversionContext) extractConfigMapKey(sel corev1.ConfigMapKeySelector) string {
	return c.extractResourceKey(KindConfigMap, sel.Name, sel.Key)
}

func (c *conversionContext) convertConfigMapToSecretSelector(sel *corev1.ConfigMapKeySelector) *monitoringv1.SecretSelector {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil
	}
	if sel.Name == "" {
		c.logger.Warn(fmt.Sprintf("ConfigMap reference for key %q has an empty name. Hardcoding placeholder and skipping Secret manifest generation. You must fix this reference and ensure the Secret is created before applying.", sel.Key))
		return &monitoringv1.SecretSelector{
			Secret: &monitoringv1.SecretKeySelector{Name: "<MISSING_CONFIGMAP_NAME>", Key: sel.Key, Namespace: c.namespace},
		}
	}
	if sel.Key == "" {
		c.logger.Warn(fmt.Sprintf("ConfigMap reference %q has an empty key. Hardcoding placeholder. You must fix this reference before applying.", sel.Name))
		return &monitoringv1.SecretSelector{
			Secret: &monitoringv1.SecretKeySelector{Name: "secret-" + sel.Name, Key: "<MISSING_CONFIGMAP_KEY>", Namespace: c.namespace},
		}
	}

	secretName := "secret-" + sel.Name
	secretKey := sel.Key

	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn(fmt.Sprintf("ConfigMap reference %q had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.", sel.Name))
	}

	if c.generatedSecrets == nil {
		c.generatedSecrets = make(map[string]*unstructured.Unstructured)
	}

	if _, exists := c.generatedSecrets[secretName]; !exists {
		obj, ok := c.cache.Get(KindConfigMap, c.namespace, sel.Name)
		if !ok {
			c.logger.Warn(fmt.Sprintf("TLS ConfigMap reference %q was not found in the inputs. Updated reference to GMP Secret %q, but you must manually convert your ConfigMap to a Secret with this name in GMP.", sel.Name, secretName))
		} else {
			c.logger.Info(fmt.Sprintf("Translated TLS ConfigMap reference %q to GMP Secret. Generated new Secret manifest %q.", sel.Name, secretName))

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
	return &monitoringv1.SecretSelector{Secret: secretRef}
}

// convertSecretOrConfigMapToSecretSelector translates to a SecretSelector and warns on missing caches or optional configs.
func (c *conversionContext) convertSecretOrConfigMapToSecretSelector(sel pomonitoringv1.SecretOrConfigMap) *monitoringv1.SecretSelector {
	if sel.Secret != nil {
		return c.convertSecretSelector(sel.Secret)
	}

	if sel.ConfigMap != nil {
		return c.convertConfigMapToSecretSelector(sel.ConfigMap)
	}

	return nil
}

func (c *conversionContext) convertSecretSelector(sel *corev1.SecretKeySelector) *monitoringv1.SecretSelector {
	if sel == nil || (sel.Name == "" && sel.Key == "") {
		return nil
	}
	if sel.Name == "" {
		c.logger.Warn(fmt.Sprintf("Secret reference for key %q has an empty name. Hardcoding placeholder. You must fix this reference and ensure the Secret is created before applying.", sel.Key))
		return &monitoringv1.SecretSelector{
			Secret: &monitoringv1.SecretKeySelector{Name: "<MISSING_SECRET_NAME>", Key: sel.Key, Namespace: c.namespace},
		}
	}
	if sel.Key == "" {
		c.logger.Warn(fmt.Sprintf("Secret reference %q has an empty key. Hardcoding placeholder. You must fix this reference before applying.", sel.Name))
		return &monitoringv1.SecretSelector{
			Secret: &monitoringv1.SecretKeySelector{Name: sel.Name, Key: "<MISSING_SECRET_KEY>", Namespace: c.namespace},
		}
	}
	if sel.Optional != nil && *sel.Optional {
		c.logger.Warn(fmt.Sprintf("Secret reference %q had 'optional: true'. GMP does not support optional secrets. The reference is now mandatory.", sel.Name))
	}
	secretRef := &monitoringv1.SecretKeySelector{Name: sel.Name, Key: sel.Key, Namespace: c.namespace}
	return &monitoringv1.SecretSelector{Secret: secretRef}
}

// convertBasicAuth maps PO BasicAuth to GMP BasicAuth, extracting the username string.
func (c *conversionContext) convertBasicAuth(ba *pomonitoringv1.BasicAuth) *monitoringv1.BasicAuth {
	if ba == nil {
		return nil
	}
	return &monitoringv1.BasicAuth{
		Username: c.extractSecretKey(ba.Username),
		Password: c.convertSecretSelector(&ba.Password),
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
		c.logger.Warn("OAuth2 clientID neither defined as Secret nor ConfigMap. Hardcoding placeholder.")
		clientID = "<MISSING_OAUTH2_CLIENT_ID>"
	}

	return &monitoringv1.OAuth2{
		ClientID:       clientID,
		ClientSecret:   c.convertSecretSelector(&oa.ClientSecret),
		TokenURL:       oa.TokenURL,
		Scopes:         oa.Scopes,
		EndpointParams: oa.EndpointParams,
	}
}

// convertAuthorization maps PO SafeAuthorization to GMP Auth.
func (c *conversionContext) convertAuthorization(auth *pomonitoringv1.SafeAuthorization) *monitoringv1.Auth {
	if auth == nil {
		return nil
	}
	return &monitoringv1.Auth{
		Type:        auth.Type,
		Credentials: c.convertSecretSelector(auth.Credentials),
	}
}

func convertMetricRelabelings(
	logger *slog.Logger,
	configs []pomonitoringv1.RelabelConfig,
) ([]monitoringv1.RelabelingRule, error) {
	var rules []monitoringv1.RelabelingRule

	for _, config := range configs {
		action := strings.ToLower(config.Action)
		if action == "" {
			action = "replace"
		}

		targetLabel := config.TargetLabel
		switch action {
		case "labelmap":
			logger.Warn("metricRelabelings rule uses 'action: labelmap' which is not supported by GMP and has been dropped.")
			continue
		case "replace", "hashmod", "lowercase", "uppercase":
			if protectedLabels[config.TargetLabel] {
				targetLabel = "exported_" + config.TargetLabel
				logger.Warn(fmt.Sprintf("Relabeling rule attempts to write to protected target label %q. Renamed target to %q.",
					config.TargetLabel, targetLabel))
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
			logger.Warn(fmt.Sprintf("%s target label %q maps to target label %q which is already taken. Skipping.", labelKind, l, target))
			continue
		}

		seenTargets[target] = true
		mapping := monitoringv1.LabelMapping{From: l}

		if target != l {
			mapping.To = target
			logger.Warn(fmt.Sprintf("%s target label %q is protected in GMP. Renamed target to %q.", labelKind, l, target))
		}

		fromPod = append(fromPod, mapping)
	}

	if jobLabel != "" {
		target := "exported_job"
		if !seenTargets[target] {
			logger.Warn(fmt.Sprintf("GMP does not support overriding the protected 'job' label. Value on label %q has been copied into the target label 'exported_job'.", jobLabel))
			fromPod = append(fromPod, monitoringv1.LabelMapping{
				From: jobLabel,
				To:   target,
			})
			seenTargets[target] = true
		} else {
			logger.Warn(fmt.Sprintf("Job label %q could not be mapped to 'exported_job' because 'exported_job' is already taken by another target label mapping.", jobLabel))
		}
	}

	return fromPod
}
