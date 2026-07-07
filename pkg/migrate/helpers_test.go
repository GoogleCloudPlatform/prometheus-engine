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
	"testing"

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

func addSecretToCache(cache *ResourceCache, namespace, name, key, value string, isStringData bool) {
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
	cache.Add(&unstructured.Unstructured{Object: u})
}

func addConfigMapToCache(cache *ResourceCache, namespace, name, key, value string) {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{key: value},
	}

	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	cache.Add(&unstructured.Unstructured{Object: u})
}

func TestExtractSecretKey(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(cache *ResourceCache)
		selector    corev1.SecretKeySelector
		expectedVal string
	}{
		{
			name:        "Missing secret",
			setupCache:  func(cache *ResourceCache) {}, // Empty cache
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "<MISSING_SECRET_missing_KEY_user>",
		},
		{
			name: "Secret with StringData",
			setupCache: func(cache *ResourceCache) {
				addSecretToCache(cache, "default", "my-secret", "user", "admin", true)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "user"},
			expectedVal: "admin",
		},
		{
			name: "Secret with Base64 Data",
			setupCache: func(cache *ResourceCache) {
				addSecretToCache(cache, "default", "my-secret-2", "pass", "supersecret", false)
			},
			selector:    corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret-2"}, Key: "pass"},
			expectedVal: "supersecret",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			tc.setupCache(ctx.cache)

			val := extractSecretKey(ctx, tc.selector)
			if val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
		})
	}
}

func TestExtractConfigMapKey(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(cache *ResourceCache)
		selector    corev1.ConfigMapKeySelector
		expectedVal string
	}{
		{
			name:        "Missing configmap",
			setupCache:  func(cache *ResourceCache) {}, // Empty cache
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "user"},
			expectedVal: "<MISSING_CONFIGMAP_missing_KEY_user>",
		},
		{
			name: "Found configmap",
			setupCache: func(cache *ResourceCache) {
				addConfigMapToCache(cache, "default", "my-cm", "id", "client-123")
			},
			selector:    corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "id"},
			expectedVal: "client-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			tc.setupCache(ctx.cache)

			val := extractConfigMapKey(ctx, tc.selector)
			if val != tc.expectedVal {
				t.Errorf("expected %s, got %s", tc.expectedVal, val)
			}
		})
	}
}

func TestConvertConfigMapToSecretSelector(t *testing.T) {
	tests := []struct {
		name                  string
		setupCache            func(cache *ResourceCache)
		selector              *corev1.ConfigMapKeySelector
		expectedSecretName    string
		expectedSecretKey     string
		expectGeneratedSecret bool
	}{
		{
			name: "Convert ConfigMap to Secret",
			setupCache: func(cache *ResourceCache) {
				addConfigMapToCache(cache, "default", "tls-cm", "ca.crt", "cert-data")
			},
			selector: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "tls-cm"},
				Key:                  "ca.crt",
			},
			expectedSecretName:    "secret-tls-cm",
			expectedSecretKey:     "ca.crt",
			expectGeneratedSecret: true,
		},
		{
			name:                  "Nil selector",
			setupCache:            func(cache *ResourceCache) {},
			selector:              nil,
			expectedSecretName:    "",
			expectedSecretKey:     "",
			expectGeneratedSecret: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			tc.setupCache(ctx.cache)

			gmpSel := convertConfigMapToSecretSelector(ctx, tc.selector)

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
				if len(ctx.generatedSecrets) != 1 {
					t.Fatalf("expected 1 generated secret, got %d", len(ctx.generatedSecrets))
				}
				gen := ctx.generatedSecrets[0]
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
		setupCache       func(cache *ResourceCache)
		basicAuth        *pomonitoringv1.BasicAuth
		expectedUser     string
		expectedPassName string
		expectedPassKey  string
	}{
		{
			name: "Valid BasicAuth conversion",
			setupCache: func(cache *ResourceCache) {
				addSecretToCache(cache, "default", "auth-secret", "user", "myuser", true)
			},
			basicAuth: &pomonitoringv1.BasicAuth{
				Username: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "user"},
				Password: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"}, Key: "pass"},
			},
			expectedUser:     "myuser",
			expectedPassName: "auth-secret",
			expectedPassKey:  "pass",
		},
		{
			name:       "Nil BasicAuth",
			setupCache: func(cache *ResourceCache) {},
			basicAuth:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			tc.setupCache(ctx.cache)

			gmpBA := convertBasicAuth(ctx, tc.basicAuth)

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
		setupCache         func(cache *ResourceCache)
		tlsConfig          *pomonitoringv1.SafeTLSConfig
		expectedCAName     string
		expectedCAKey      string
		expectedCertName   string
		expectedCertKey    string
		expectedSkipVerify bool
		expectedServerName string
	}{
		{
			name: "Full TLS Config Conversion",
			setupCache: func(cache *ResourceCache) {
				addConfigMapToCache(cache, "default", "ca-cm", "ca.crt", "ca-data")
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
		},
		{
			name:       "Nil TLS Config",
			setupCache: func(cache *ResourceCache) {},
			tlsConfig:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestConversionContext()
			tc.setupCache(ctx.cache)

			gmpTLS := convertSafeTLSConfig(ctx, tc.tlsConfig)

			if tc.tlsConfig == nil {
				if gmpTLS != nil {
					t.Errorf("expected nil result for nil TLS config, got %+v", gmpTLS)
				}
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
