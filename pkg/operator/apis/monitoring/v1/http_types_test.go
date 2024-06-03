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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/secrets"
)

type secretNamespacePodMonitoringTestCase struct {
	monitoringNamespace string
	expectedNamespace   string
}

type secretNamespaceClusterPodMonitoringTestCase struct {
	secretNamespace   string
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

	testCases := []secretNamespacePodMonitoringTestCase{
		{
			monitoringNamespace: "",
			expectedNamespace:   metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: metav1.NamespaceDefault,
			expectedNamespace:   metav1.NamespaceDefault,
		},
		{
			monitoringNamespace: "foo",
			expectedNamespace:   "foo",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("namespace=%s", tc.monitoringNamespace), func(t *testing.T) {
			p := &PodMonitoring{
				ObjectMeta: metav1.ObjectMeta{Namespace: tc.monitoringNamespace},
			}

			pool := PrometheusSecretConfigs{}
			c := &SecretKeySelector{
				Name: "secret1",
				Key:  "key1",
			}

			ref, err := c.toPrometheusSecretRef(p, pool)
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

	testCases := []secretNamespaceClusterPodMonitoringTestCase{
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
