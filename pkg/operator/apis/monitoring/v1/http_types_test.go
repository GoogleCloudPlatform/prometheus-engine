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

func TestClusterSecretKeySelector_toPrometheusSecretRef_PodMonitoring(t *testing.T) {
	p := &PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{Namespace: "foo"},
	}

	t.Run("nil", func(t *testing.T) {
		pool := PrometheusSecretConfigs{}
		var c *KubernetesSecretKeySelector

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

	// Empty or correct namespace.
	for _, ns := range []string{"", p.Namespace} {
		t.Run(fmt.Sprintf("namespace=%v", ns), func(t *testing.T) {
			pool := PrometheusSecretConfigs{}
			c := &KubernetesSecretKeySelector{
				Name:      "secret1",
				Key:       "key1",
				Namespace: ns,
			}

			ref, err := c.toPrometheusSecretRef(p, pool)
			if err != nil {
				t.Fatalf("unexpected failure: %s", err)
			}
			if exp, got := "foo/secret1/key1", ref; exp != got {
				t.Fatalf("expected ref %v, got %v", exp, got)
			}
			if exp, got := []secrets.SecretConfig{
				{
					Name: "foo/secret1/key1", Config: secrets.KubernetesSecretConfig{
						Name:      c.Name,
						Key:       c.Key,
						Namespace: p.Namespace,
					},
				},
			}, pool.SecretConfigs(); cmp.Diff(exp, got) != "" {
				t.Fatalf("unpexpted secret configs; diff: %v", cmp.Diff(exp, got))
			}
		})
	}
	t.Run("wrong namespace", func(t *testing.T) {
		pool := PrometheusSecretConfigs{}
		c := &KubernetesSecretKeySelector{
			Name:      "secret1",
			Key:       "key1",
			Namespace: "different",
		}

		if _, err := c.toPrometheusSecretRef(p, pool); err == nil {
			t.Fatal("expected failure, got nil")
		}
		if len(pool.SecretConfigs()) != 0 {
			t.Fatalf("expected no configs, got %v", pool.SecretConfigs())
		}
	})
}

func TestClusterSecretKeySelector_toPrometheusSecretRef_ClusterPodMonitoring(t *testing.T) {
	p := &ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{Namespace: "foo"},
	}

	t.Run("nil", func(t *testing.T) {
		pool := PrometheusSecretConfigs{}
		var c *KubernetesSecretKeySelector

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

	for _, ns := range []string{"", p.Namespace, "different"} {
		t.Run(fmt.Sprintf("namespace=%v", ns), func(t *testing.T) {
			expectedNs := "foo"
			if ns == "different" {
				expectedNs = ns
			}

			pool := PrometheusSecretConfigs{}
			c := &KubernetesSecretKeySelector{
				Name:      "secret1",
				Key:       "key1",
				Namespace: ns,
			}

			ref, err := c.toPrometheusSecretRef(p, pool)
			if err != nil {
				t.Fatalf("unexpected failure: %s", err)
			}
			if exp, got := expectedNs+"/secret1/key1", ref; exp != got {
				t.Fatalf("expected ref %v, got %v", exp, got)
			}
			if exp, got := []secrets.SecretConfig{
				{
					Name: expectedNs + "/secret1/key1", Config: secrets.KubernetesSecretConfig{
						Name:      c.Name,
						Key:       c.Key,
						Namespace: expectedNs,
					},
				},
			}, pool.SecretConfigs(); cmp.Diff(exp, got) != "" {
				t.Fatalf("unpexpted secret configs; diff: %v", cmp.Diff(exp, got))
			}
		})
	}
}
