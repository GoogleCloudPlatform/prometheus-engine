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
	"fmt"
	"log/slog"
	"os"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/google/export"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

func newTestConversionContext() *conversionContext {
	return &conversionContext{
		logger:          slog.New(slog.NewTextHandler(os.Stdout, nil)),
		cache:           NewResourceCache(),
		sourceNamespace: "default",
		targetNamespace: "default",
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

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	if err != nil {
		return fmt.Errorf("failed to convert Secret %s/%s to unstructured: %w", namespace, name, err)
	}
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

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	if err != nil {
		return fmt.Errorf("failed to convert ConfigMap %s/%s to unstructured: %w", namespace, name, err)
	}
	return cache.Add(&unstructured.Unstructured{Object: u})
}

func TestExtractSecretKey(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(cache *ResourceCache) error
		selector    corev1.SecretKeySelector
		expectedVal string
		expectTodos int
		wantErr     bool
	}{
		{
			name:        "Missing secret",
			setupCache:  func(_ *ResourceCache) error { return nil }, // Empty cache.
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "TODO_SET_USER_FROM_SECRET_MISSING",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name: "Secret with StringData",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret", "user", "admin", true)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "user"},
			expectedVal: "admin",
			expectTodos: 0,
			wantErr:     false,
		},
		{
			name: "Secret with Base64 Data",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret-2", "pass", "supersecret", false)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret-2"}, Key: "pass"},
			expectedVal: "supersecret",
			expectTodos: 0,
			wantErr:     false,
		},
		{
			name:        "Secret reference with empty Name",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.SecretKeySelector{Key: "user"},
			expectedVal: "TODO_SET_USER_FROM_SECRET_EMPTY_NAME",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name:        "Secret reference with empty Key",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
			expectedVal: "TODO_SET_EMPTY_KEY_FROM_SECRET_MY-SECRET",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name: "Secret exists but key missing",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "my-secret", "user", "admin", true)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "password"},
			expectedVal: "TODO_MISSING_KEY_PASSWORD_IN_SECRET_MY-SECRET",
			expectTodos: 1,
			wantErr:     false,
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
			expectedVal: "TODO_CORRUPT_SECRET_DATA_PASS",
			expectTodos: 1,
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			val := ctx.extractSecretKey(tc.selector)
			if val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
			if len(ctx.todos) != tc.expectTodos {
				t.Errorf("expected %d todos, got %d", tc.expectTodos, len(ctx.todos))
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
		expectTodos int
		wantErr     bool
	}{
		{
			name:        "Missing configmap",
			setupCache:  func(_ *ResourceCache) error { return nil }, // Empty cache.
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "TODO_SET_USER_FROM_CONFIGMAP_MISSING",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name: "Found configmap",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "my-cm", "id", "client-123")
			},
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "id"},
			expectedVal: "client-123",
			expectTodos: 0,
			wantErr:     false,
		},
		{
			name:        "Configmap reference with empty Name",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.ConfigMapKeySelector{Key: "user"},
			expectedVal: "TODO_SET_USER_FROM_CONFIGMAP_EMPTY_NAME",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name:        "Configmap reference with empty Key",
			setupCache:  func(_ *ResourceCache) error { return nil },
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}},
			expectedVal: "TODO_SET_EMPTY_KEY_FROM_CONFIGMAP_MY-CM",
			expectTodos: 1,
			wantErr:     false,
		},
		{
			name: "Configmap exists but key missing",
			setupCache: func(cache *ResourceCache) error {
				return addConfigMapToCache(cache, "default", "my-cm", "id", "client-123")
			},
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "secret"},
			expectedVal: "TODO_MISSING_KEY_SECRET_IN_CONFIGMAP_MY-CM",
			expectTodos: 1,
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			val := ctx.extractConfigMapKey(tc.selector)
			if val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
			if len(ctx.todos) != tc.expectTodos {
				t.Errorf("expected %d todos, got %d", tc.expectTodos, len(ctx.todos))
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
			expectedSecretName:    "secret-TODO_SET_CONFIGMAP_NAME",
			expectedSecretKey:     "ca.crt",
			expectGeneratedSecret: false,
			wantErr:               false,
		},
		{
			name:                  "Empty key reference",
			setupCache:            func(_ *ResourceCache) error { return nil },
			selector:              &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"}},
			expectedSecretName:    "secret-tls-cm",
			expectedSecretKey:     "TODO_SET_CONFIGMAP_KEY",
			expectGeneratedSecret: false,
			wantErr:               false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpSel := ctx.convertConfigMapToSecretSelector(tc.selector)

			if tc.selector == nil {
				if gmpSel != nil {
					t.Errorf("expected nil result for nil selector, got %+v", gmpSel)
				}
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
			name: "Malformed Username reference",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "auth-secret", "pass", "pass123", true)
			},
			basicAuth: &pomonitoringv1.BasicAuth{
				Username: corev1.SecretKeySelector{Key: "user"},
				Password: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "pass"},
			},
			expectedUser:     "TODO_SET_USER_FROM_SECRET_EMPTY_NAME",
			expectedPassName: "auth-secret",
			expectedPassKey:  "pass",
			wantErr:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpBA := ctx.convertBasicAuth(tc.basicAuth)

			if tc.basicAuth == nil {
				if gmpBA != nil {
					t.Errorf("expected nil result for nil BasicAuth, got %+v", gmpBA)
				}
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
			expectedCAName: "secret-TODO_SET_CONFIGMAP_NAME",
			expectedCAKey:  "ca.crt",
			wantErr:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			gmpTLS := ctx.convertSafeTLSConfig(tc.tlsConfig)

			if tc.tlsConfig == nil {
				if gmpTLS != nil {
					t.Errorf("expected nil result for nil TLS config, got %+v", gmpTLS)
				}
				return
			}

			if tc.expectedCAName != "" {
				if gmpTLS.CA == nil || gmpTLS.CA.Secret == nil || gmpTLS.CA.Secret.Name != tc.expectedCAName || gmpTLS.CA.Secret.Key != tc.expectedCAKey {
					t.Errorf("unexpected CA selector: %+v", gmpTLS.CA)
				}
			}
			if tc.expectedCertName != "" {
				if gmpTLS.Cert == nil || gmpTLS.Cert.Secret == nil || gmpTLS.Cert.Secret.Name != tc.expectedCertName || gmpTLS.Cert.Secret.Key != tc.expectedCertKey {
					t.Errorf("unexpected Cert selector: %+v", gmpTLS.Cert)
				}
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

func TestConvertOAuth2(t *testing.T) {
	tests := []struct {
		name             string
		setupCache       func(cache *ResourceCache) error
		oauth2           *pomonitoringv1.OAuth2
		expectedClientID string
		expectedSecName  string
		expectedSecKey   string
		expectedTokenURL string
		expectTodos      int
	}{
		{
			name:       "Nil OAuth2 returns nil",
			setupCache: func(_ *ResourceCache) error { return nil },
			oauth2:     nil,
		},
		{
			name: "Valid OAuth2 from Secret",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "oauth-sec", "client_id", "my-client", true)
			},
			oauth2: &pomonitoringv1.OAuth2{
				ClientID: pomonitoringv1.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-sec"},
						Key:                  "client_id",
					},
				},
				ClientSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-sec"},
					Key:                  "client_secret",
				},
				TokenURL: "https://auth.example.com/token",
			},
			expectedClientID: "my-client",
			expectedSecName:  "oauth-sec",
			expectedSecKey:   "client_secret",
			expectedTokenURL: "https://auth.example.com/token",
			expectTodos:      0,
		},
		{
			name:       "Empty ClientID generates placeholder and TODO",
			setupCache: func(_ *ResourceCache) error { return nil },
			oauth2: &pomonitoringv1.OAuth2{
				ClientSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-sec"},
					Key:                  "client_secret",
				},
				TokenURL: "https://auth.example.com/token",
			},
			expectedClientID: "TODO_SET_OAUTH2_CLIENT_ID",
			expectedSecName:  "oauth-sec",
			expectedSecKey:   "client_secret",
			expectedTokenURL: "https://auth.example.com/token",
			expectTodos:      1,
		},
		{
			name: "Empty TokenURL generates placeholder and TODO",
			setupCache: func(cache *ResourceCache) error {
				return addSecretToCache(cache, "default", "oauth-sec", "client_id", "my-client", true)
			},
			oauth2: &pomonitoringv1.OAuth2{
				ClientID: pomonitoringv1.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-sec"},
						Key:                  "client_id",
					},
				},
				ClientSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-sec"},
					Key:                  "client_secret",
				},
				TokenURL: "",
			},
			expectedClientID: "my-client",
			expectedSecName:  "oauth-sec",
			expectedSecKey:   "client_secret",
			expectedTokenURL: "TODO_SET_OAUTH2_TOKEN_URL",
			expectTodos:      1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			if err := tc.setupCache(ctx.cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			res := ctx.convertOAuth2(tc.oauth2)
			if tc.oauth2 == nil {
				if res != nil {
					t.Errorf("expected nil result for nil OAuth2, got %+v", res)
				}
				return
			}
			if res.ClientID != tc.expectedClientID {
				t.Errorf("expected ClientID %q, got %q", tc.expectedClientID, res.ClientID)
			}
			if res.TokenURL != tc.expectedTokenURL {
				t.Errorf("expected TokenURL %q, got %q", tc.expectedTokenURL, res.TokenURL)
			}
			if res.ClientSecret.Secret == nil || res.ClientSecret.Secret.Name != tc.expectedSecName || res.ClientSecret.Secret.Key != tc.expectedSecKey {
				t.Errorf("unexpected ClientSecret selector: %+v", res.ClientSecret)
			}
			if len(ctx.todos) != tc.expectTodos {
				t.Errorf("expected %d todos, got %d", tc.expectTodos, len(ctx.todos))
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
	gmpSel1 := ctx.convertConfigMapToSecretSelector(selector)
	if gmpSel1 == nil || gmpSel1.Secret.Name != "secret-tls-cm" {
		t.Fatal("first call failed to translate selector")
	}

	// Call second time.
	gmpSel2 := ctx.convertConfigMapToSecretSelector(selector)
	if gmpSel2 == nil || gmpSel2.Secret.Name != "secret-tls-cm" {
		t.Fatal("second call failed to translate selector")
	}

	// Ensure only one secret was generated in total.
	genSecrets := ctx.getGeneratedSecrets()
	if len(genSecrets) != 1 {
		t.Fatalf("expected exactly 1 generated secret due to duplication, got %d", len(genSecrets))
	}
}

func TestMergeFromPod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	staticTargetLabels := []monitoringv1.LabelMapping{
		{From: "app", To: "application"},
		{From: "env"},
	}

	relabelFromPod := []monitoringv1.LabelMapping{
		{From: "env"}, // Duplicate, should be deduplicated.
		{From: "tier"},
	}

	merged := mergeFromPod(logger, staticTargetLabels, relabelFromPod)

	expected := []monitoringv1.LabelMapping{
		{From: "env"},
		{From: "tier"},
		{From: "app", To: "application"},
	}

	if diff := cmp.Diff(expected, merged); diff != "" {
		t.Errorf("mergeFromPod mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertLimits(t *testing.T) {
	tests := []struct {
		name                  string
		sampleLimit           *uint64
		labelLimit            *uint64
		labelNameLengthLimit  *uint64
		labelValueLengthLimit *uint64
		expected              *monitoringv1.ScrapeLimits
	}{
		{
			name:     "All nil inputs return nil",
			expected: nil,
		},
		{
			name:        "Explicit zero value returns nil as zero values are omitted by omitempty",
			sampleLimit: ptrTo(uint64(0)),
			expected:    nil,
		},
		{
			name:                  "Non-zero limits are converted",
			sampleLimit:           ptrTo(uint64(5000)),
			labelLimit:            ptrTo(uint64(50)),
			labelNameLengthLimit:  ptrTo(uint64(100)),
			labelValueLengthLimit: ptrTo(uint64(200)),
			expected: &monitoringv1.ScrapeLimits{
				Samples:          5000,
				Labels:           50,
				LabelNameLength:  100,
				LabelValueLength: 200,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convertLimits(tc.sampleLimit, tc.labelLimit, tc.labelNameLengthLimit, tc.labelValueLengthLimit)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("convertLimits() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDetermineNamespaceScoping(t *testing.T) {
	tests := []struct {
		name              string
		nsSel             pomonitoringv1.NamespaceSelector
		defaultNS         string
		expectedNS        []string
		expectedIsCluster bool
		expectErr         bool
	}{
		{
			name:              "any namespace true",
			nsSel:             pomonitoringv1.NamespaceSelector{Any: true},
			defaultNS:         "default",
			expectedNS:        nil,
			expectedIsCluster: true,
			expectErr:         false,
		},
		{
			name:              "specific matchNames",
			nsSel:             pomonitoringv1.NamespaceSelector{MatchNames: []string{"ns1", "ns2"}},
			defaultNS:         "default",
			expectedNS:        []string{"ns1", "ns2"},
			expectedIsCluster: false,
			expectErr:         false,
		},
		{
			name:              "empty matchNames fallback to default",
			nsSel:             pomonitoringv1.NamespaceSelector{},
			defaultNS:         "default",
			expectedNS:        []string{"default"},
			expectedIsCluster: false,
			expectErr:         false,
		},
		{
			name:      "invalid matchNames",
			nsSel:     pomonitoringv1.NamespaceSelector{MatchNames: []string{""}},
			defaultNS: "default",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns, isCluster, err := determineNamespaceScoping(tc.nsSel, tc.defaultNS)
			if (err != nil) != tc.expectErr {
				t.Fatalf("determineNamespaceScoping() error = %v, expectErr = %v", err, tc.expectErr)
			}
			if tc.expectErr {
				return
			}
			if isCluster != tc.expectedIsCluster {
				t.Errorf("determineNamespaceScoping() isCluster = %v, want %v", isCluster, tc.expectedIsCluster)
			}
			if diff := cmp.Diff(tc.expectedNS, ns); diff != "" {
				t.Errorf("determineNamespaceScoping() namespaces mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveScrapeIntervalAndTimeout(t *testing.T) {
	tests := []struct {
		name            string
		interval        string
		timeout         string
		expectedInt     string
		expectedTimeout string
		expectTodos     int
	}{
		{
			name:            "empty defaults to 30s",
			interval:        "",
			timeout:         "",
			expectedInt:     "30s",
			expectedTimeout: "",
			expectTodos:     0,
		},
		{
			name:            "valid interval and timeout",
			interval:        "15s",
			timeout:         "10s",
			expectedInt:     "15s",
			expectedTimeout: "10s",
			expectTodos:     0,
		},
		{
			name:            "timeout larger than interval is capped",
			interval:        "10s",
			timeout:         "20s",
			expectedInt:     "10s",
			expectedTimeout: "10s",
			expectTodos:     0,
		},
		{
			name:            "invalid interval duration defaults to 30s with todo",
			interval:        "invalid",
			expectedInt:     "30s",
			expectedTimeout: "",
			expectTodos:     1,
		},
		{
			name:            "invalid timeout duration is dropped with todo",
			interval:        "15s",
			timeout:         "invalid",
			expectedInt:     "15s",
			expectedTimeout: "",
			expectTodos:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			intVal, toVal := ctx.resolveScrapeIntervalAndTimeout(tc.interval, tc.timeout)
			if intVal != tc.expectedInt {
				t.Errorf("resolveScrapeIntervalAndTimeout() interval = %v, want %v", intVal, tc.expectedInt)
			}
			if toVal != tc.expectedTimeout {
				t.Errorf("resolveScrapeIntervalAndTimeout() timeout = %v, want %v", toVal, tc.expectedTimeout)
			}
			if len(ctx.todos) != tc.expectTodos {
				t.Errorf("expected %d todos, got %d", tc.expectTodos, len(ctx.todos))
			}
		})
	}
}

func TestConvertProxyURL(t *testing.T) {
	tests := []struct {
		name        string
		proxyURL    *string
		expectedURL string
		expectTodos int
		expectErr   bool
	}{
		{
			name:        "nil proxyURL",
			proxyURL:    nil,
			expectedURL: "",
			expectTodos: 0,
			expectErr:   false,
		},
		{
			name:        "valid proxyURL without credentials",
			proxyURL:    ptrTo("http://proxy.example.com"),
			expectedURL: "http://proxy.example.com",
			expectTodos: 0,
			expectErr:   false,
		},
		{
			name:        "proxyURL with credentials sanitizes password and adds todo",
			proxyURL:    ptrTo("http://user:pass@proxy.example.com:8080"),
			expectedURL: "http://proxy.example.com:8080",
			expectTodos: 1,
		},
		{
			name:        "malformed proxyURL returns placeholder and adds todo",
			proxyURL:    ptrTo("://invalid-url"),
			expectedURL: "TODO_SET_VALID_PROXY_URL",
			expectTodos: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			convCtx := &conversionContext{}
			url := convCtx.convertProxyURL(tc.proxyURL)
			if url != tc.expectedURL {
				t.Errorf("convertProxyURL() = %v, want %v", url, tc.expectedURL)
			}
			if len(convCtx.todos) != tc.expectTodos {
				t.Errorf("expected %d todos, got %d", tc.expectTodos, len(convCtx.todos))
			}
		})
	}
}

func TestResolveFilterRunning(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tests := []struct {
		name           string
		filterRunnings []*bool
		isCluster      bool
		expected       *bool
	}{
		{
			name:           "empty cluster monitor defaults to nil",
			filterRunnings: nil,
			isCluster:      true,
			expected:       nil,
		},
		{
			name:           "empty namespaced monitor defaults to nil",
			filterRunnings: nil,
			isCluster:      false,
			expected:       nil,
		},
		{
			name:           "all true resolves to nil (GMP default)",
			filterRunnings: []*bool{ptrTo(true), ptrTo(true)},
			isCluster:      false,
			expected:       nil,
		},
		{
			name:           "all false resolves to false",
			filterRunnings: []*bool{ptrTo(false), ptrTo(false)},
			isCluster:      false,
			expected:       ptrTo(false),
		},
		{
			name:           "mixed true and false resolves to false",
			filterRunnings: []*bool{ptrTo(true), ptrTo(false)},
			isCluster:      false,
			expected:       ptrTo(false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveFilterRunning(tc.filterRunnings, logger, tc.isCluster)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("resolveFilterRunning() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveAttachMetadata(t *testing.T) {
	tests := []struct {
		name           string
		attachMetadata *pomonitoringv1.AttachMetadata
		base           *[]string
		isCluster      bool
		expected       *[]string
	}{
		{
			name:           "nil attachMetadata returns base",
			attachMetadata: nil,
			base:           &[]string{labelPod},
			isCluster:      false,
			expected:       &[]string{labelPod},
		},
		{
			name:           "attachMetadata node false returns base",
			attachMetadata: &pomonitoringv1.AttachMetadata{Node: ptrTo(false)},
			base:           nil,
			isCluster:      false,
			expected:       nil,
		},
		{
			name:           "namespaced monitor with nil base returns node",
			attachMetadata: &pomonitoringv1.AttachMetadata{Node: ptrTo(true)},
			base:           nil,
			isCluster:      false,
			expected:       &[]string{labelContainer, labelNode, labelPod, labelTopLevelControllerName, labelTopLevelControllerType},
		},
		{
			name:           "cluster monitor with nil base returns node plus cluster defaults",
			attachMetadata: &pomonitoringv1.AttachMetadata{Node: ptrTo(true)},
			base:           nil,
			isCluster:      true,
			expected:       &[]string{labelContainer, export.KeyNamespace, labelNode, labelPod, labelTopLevelControllerName, labelTopLevelControllerType},
		},
		{
			name:           "namespaced monitor with existing base appends node",
			attachMetadata: &pomonitoringv1.AttachMetadata{Node: ptrTo(true)},
			base:           &[]string{labelPod},
			isCluster:      false,
			expected:       &[]string{labelNode, labelPod},
		},
		{
			name:           "namespaced monitor with node already present does not duplicate",
			attachMetadata: &pomonitoringv1.AttachMetadata{Node: ptrTo(true)},
			base:           &[]string{labelNode, labelPod},
			isCluster:      false,
			expected:       &[]string{labelNode, labelPod},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAttachMetadata(tc.attachMetadata, tc.base, tc.isCluster)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("resolveAttachMetadata() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToStrictUnstructured(t *testing.T) {
	tests := []struct {
		name string
		obj  any
	}{
		{
			name: "scrape limits with uint64 converts to int64",
			obj: &monitoringv1.ScrapeLimits{
				Samples: 5000,
				Labels:  100,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := toStrictUnstructured(tc.obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Verify uint64 fields are converted to int64.
			if val, ok := u["samples"]; ok {
				if _, isInt64 := val.(int64); !isInt64 {
					t.Errorf("expected samples to be int64, got %T", val)
				}
			}
			// Verify DeepCopy does not panic.
			unstruct := &unstructured.Unstructured{Object: u}
			_ = unstruct.DeepCopy()
		})
	}
}

func TestDecoupledNamespaces(t *testing.T) {
	ctx := newTestConversionContext()
	ctx.sourceNamespace = "source-ns"
	ctx.targetNamespace = "target-ns"
	ctx.isClusterScoped = true

	// Verify that secret extraction reads from sourceNamespace.
	if err := addSecretToCache(ctx.cache, "source-ns", "my-secret", "user", "admin", true); err != nil {
		t.Fatalf("failed to add secret to cache: %v", err)
	}
	sel := corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
		Key:                  "user",
	}
	val := ctx.extractSecretKey(sel)
	if val != "admin" {
		t.Errorf("extractSecretKey() = %q, want %q", val, "admin")
	}

	// Verify that ConfigMap to Secret conversion reads from sourceNamespace and generates Secret in targetNamespace.
	if err := addConfigMapToCache(ctx.cache, "source-ns", "tls-cm", "ca.crt", "cert-data"); err != nil {
		t.Fatalf("failed to add configmap to cache: %v", err)
	}
	cmSel := &corev1.ConfigMapKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"},
		Key:                  "ca.crt",
	}
	secretSel := ctx.convertConfigMapToSecretSelector(cmSel)
	if secretSel.Secret.Namespace != "target-ns" {
		t.Errorf("expected selector namespace %q, got %q", "target-ns", secretSel.Secret.Namespace)
	}
	genSecrets := ctx.getGeneratedSecrets()
	if len(genSecrets) != 1 {
		t.Fatalf("expected 1 generated secret, got %d", len(genSecrets))
	}
	if genSecrets[0].GetNamespace() != "target-ns" {
		t.Errorf("expected generated secret namespace %q, got %q", "target-ns", genSecrets[0].GetNamespace())
	}
}
func TestFindServicesBySelector(t *testing.T) {
	tests := []struct {
		name       string
		setupCache func(cache *ResourceCache) error
		selector   metav1.LabelSelector
		namespaces []string
		expected   []string
		wantErr    bool
	}{
		{
			name: "Match single service by label",
			setupCache: func(cache *ResourceCache) error {
				return addServiceToCache(t, cache, "default", "svc-a", map[string]string{"app": "foo"})
			},
			selector:   metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			namespaces: []string{"default"},
			expected:   []string{"svc-a"},
			wantErr:    false,
		},
		{
			name: "Match multiple services",
			setupCache: func(cache *ResourceCache) error {
				if err := addServiceToCache(t, cache, "default", "svc-b", map[string]string{"app": "foo", "env": "prod"}); err != nil {
					return err
				}
				return addServiceToCache(t, cache, "default", "svc-a", map[string]string{"app": "foo"})
			},
			selector:   metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			namespaces: []string{"default"},
			expected:   []string{"svc-a", "svc-b"},
			wantErr:    false,
		},
		{
			name: "Filter by namespace",
			setupCache: func(cache *ResourceCache) error {
				if err := addServiceToCache(t, cache, "ns-a", "svc-a", map[string]string{"app": "foo"}); err != nil {
					return err
				}
				return addServiceToCache(t, cache, "ns-b", "svc-b", map[string]string{"app": "foo"})
			},
			selector:   metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			namespaces: []string{"ns-a"},
			expected:   []string{"svc-a"},
			wantErr:    false,
		},
		{
			name: "Match any namespace",
			setupCache: func(cache *ResourceCache) error {
				if err := addServiceToCache(t, cache, "ns-a", "svc-a", map[string]string{"app": "foo"}); err != nil {
					return err
				}
				return addServiceToCache(t, cache, "ns-b", "svc-b", map[string]string{"app": "foo"})
			},
			selector:   metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			namespaces: nil,
			expected:   []string{"svc-a", "svc-b"},
			wantErr:    false,
		},
		{
			name: "No match",
			setupCache: func(cache *ResourceCache) error {
				return addServiceToCache(t, cache, "default", "svc-a", map[string]string{"app": "bar"})
			},
			selector:   metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			namespaces: []string{"default"},
			expected:   nil,
			wantErr:    false,
		},
		{
			name:       "Invalid selector",
			setupCache: func(_ *ResourceCache) error { return nil },
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "InvalidOperator",
					},
				},
			},
			namespaces: []string{"default"},
			expected:   nil,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewResourceCache()
			if err := tc.setupCache(cache); err != nil {
				t.Fatalf("failed to setup cache: %v", err)
			}

			matched, err := cache.findServicesBySelector(tc.selector, tc.namespaces)
			if (err != nil) != tc.wantErr {
				t.Fatalf("findServicesBySelector() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if len(matched) != len(tc.expected) {
				t.Fatalf("expected %d matches, got %d", len(tc.expected), len(matched))
			}

			for i, svc := range matched {
				if svc.Name != tc.expected[i] {
					t.Errorf("expected matched service at index %d to be %s, got %s", i, tc.expected[i], svc.Name)
				}
			}
		})
	}
}

func TestResolveServicePort(t *testing.T) {
	tests := []struct {
		name       string
		service    *corev1.Service
		portStr    string
		expected   intstr.IntOrString
		expectTodo bool
	}{
		{
			name: "Resolve by name to int",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
			}),
			portStr:    "web",
			expected:   intstr.FromInt32(8080),
			expectTodo: false,
		},
		{
			name: "Resolve by name to string",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromString("http-web")},
			}),
			portStr:    "web",
			expected:   intstr.FromString("http-web"),
			expectTodo: false,
		},
		{
			name: "Resolve by port number to targetPort int",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
			}),
			portStr:    "80",
			expected:   intstr.FromInt32(8080),
			expectTodo: false,
		},
		{
			name: "Resolve by targetPort string when port name omitted",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromString("http-metrics")},
			}),
			portStr:    "http-metrics",
			expected:   intstr.FromString("http-metrics"),
			expectTodo: false,
		},
		{
			name: "Resolve by port number",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromString("http-web")},
			}),
			portStr:    "80",
			expected:   intstr.FromString("http-web"),
			expectTodo: false,
		},
		{
			name: "Resolve with omitted targetPort",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80},
			}),
			portStr:    "web",
			expected:   intstr.FromInt32(80),
			expectTodo: false,
		},
		{
			name: "Port not found",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80},
			}),
			portStr:    "admin",
			expected:   intstr.FromString("TODO_RESOLVE_PORT_ADMIN"),
			expectTodo: true,
		},
		{
			name:       "Nil service",
			service:    nil,
			portStr:    "web",
			expected:   intstr.FromString("TODO_RESOLVE_PORT"),
			expectTodo: false,
		},
		{
			name: "Skip malformed port entry and resolve valid later entry",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "malformed", Port: 0},
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				},
			},
			portStr:    "web",
			expected:   intstr.FromInt32(8080),
			expectTodo: false,
		},
		{
			name: "All ports malformed returns todo placeholder",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "malformed1", Port: 0},
						{Name: "malformed2", Port: -1},
					},
				},
			},
			portStr:    "web",
			expected:   intstr.FromString("TODO_RESOLVE_PORT_WEB"),
			expectTodo: true,
		},
		{
			name: "Out of range port number is rejected",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "overflow-port", Port: -5},
						{Name: "high-port", Port: 70000},
						{Name: "web", Port: 80, TargetPort: intstr.FromInt32(8080)},
					},
				},
			},
			portStr:    "web",
			expected:   intstr.FromInt32(8080),
			expectTodo: false,
		},
		{
			name: "Resolve with empty string targetPort defaults to port number",
			service: makeTestTypedService("default", "my-svc", nil, []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromString("")},
			}),
			portStr:    "web",
			expected:   intstr.FromInt32(80),
			expectTodo: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, todo := resolveServicePort(logger, tc.service, tc.portStr)
			if (todo != nil) != tc.expectTodo {
				t.Fatalf("resolveServicePort() todo = %v, expectTodo %v", todo, tc.expectTodo)
			}
			if got != tc.expected {
				t.Errorf("expected %+v, got %+v", tc.expected, got)
			}
		})
	}
}

// makeTestTypedService builds a corev1.Service object from labels and ports.
func makeTestTypedService(namespace, name string, labels map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
		},
	}
}

// makeTestService builds a Service Unstructured object from labels and ports, failing the test on conversion error.
func makeTestService(t *testing.T, namespace, name string, labels map[string]string, ports []corev1.ServicePort) *unstructured.Unstructured {
	t.Helper()
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	if err != nil {
		t.Fatalf("failed to convert Service %s/%s to unstructured: %v", namespace, name, err)
	}
	return &unstructured.Unstructured{Object: u}
}

// addServiceToCache creates a test Service and adds it to the resource cache.
func addServiceToCache(t *testing.T, cache *ResourceCache, namespace, name string, labels map[string]string) error {
	t.Helper()
	return cache.Add(makeTestService(t, namespace, name, labels, nil))
}

func TestMakeUniqueResourceName(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		suffix   string
		expected string
	}{
		{
			name:     "Short names unchanged",
			base:     "my-monitor",
			suffix:   "service-a",
			expected: "my-monitor-service-a",
		},
		{
			name:     "Long name truncated with hash",
			base:     "my-very-long-servicemonitor-name-for-production-apps-in-cluster",
			suffix:   "my-service-alpha-backend",
			expected: "my-very-long-servicemonitor-name-for-production-apps-in-f21e5d",
		},
		{
			name:     "Long names identical up to 56 characters do not collide",
			base:     "my-very-long-servicemonitor-name-for-production-apps-in-cluster-b",
			suffix:   "my-service-alpha-backend",
			expected: "my-very-long-servicemonitor-name-for-production-apps-in-b6ef21",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeUniqueResourceName(tc.base, tc.suffix)
			if got != tc.expected {
				t.Errorf("makeUniqueResourceName() = %q, want %q", got, tc.expected)
			}
			if len(got) > validation.DNS1123LabelMaxLength {
				t.Errorf("makeUniqueResourceName() length %d exceeds 63 characters", len(got))
			}
		})
	}
}

func TestAddMigrationTodo(t *testing.T) {
	tests := []struct {
		name        string
		initialObj  map[string]any
		category    string
		reason      string
		action      string
		expectedMap map[string]string
	}{
		{
			name: "single todo with action",
			initialObj: map[string]any{
				"metadata": map[string]any{},
			},
			category: "WARNING",
			reason:   "Dropped unsupported 'annotationMatches' selector.",
			action:   "Verify 'spec.selector.matchLabels' on target pods and remove guardrail label.",
			expectedMap: map[string]string{
				"gmp.googleapis.com/todo-1": "[WARNING] Dropped unsupported 'annotationMatches' selector. ACTION: Verify 'spec.selector.matchLabels' on target pods and remove guardrail label.",
			},
		},
		{
			name: "sequential todos preserving existing annotations",
			initialObj: map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"existing.io/key":           "existing-val",
						"gmp.googleapis.com/todo-1": "[WARNING] First todo. ACTION: Fix first item.",
					},
				},
			},
			category: "ERROR",
			reason:   "Invalid proxy URL.",
			action:   "Move credentials to Secret.",
			expectedMap: map[string]string{
				"existing.io/key":           "existing-val",
				"gmp.googleapis.com/todo-1": "[WARNING] First todo. ACTION: Fix first item.",
				"gmp.googleapis.com/todo-2": "[ERROR] Invalid proxy URL. ACTION: Move credentials to Secret.",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &unstructured.Unstructured{Object: tc.initialObj}
			AddMigrationTodo(u, tc.category, tc.reason, tc.action)
			if diff := cmp.Diff(tc.expectedMap, u.GetAnnotations()); diff != "" {
				t.Errorf("AddMigrationTodo() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInjectSafetyGuardrail(t *testing.T) {
	tests := []struct {
		name          string
		initialObj    map[string]any
		expectedMatch map[string]string
	}{
		{
			name: "inject into empty matchLabels",
			initialObj: map[string]any{
				"spec": map[string]any{},
			},
			expectedMatch: map[string]string{
				"gmp.googleapis.com/migration-review-required": "true",
			},
		},
		{
			name: "preserve existing matchLabels",
			initialObj: map[string]any{
				"spec": map[string]any{
					"selector": map[string]any{
						"matchLabels": map[string]any{
							"app": "frontend",
						},
					},
				},
			},
			expectedMatch: map[string]string{
				"app": "frontend",
				"gmp.googleapis.com/migration-review-required": "true",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &unstructured.Unstructured{Object: tc.initialObj}
			if err := InjectSafetyGuardrail(u); err != nil {
				t.Fatalf("InjectSafetyGuardrail() unexpected error: %v", err)
			}
			gotMatch, _, _ := unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
			if diff := cmp.Diff(tc.expectedMatch, gotMatch); diff != "" {
				t.Errorf("InjectSafetyGuardrail() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
