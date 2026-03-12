// Copyright 2024 Google LLC
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

package v1

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/common/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/prometheus/prometheus/google/secrets"
)

type secretNamespaceTestCase struct {
	// Is empty when testing for cluster-scoped resources.
	monitoringNamespace string
	secretNamespace     string
	// Is empty when an error is expected.
	expectedNamespace string
}

func TestClusterSecretKeySelector_toPrometheusSecretRef_PodMonitoring(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		p := &PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{Namespace: "foo"},
		}
		pool := PrometheusSecretConfigs{}
		var c *SecretKeySelector

		ref, err := c.toPrometheusSecretRef(p, pool)
		if err != nil {
			t.Fatalf("unexpected failure: %s", err)
		}
		if ref != "" {
			t.Fatalf("expected empty ref, got %v", ref)
		}
		if len(pool.SecretConfigs()) != 0 {
			t.Fatalf("expected no configs, got %v", pool.SecretConfigs())
		}
	})

	testCases := []secretNamespaceTestCase{
		{
			monitoringNamespace: "",
			secretNamespace:     "",
			expectedNamespace:   metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: metav1.NamespaceDefault,
			secretNamespace:     "",
			expectedNamespace:   metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: "",
			secretNamespace:     metav1.NamespaceDefault,
			expectedNamespace:   metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: "foo",
			secretNamespace:     metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: "foo",
			secretNamespace:     "",
			expectedNamespace:   "foo",
		},
		{
			monitoringNamespace: "foo",
			secretNamespace:     "foo",
			expectedNamespace:   "foo",
		},
		{
			monitoringNamespace: "foo",
			secretNamespace:     "different",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("namespace=%s,secret=%s", tc.monitoringNamespace, tc.secretNamespace), func(t *testing.T) {
			// Enforcing K8s default namespace for `GetNamespace()` consistency.
			monitoringNamespace := tc.monitoringNamespace
			if tc.monitoringNamespace == "" {
				monitoringNamespace = metav1.NamespaceDefault
			}

			p := &PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{Namespace: monitoringNamespace},
			}

			pool := PrometheusSecretConfigs{}
			c := &SecretKeySelector{
				Name:      "secret1",
				Key:       "key1",
				Namespace: tc.secretNamespace,
			}

			ref, err := c.toPrometheusSecretRef(p, pool)
			if tc.expectedNamespace == "" {
				if err == nil {
					t.Fatal("expected failure, got nil")
				}
				if len(pool.SecretConfigs()) != 0 {
					t.Fatalf("expected no configs, got %v", pool.SecretConfigs())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected failure: %s", err)
			}

			expectedName := fmt.Sprintf("%s/secret1/key1", tc.expectedNamespace)
			if exp, got := expectedName, ref; exp != got {
				t.Fatalf("expected ref %s, got %s", exp, got)
			}
			if exp, got := []secrets.SecretConfig{
				{
					Name: expectedName,
					Config: secrets.KubernetesSecretConfig{
						Name:      c.Name,
						Key:       c.Key,
						Namespace: tc.expectedNamespace,
					},
				},
			}, pool.SecretConfigs(); cmp.Diff(exp, got) != "" {
				t.Fatalf("unexpected secret configs; diff: %v", cmp.Diff(exp, got))
			}
		})
	}
}

func TestClusterSecretKeySelector_toPrometheusSecretRef_ClusterPodMonitoring(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		p := &ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{},
		}
		pool := PrometheusSecretConfigs{}
		var c *SecretKeySelector

		ref, err := c.toPrometheusSecretRef(p, pool)
		if err != nil {
			t.Fatalf("unexpected failure: %s", err)
		}
		if ref != "" {
			t.Fatalf("expected empty ref, got %v", ref)
		}
		if len(pool.SecretConfigs()) != 0 {
			t.Fatalf("expected no configs, got %v", pool.SecretConfigs())
		}
	})

	testCases := []secretNamespaceTestCase{
		{
			secretNamespace:   "",
			expectedNamespace: metav1.NamespaceDefault,
		},
		{
			secretNamespace:   metav1.NamespaceDefault,
			expectedNamespace: metav1.NamespaceDefault,
		},
		{
			secretNamespace:   "foo",
			expectedNamespace: "foo",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("secret=%s", tc.secretNamespace), func(t *testing.T) {
			p := &ClusterPodMonitoring{
				ObjectMeta: metav1.ObjectMeta{},
			}

			pool := PrometheusSecretConfigs{}
			c := &SecretKeySelector{
				Name:      "secret1",
				Key:       "key1",
				Namespace: tc.secretNamespace,
			}

			ref, err := c.toPrometheusSecretRef(p, pool)
			if tc.expectedNamespace == "" {
				if err == nil {
					t.Fatal("expected failure, got nil")
				}
				if len(pool.SecretConfigs()) != 0 {
					t.Fatalf("expected no configs, got %v", pool.SecretConfigs())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected failure: %s", err)
			}

			expectedName := fmt.Sprintf("%s/secret1/key1", tc.expectedNamespace)
			if exp, got := expectedName, ref; exp != got {
				t.Fatalf("expected ref %s, got %s", exp, got)
			}
			if exp, got := []secrets.SecretConfig{
				{
					Name: expectedName,
					Config: secrets.KubernetesSecretConfig{
						Name:      c.Name,
						Key:       c.Key,
						Namespace: tc.expectedNamespace,
					},
				},
			}, pool.SecretConfigs(); cmp.Diff(exp, got) != "" {
				t.Fatalf("unexpected secret configs; diff: %v", cmp.Diff(exp, got))
			}
		})
	}
}

func TestHTTPClientConfig_ToPrometheusConfig_DefaultServiceAccount(t *testing.T) {
	testCases := []struct {
		desc             string
		crd              PodMonitoringCRD
		httpClientConfig *HTTPClientConfig
		expected         config.HTTPClientConfig
	}{
		{
			desc: "PodMonitoring should have no default service account if omitted",
			crd:  &PodMonitoring{},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.Authorization = nil
				return c
			}(),
		},
		{
			desc: "ClusterPodMonitoring should have no default service account if omitted",
			crd:  &ClusterPodMonitoring{},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.Authorization = nil
				return c
			}(),
		},
		{
			desc: "ClusterPodMonitoring should have default service account if explicitly Bearer",
			crd:  &ClusterPodMonitoring{},
			httpClientConfig: &HTTPClientConfig{
				Authorization: &Auth{
					Type: "Bearer",
				},
			},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.Authorization = &config.Authorization{
					Type:            "Bearer",
					CredentialsFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				}
				return c
			}(),
		},
		{
			desc: "ClusterPodMonitoring should allow custom TLS to override default and keep no service account",
			crd:  &ClusterPodMonitoring{},
			httpClientConfig: &HTTPClientConfig{
				TLS: &TLS{
					InsecureSkipVerify: true,
				},
			},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.Authorization = nil
				c.TLSConfig = config.TLSConfig{
					InsecureSkipVerify: true,
				}
				return c
			}(),
		},
		{
			desc: "BasicAuth should disable default service account token but keep CA",
			crd:  &PodMonitoring{},
			httpClientConfig: &HTTPClientConfig{
				BasicAuth: &BasicAuth{
					Username: "user",
				},
			},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.BasicAuth = &config.BasicAuth{
					Username: "user",
				}
				return c
			}(),
		},
		{
			desc: "OAuth2 should disable default service account token but keep CA",
			crd:  &PodMonitoring{},
			httpClientConfig: &HTTPClientConfig{
				OAuth2: &OAuth2{
					ClientID: "client",
				},
			},
			expected: func() config.HTTPClientConfig {
				c := config.DefaultHTTPClientConfig
				c.OAuth2 = &config.OAuth2{
					ClientID: "client",
				}
				return c
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			h := tc.httpClientConfig
			if h == nil {
				h = &HTTPClientConfig{}
			}
			got, err := h.ToPrometheusConfig(tc.crd, nil)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreUnexported(config.HTTPClientConfig{}, config.Authorization{}, config.TLSConfig{}, config.ProxyConfig{}),
			}
			if diff := cmp.Diff(tc.expected, got, opts...); diff != "" {
				t.Errorf("unexpected config (-want +got):\n%s", diff)
			}
		})
	}
}
