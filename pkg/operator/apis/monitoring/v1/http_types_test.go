package v1

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/secrets"
	"github.com/prometheus/prometheus/secrets/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			if exp, got := []secrets.SecretConfig[kubernetes.SecretConfig]{
				{
					Name: "foo/secret1/key1", Config: kubernetes.SecretConfig{
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
			if exp, got := []secrets.SecretConfig[kubernetes.SecretConfig]{
				{
					Name: expectedNs + "/secret1/key1", Config: kubernetes.SecretConfig{
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
