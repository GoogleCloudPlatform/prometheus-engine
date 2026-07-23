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
	"log/slog"
	"os"
	"reflect"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func newTestConversionContext() *conversionContext {
	return &conversionContext{
		logger:    slog.New(slog.NewTextHandler(os.Stdout, nil)),
		cache:     NewResourceCache(),
		namespace: "default",
	}
}

func addSecretToCache(cache *ResourceCache, namespace, name, key, value string, isStringData bool) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if isStringData {
		secret.StringData = map[string]string{key: value}
	} else {
		secret.Data = map[string][]byte{key: []byte(value)}
	}

	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	return cache.Add(&unstructured.Unstructured{Object: u})
}

func addConfigMapToCache(cache *ResourceCache, namespace, name, key, value string) error {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{key: value},
	}

	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	return cache.Add(&unstructured.Unstructured{Object: u})
}

func TestExtractSecretKey(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(cache *ResourceCache) error
		selector    corev1.SecretKeySelector
		expectedVal string
		wantErr     bool
	}{
		{
			name:        "Missing secret",
			setupCache:  func(_ *ResourceCache) error { return nil }, // Empty cache.
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "<MISSING_SECRET_missing_KEY_user>",
			wantErr:     false,
		},
		{
			name: "Secret with StringData",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret", "user", "admin", true)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "user"},
			expectedVal: "admin",
			wantErr:     false,
		},
		{
			name: "Secret with Base64 Data",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret-2", "pass", "supersecret", false)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret-2"}, Key: "pass"},
			expectedVal: "supersecret",
			wantErr:     false,
		},
		{
			name:        "Secret reference with empty Name",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.SecretKeySelector{Key: "user"},
			expectedVal: "",
			wantErr:     true,
		},
		{
			name:        "Secret reference with empty Key",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
			expectedVal: "",
			wantErr:     true,
		},
		{
			name: "Secret exists but key missing",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret", "user", "admin", true)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "password"},
			expectedVal: "",
			wantErr:     true,
		},
		{
			name: "Secret exists but data corrupted",
			setupCache: func(cache *ResourceCache) error {
				secret := &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]any{
							"name":      "corrupted-secret",
							"namespace": "default",
						},
						"data": map[string]any{
							"pass": "not-base64-data-with-invalid-chars-@!#",
						},
					},
				}
				return cache.Add(secret)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "corrupted-secret"}, Key: "pass"},
			expectedVal: "",
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			val, err := ctx.extractSecretKey(tc.selector)
			if (err != nil) != tc.wantErr {
				t.Fatalf("extractSecretKey() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
		})
	}
}

func TestExtractConfigMapKey(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(cache *ResourceCache) error
		selector    corev1.ConfigMapKeySelector
		expectedVal string
		wantErr     bool
	}{
		{
			name:        "Missing configmap",
			setupCache:  func(_ *ResourceCache) error { return nil }, // Empty cache.
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "<MISSING_CONFIGMAP_missing_KEY_user>",
			wantErr:     false,
		},
		{
			name: "Found configmap",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "my-cm", "id", "client-123")
			},
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "id"},
			expectedVal: "client-123",
			wantErr:     false,
		},
		{
			name:        "Configmap reference with empty Name",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.ConfigMapKeySelector{Key: "user"},
			expectedVal: "",
			wantErr:     true,
		},
		{
			name:        "Configmap reference with empty Key",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}},
			expectedVal: "",
			wantErr:     true,
		},
		{
			name: "Configmap exists but key missing",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "my-cm", "id", "client-123")
			},
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "secret"},
			expectedVal: "",
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			val, err := ctx.extractConfigMapKey(tc.selector)
			if (err != nil) != tc.wantErr {
				t.Fatalf("extractConfigMapKey() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
		})
	}
}

func TestConvertConfigMapToSecretSelector(t *testing.T) {
	tests := []struct {
		name                  string
		setupCache            func(cache *ResourceCache) error
		selector              *corev1.ConfigMapKeySelector
		expectedSecretName    string
		expectedSecretKey     string
		expectGeneratedSecret bool
		wantErr               bool
	}{
		{
			name: "Convert ConfigMap to Secret",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "tls-cm", "ca.crt", "cert-data")
			},
			selector: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"},
				Key:                  "ca.crt",
			},
			expectedSecretName:    "secret-tls-cm",
			expectedSecretKey:     "ca.crt",
			expectGeneratedSecret: true,
			wantErr:               false,
		},
		{
			name:                  "Nil selector",
			setupCache:            func(_ *ResourceCache) error { return nil },
			selector:              nil,
			expectedSecretName:    "",
			expectedSecretKey:     "",
			expectGeneratedSecret: false,
			wantErr:               false,
		},
		{
			name:                  "Empty name reference",
			setupCache:            func(_ *ResourceCache) error { return nil },
			selector:              &corev1.ConfigMapKeySelector{Key: "ca.crt"},
			expectedSecretName:    "",
			expectedSecretKey:     "",
			expectGeneratedSecret: false,
			wantErr:               true,
		},
		{
			name:                  "Empty key reference",
			setupCache:            func(_ *ResourceCache) error { return nil },
			selector:              &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"}},
			expectedSecretName:    "",
			expectedSecretKey:     "",
			expectGeneratedSecret: false,
			wantErr:               true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpSel, err := ctx.convertConfigMapToSecretSelector(tc.selector)
			if (err != nil) != tc.wantErr {
				t.Fatalf("convertConfigMapToSecretSelector() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.selector == nil {
				if gmpSel != nil {
					t.Errorf("expected nil result for nil selector, got %+v", gmpSel)
				}
				return
			}
			if tc.wantErr {
				return
			}

			if gmpSel == nil || gmpSel.Secret == nil || gmpSel.Secret.Name != tc.expectedSecretName || gmpSel.Secret.Key != tc.expectedSecretKey {
				t.Errorf("unexpected secret selector: %+v", gmpSel)
			}

			if tc.expectGeneratedSecret {
				genSecrets := ctx.getGeneratedSecrets()
				if len(genSecrets) != 1 {
					t.Fatalf("expected 1 generated secret, got %d", len(genSecrets))
				}
				gen := genSecrets[0]
				if gen.GetName() != tc.expectedSecretName {
					t.Errorf("expected generated secret name %s, got %s", tc.expectedSecretName, gen.GetName())
				}
			}
		})
	}
}

func TestConvertBasicAuth(t *testing.T) {
	tests := []struct {
		name             string
		setupCache       func(cache *ResourceCache) error
		basicAuth        *pomonitoringv1.BasicAuth
		expectedUser     string
		expectedPassName string
		expectedPassKey  string
		wantErr          bool
	}{
		{
			name: "Valid BasicAuth conversion",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "auth-secret", "user", "myuser", true)
			},
			basicAuth: &pomonitoringv1.BasicAuth{
				Username: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "user"},
				Password: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "pass"},
			},
			expectedUser:     "myuser",
			expectedPassName: "auth-secret",
			expectedPassKey:  "pass",
			wantErr:          false,
		},
		{
			name:       "Nil BasicAuth",
			setupCache: func(_ *ResourceCache) error { return nil },
			basicAuth:  nil,
			wantErr:    false,
		},
		{
			name:       "Malformed Username reference",
			setupCache: func(_ *ResourceCache) error { return nil },
			basicAuth: &pomonitoringv1.BasicAuth{
				Username: corev1.SecretKeySelector{Key: "user"},
				Password: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "pass"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpBA, err := ctx.convertBasicAuth(tc.basicAuth)
			if (err != nil) != tc.wantErr {
				t.Fatalf("convertBasicAuth() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.basicAuth == nil {
				if gmpBA != nil {
					t.Errorf("expected nil result for nil BasicAuth, got %+v", gmpBA)
				}
				return
			}
			if tc.wantErr {
				return
			}

			if gmpBA.Username != tc.expectedUser {
				t.Errorf("expected username %s, got %s", tc.expectedUser, gmpBA.Username)
			}
			if gmpBA.Password.Secret.Name != tc.expectedPassName || gmpBA.Password.Secret.Key != tc.expectedPassKey {
				t.Errorf("unexpected password selector: %+v", gmpBA.Password)
			}
		})
	}
}

func TestConvertSafeTLSConfig(t *testing.T) {
	trueVal := true
	tests := []struct {
		name               string
		setupCache         func(cache *ResourceCache) error
		tlsConfig          *pomonitoringv1.SafeTLSConfig
		expectedCAName     string
		expectedCAKey      string
		expectedCertName   string
		expectedCertKey    string
		expectedSkipVerify bool
		expectedServerName string
		wantErr            bool
	}{
		{
			name: "Full TLS Config Conversion",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "ca-cm", "ca.crt", "ca-data")
			},
			tlsConfig: &pomonitoringv1.SafeTLSConfig{
				CA: pomonitoringv1.SecretOrConfigMap{
					ConfigMap: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "ca-cm"}, Key: "ca.crt"},
				},
				Cert: pomonitoringv1.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "cert-sec"}, Key: "tls.crt"},
				},
				InsecureSkipVerify: &trueVal,
				ServerName:         &[]string{"my-server"}[0],
			},
			expectedCAName:     "secret-ca-cm",
			expectedCAKey:      "ca.crt",
			expectedCertName:   "cert-sec",
			expectedCertKey:    "tls.crt",
			expectedSkipVerify: true,
			expectedServerName: "my-server",
			wantErr:            false,
		},
		{
			name:       "Nil TLS Config",
			setupCache: func(_ *ResourceCache) error { return nil },
			tlsConfig:  nil,
			wantErr:    false,
		},
		{
			name:       "Malformed CA reference",
			setupCache: func(_ *ResourceCache) error { return nil },
			tlsConfig: &pomonitoringv1.SafeTLSConfig{
				CA: pomonitoringv1.SecretOrConfigMap{
					ConfigMap: &corev1.ConfigMapKeySelector{Key: "ca.crt"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpTLS, err := ctx.convertSafeTLSConfig(tc.tlsConfig)
			if (err != nil) != tc.wantErr {
				t.Fatalf("convertSafeTLSConfig() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.tlsConfig == nil {
				if gmpTLS != nil {
					t.Errorf("expected nil result for nil TLS config, got %+v", gmpTLS)
				}
				return
			}
			if tc.wantErr {
				return
			}

			if gmpTLS.CA.Secret.Name != tc.expectedCAName || gmpTLS.CA.Secret.Key != tc.expectedCAKey {
				t.Errorf("unexpected CA selector: %+v", gmpTLS.CA)
			}
			if gmpTLS.Cert.Secret.Name != tc.expectedCertName || gmpTLS.Cert.Secret.Key != tc.expectedCertKey {
				t.Errorf("unexpected Cert selector: %+v", gmpTLS.Cert)
			}
			if gmpTLS.InsecureSkipVerify != tc.expectedSkipVerify {
				t.Errorf("expected InsecureSkipVerify %v, got %v", tc.expectedSkipVerify, gmpTLS.InsecureSkipVerify)
			}
			if gmpTLS.ServerName != tc.expectedServerName {
				t.Errorf("expected server name %s, got %s", tc.expectedServerName, gmpTLS.ServerName)
			}
		})
	}
}

func TestConvertConfigMapToSecretSelectorDeduplication(t *testing.T) {
	ctx := newTestConversionContext()
	err := addConfigMapToCache(ctx.cache, "default", "tls-cm", "ca.crt", "cert-data")
	if err != nil {
		t.Fatalf("failed to setup cache: %v", err)
	}

	selector := &corev1.ConfigMapKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"},
		Key:                  "ca.crt",
	}

	// Call first time.
	gmpSel1, err := ctx.convertConfigMapToSecretSelector(selector)
	if err != nil {
		t.Fatalf("first call failed with error: %v", err)
	}
	if gmpSel1 == nil || gmpSel1.Secret.Name != "secret-tls-cm" {
		t.Fatal("first call failed to translate selector")
	}

	// Call second time.
	gmpSel2, err := ctx.convertConfigMapToSecretSelector(selector)
	if err != nil {
		t.Fatalf("second call failed with error: %v", err)
	}
	if gmpSel2 == nil || gmpSel2.Secret.Name != "secret-tls-cm" {
		t.Fatal("second call failed to translate selector")
	}

	// Ensure only one secret was generated in total.
	genSecrets := ctx.getGeneratedSecrets()
	if len(genSecrets) != 1 {
		t.Fatalf("expected exactly 1 generated secret due to duplication, got %d", len(genSecrets))
	}
}

func TestParseAndCleanNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Deduplication and Trimming",
			input:    []string{"ns-a", " ns-a ", "  ns-a", "", "   ", "ns-b ", "ns-c"},
			expected: []string{"ns-a", "ns-b", "ns-c"},
		},
		{
			name:     "Empty list",
			input:    []string{},
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := ParseAndCleanNamespaces(tc.input)
			if !reflect.DeepEqual(actual, tc.expected) {
				if len(actual) == 0 && len(tc.expected) == 0 {
					return
				}
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestConvertPreScrapeRelabelings_TargetFiltering(t *testing.T) {
	tests := []struct {
		name              string
		configs           []pomonitoringv1.RelabelConfig
		expectMatchLabels map[string]string
		expectExprLen     int
		expectExprKey     string
		expectExprOp      metav1.LabelSelectorOperator
		expectExprValues  []string
	}{
		{
			name: "Keep exact match to MatchLabels",
			configs: []pomonitoringv1.RelabelConfig{
				{
					SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_env"},
					Regex:        "^production$",
					Action:       "keep",
				},
			},
			expectMatchLabels: map[string]string{"env": "production"},
		},
		{
			name: "Drop set match to MatchExpressions",
			configs: []pomonitoringv1.RelabelConfig{
				{
					SourceLabels: []pomonitoringv1.LabelName{"__meta_kubernetes_pod_label_tier"},
					Regex:        "(test|staging)",
					Action:       "drop",
				},
			},
			expectMatchLabels: nil,
			expectExprLen:     1,
			expectExprKey:     "tier",
			expectExprOp:      metav1.LabelSelectorOpNotIn,
			expectExprValues:  []string{"test", "staging"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			result := convertPreScrapeRelabelings(logger, tc.configs, true)

			if !reflect.DeepEqual(result.MatchLabels, tc.expectMatchLabels) {
				if len(result.MatchLabels) != 0 || len(tc.expectMatchLabels) != 0 {
					t.Errorf("expected MatchLabels %v, got %v", tc.expectMatchLabels, result.MatchLabels)
				}
			}

			if len(result.MatchExpressions) != tc.expectExprLen {
				t.Fatalf("expected %d MatchExpressions, got %d", tc.expectExprLen, len(result.MatchExpressions))
			}

			if tc.expectExprLen > 0 {
				expr := result.MatchExpressions[0]
				if expr.Key != tc.expectExprKey || expr.Operator != tc.expectExprOp || !reflect.DeepEqual(expr.Values, tc.expectExprValues) {
					t.Errorf("unexpected MatchExpression: %+v", expr)
				}
			}
		})
	}
}

func TestConvertRelabelings_ProtectedLabels(t *testing.T) {
	tests := []struct {
		name           string
		configs        []pomonitoringv1.RelabelConfig
		expectedTarget string
	}{
		{
			name: "Rename protected project_id",
			configs: []pomonitoringv1.RelabelConfig{
				{SourceLabels: []pomonitoringv1.LabelName{"custom_id"}, TargetLabel: "project_id", Action: "replace"},
			},
			expectedTarget: "exported_project_id",
		},
		{
			name: "Rename protected namespace",
			configs: []pomonitoringv1.RelabelConfig{
				{SourceLabels: []pomonitoringv1.LabelName{"ns"}, TargetLabel: "namespace", Action: "replace"},
			},
			expectedTarget: "exported_namespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			// Test Pre-Scrape promotion.
			res := convertPreScrapeRelabelings(logger, tc.configs, true)
			if len(res.PromotedRules) != 1 || res.PromotedRules[0].TargetLabel != tc.expectedTarget {
				t.Errorf("expected promoted target label %q, got %v", tc.expectedTarget, res.PromotedRules)
			}

			// Test Post-Scrape modification.
			modRules, _ := convertMetricRelabelings(logger, tc.configs)
			if len(modRules) != 1 || modRules[0].TargetLabel != tc.expectedTarget {
				t.Errorf("expected modified metric target label %q, got %v", tc.expectedTarget, modRules)
			}
		})
	}
}

func TestConvertPreScrapeRelabelings_UnsupportedActions(t *testing.T) {
	tests := []struct {
		name    string
		configs []pomonitoringv1.RelabelConfig
	}{
		{
			name: "Drop unsupported label actions",
			configs: []pomonitoringv1.RelabelConfig{
				{SourceLabels: []pomonitoringv1.LabelName{"t"}, Action: "labelmap"},
				{SourceLabels: []pomonitoringv1.LabelName{"t"}, Action: "labelkeep"},
				{SourceLabels: []pomonitoringv1.LabelName{"t"}, Action: "labeldrop"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			res := convertPreScrapeRelabelings(logger, tc.configs, true)
			if len(res.PromotedRules) != 0 {
				t.Errorf("expected all rules to be skipped, got %d rules", len(res.PromotedRules))
			}
		})
	}
}

func TestMergeCollisionWarnings(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("mergeLabelSelector", func(t *testing.T) {
		baseSelector := metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
		extraLabels := map[string]string{"env": "qa", "tier": "backend"} // env conflicts.

		mergedSel := mergeLabelSelector(logger, baseSelector, extraLabels, nil)
		if mergedSel.MatchLabels["env"] != "prod" {
			t.Errorf("expected env=prod (retained), got %s", mergedSel.MatchLabels["env"])
		}
		if mergedSel.MatchLabels["tier"] != "backend" {
			t.Errorf("expected tier=backend (merged), got %s", mergedSel.MatchLabels["tier"])
		}
	})

	t.Run("mergeFromPod", func(t *testing.T) {
		baseMapping := []monitoringv1.LabelMapping{{From: "app"}}
		extraMapping := []monitoringv1.LabelMapping{{From: "service", To: "app"}, {From: "version"}} // To: app conflicts.

		mergedMap := mergeFromPod(logger, baseMapping, extraMapping)
		if len(mergedMap) != 2 {
			t.Fatalf("expected 2 merged mappings (collision skipped), got %d", len(mergedMap))
		}
		if diff := cmp.Diff(baseMapping[0], mergedMap[0]); diff != "" {
			t.Errorf("expected first mapping untouched: %s", diff)
		}
		if mergedMap[1].From != "version" {
			t.Errorf("expected second mapping from=version, got %s", mergedMap[1].From)
		}
	})
}
