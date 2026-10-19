// Copyright 2025 Google LLC
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

package e2e

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// existingKindCluster returns a client and kubeconfig path for the cluster of
// the current kubeconfig context if that context points to a reachable kind
// cluster (e.g. the one provisioned by hack/kind-test.sh). Otherwise, it returns
// an error describing why the cluster cannot be reused.
func existingKindCluster(t *testing.T) (client.Client, string, error) {
	t.Helper()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	// kubectl --kubeconfig accepts a single file rather than a KUBECONFIG-style
	// list, so use the first existing file from the loading precedence.
	kubeconfigPath := rules.GetDefaultFilename()
	if kubeconfigPath == "" {
		return nil, "", errors.New("no kubeconfig path found")
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil)
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, "", fmt.Errorf("load kubeconfig: %w", err)
	}
	if !strings.HasPrefix(rawConfig.CurrentContext, "kind-") {
		return nil, "", fmt.Errorf("current context %q is not a kind cluster", rawConfig.CurrentContext)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("build REST config for context %q: %w", rawConfig.CurrentContext, err)
	}
	if err := setRESTConfigDefaults(restConfig); err != nil {
		return nil, "", fmt.Errorf("set REST config defaults: %w", err)
	}

	c, err := newKubeClient(restConfig)
	if err != nil {
		return nil, "", fmt.Errorf("create client: %w", err)
	}
	if err := c.List(t.Context(), &corev1.NamespaceList{}, client.Limit(1)); err != nil {
		return nil, "", fmt.Errorf("cluster of context %q is not reachable: %w", rawConfig.CurrentContext, err)
	}
	t.Logf("Reusing kind cluster of current context %q (kubeconfig %s)", rawConfig.CurrentContext, kubeconfigPath)
	return c, kubeconfigPath, nil
}

// createOrReuseKindCluster returns a client and kubeconfig path for a kind
// cluster with the GMP CRDs and validating admission policies applied. It reuses
// the cluster of the current kubeconfig context if that is a reachable kind
// cluster, and otherwise creates a dedicated one that is deleted after the test.
//
// Callers must not use t.Parallel: tests reusing a cluster share it, and
// concurrent "kubectl apply" runs of the same manifests can fail with
// AlreadyExists errors.
func createOrReuseKindCluster(t *testing.T) (client.Client, string) {
	t.Helper()

	c, kubeconfigPath, reuseErr := existingKindCluster(t)
	if reuseErr != nil {
		t.Logf("Creating a dedicated kind cluster instead of reusing the current context: %v", reuseErr)

		// Add a randomized suffix to the test cluster name to reduce collisions.
		clusterName := fmt.Sprintf("crd-test-%s", rand.String(6))

		tmp := t.TempDir()
		kubeconfigPath = filepath.Join(tmp, "kubeconfig")

		// Create a cluster with a randomized name, and save the kubeconfig in a temporary directory scoped to this test.
		createClusterOutput, err := exec.CommandContext(t.Context(), "kind", "create", "cluster", "--name", clusterName, "--kubeconfig", kubeconfigPath).CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s\n", createClusterOutput)

		t.Cleanup(cleanupKindCluster(t, clusterName))

		// Load the test cluster kubeconfig.
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			t.Fatalf("%v", err)
		}

		// Create a client for the test cluster.
		c, err = newKubeClient(config)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Apply GMP CRDs.
	applyCRDsOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "../manifests/setup.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("%s\b%v", applyCRDsOutput, err)
	}
	t.Logf("%s\n", applyCRDsOutput)

	// Create Public namespace for OperatorConfig if it does not already exist.
	if err := c.Create(t.Context(), &corev1.Namespace{Name: "gmp-public"}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}

	// Apply Validating Admission Policy.
	applyValidatingAdmissionOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "../charts/operator/templates/validating-admission-policy.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("%s\b%v", applyValidatingAdmissionOutput, err)
	}
	t.Logf("%s\n", applyValidatingAdmissionOutput)

	// Wait for CRDs to be created - there seems to be race condition without this wait.
	if _, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "wait", "customresourcedefinition.apiextensions.k8s.io/clusternodemonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/clusterpodmonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/clusterrules.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/globalrules.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/operatorconfigs.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/podmonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/rules.monitoring.googleapis.com", "--for=create").CombinedOutput(); err != nil {
		t.Fatal(err)
	}

	return c, kubeconfigPath
}

func cleanupKindCluster(t *testing.T, clusterName string) func() {
	return func() {
		out, err := exec.Command("kind", "delete", "cluster", "--name", clusterName).CombinedOutput()
		t.Logf("%s\n", out)
		if err != nil {
			t.Log(err.Error())
		}
	}
}

func deleteTestObject(t *testing.T, c client.Client, obj client.Object) {
	t.Helper()

	err := c.Delete(t.Context(), obj)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Errorf("Failed to delete object %s: %v", obj.GetName(), err)
		return
	}

	err = wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj.DeepCopyObject().(client.Object))
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		t.Errorf("Failed waiting for object %s to be deleted: %v", obj.GetName(), err)
	}
}

func TestClusterPodMonitoringDefaultingYAML(t *testing.T) {
	c, kubeconfigPath := createOrReuseKindCluster(t)

	tests := []struct {
		file string
		want monitoringv1.ClusterPodMonitoringSpec
	}{
		{
			file: "cluster-pod-monitoring.yaml",
			want: monitoringv1.ClusterPodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "prom-example",
					},
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{
					{
						Port:     intstr.IntOrString{StrVal: "metrics", Type: intstr.String},
						Interval: "30s",
					},
				},
				TargetLabels: monitoringv1.ClusterTargetLabels{
					Metadata: &[]string{"container", "namespace", "pod", "top_level_controller_name", "top_level_controller_type"},
				},
			},
		},
	}

	for _, tc := range tests {
		createClusterPodMonitoringOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tc.file).CombinedOutput()
		if err != nil {
			t.Fatalf("%s\b%v", createClusterPodMonitoringOutput, err)
		}
		t.Logf("%s\n", createClusterPodMonitoringOutput)

		var got monitoringv1.ClusterPodMonitoring
		if err := c.Get(t.Context(), client.ObjectKey{Namespace: "default", Name: "prom-example"}, &got); err != nil {
			t.Error(err)
		} else {
			deleteTestObject(t, c, &got)
		}

		if diff := cmp.Diff(tc.want, got.Spec); diff != "" {
			t.Error(diff)
		}
	}
}

func TestPodMonitoringDefaultingYAML(t *testing.T) {
	c, kubeconfigPath := createOrReuseKindCluster(t)

	tests := []struct {
		file string
		want monitoringv1.PodMonitoringSpec
	}{
		{
			file: "../examples/pod-monitoring.yaml",
			want: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "prom-example",
					},
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{
					{
						Port:     intstr.IntOrString{StrVal: "metrics", Type: intstr.String},
						Interval: "30s",
					},
				},
				TargetLabels: monitoringv1.TargetLabels{
					Metadata: &[]string{"container", "pod", "top_level_controller_name", "top_level_controller_type"},
				},
			},
		},
	}

	for _, tc := range tests {
		createPodMonitoringOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tc.file).CombinedOutput()
		if err != nil {
			t.Fatalf("%s\b%v", createPodMonitoringOutput, err)
		}
		t.Logf("%s\n", createPodMonitoringOutput)

		var got monitoringv1.PodMonitoring
		if err := c.Get(t.Context(), client.ObjectKey{Namespace: "default", Name: "prom-example"}, &got); err != nil {
			t.Error(err)
		} else {
			deleteTestObject(t, c, &got)
		}

		if diff := cmp.Diff(tc.want, got.Spec); diff != "" {
			t.Error(diff)
		}
	}
}

func TestCRDDefaulting(t *testing.T) {
	c, _ := createOrReuseKindCluster(t)

	type cpmTest struct {
		obj  *monitoringv1.ClusterPodMonitoring
		want *monitoringv1.ClusterPodMonitoring
	}
	runCPM := func(t *testing.T, tests map[string]cpmTest) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					defer deleteTestObject(t, c, tc.obj)
				}

				err := c.Get(
					t.Context(),
					client.ObjectKeyFromObject(tc.obj),
					tc.obj,
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if diff := cmp.Diff(tc.want.Spec, tc.obj.Spec); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	}

	type pmTest struct {
		obj  *monitoringv1.PodMonitoring
		want *monitoringv1.PodMonitoring
	}
	runPM := func(t *testing.T, tests map[string]pmTest) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					defer deleteTestObject(t, c, tc.obj)
				}

				err := c.Get(
					t.Context(),
					client.ObjectKeyFromObject(tc.obj),
					tc.obj,
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if diff := cmp.Diff(tc.want.Spec, tc.obj.Spec); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	}

	t.Run("ClusterPodMonitoring", func(t *testing.T) {
		tests := map[string]cpmTest{
			"TargetLabels/default": {
				obj: &monitoringv1.ClusterPodMonitoring{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				want: &monitoringv1.ClusterPodMonitoring{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							Metadata: &[]string{"container", "namespace", "pod", "top_level_controller_name", "top_level_controller_type"},
						},
					},
				},
			},
			"TargetLabels/nondefault": {
				obj: &monitoringv1.ClusterPodMonitoring{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "from",
									To:   "to",
								},
							},
						},
					},
				},
				want: &monitoringv1.ClusterPodMonitoring{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							Metadata: &[]string{"container", "namespace", "pod", "top_level_controller_name", "top_level_controller_type"},
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "from",
									To:   "to",
								},
							},
						},
					},
				},
			},
		}

		runCPM(t, tests)
	})

	t.Run("PodMonitoring", func(t *testing.T) {
		tests := map[string]pmTest{
			"TargetLabels/default": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				want: &monitoringv1.PodMonitoring{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							Metadata: &[]string{"container", "pod", "top_level_controller_name", "top_level_controller_type"},
						},
					},
				},
			},
			"TargetLabels/nondefault": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "from",
									To:   "to",
								},
							},
						},
					},
				},
				want: &monitoringv1.PodMonitoring{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							Metadata: &[]string{"container", "pod", "top_level_controller_name", "top_level_controller_type"},
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "from",
									To:   "to",
								},
							},
						},
					},
				},
			},
		}
		runPM(t, tests)
	})

	type ocTest struct {
		obj  *monitoringv1.OperatorConfig
		want *monitoringv1.OperatorConfig
	}
	runOC := func(t *testing.T, tests map[string]ocTest) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				defer func() {
					_ = c.Delete(t.Context(), tc.obj)
					_ = wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
						err := c.Get(ctx, client.ObjectKeyFromObject(tc.obj), tc.obj.DeepCopyObject().(client.Object))
						return apierrors.IsNotFound(err), nil
					})
				}()

				err := c.Get(
					t.Context(),
					client.ObjectKeyFromObject(tc.obj),
					tc.obj,
				)
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if diff := cmp.Diff(tc.want.ManagedAlertmanager, tc.obj.ManagedAlertmanager); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	}

	t.Run("OperatorConfig", func(t *testing.T) {
		tests := map[string]ocTest{
			"ManagedAlertmanager/default": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
				},
				want: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ConfigSecret: &corev1.SecretKeySelector{
							Name: "alertmanager",
							Key:  "alertmanager.yaml",
						},
					},
				},
			},
			"ManagedAlertmanager/nondefault": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ExternalURL: "https://alertmanager.example.com",
					},
				},
				want: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ConfigSecret: &corev1.SecretKeySelector{
							Name: "alertmanager",
							Key:  "alertmanager.yaml",
						},
						ExternalURL: "https://alertmanager.example.com",
					},
				},
			},
		}
		runOC(t, tests)
	})

	t.Run("Rules", func(t *testing.T) {
		tests := map[string]struct {
			obj  *monitoringv1.Rules
			want *monitoringv1.Rules
		}{
			"Interval/default": {
				obj: &monitoringv1.Rules{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.Rules{
					Name:      "default",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "1m",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
			"Interval/nondefault": {
				obj: &monitoringv1.Rules{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.Rules{
					Name:      "nondefault",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if err := c.Get(t.Context(), client.ObjectKeyFromObject(tc.obj), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want.Spec, tc.obj.Spec); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	})

	t.Run("ClusterRules", func(t *testing.T) {
		tests := map[string]struct {
			obj  *monitoringv1.ClusterRules
			want *monitoringv1.ClusterRules
		}{
			"Interval/default": {
				obj: &monitoringv1.ClusterRules{
					Name: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.ClusterRules{
					Name: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "1m",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
			"Interval/nondefault": {
				obj: &monitoringv1.ClusterRules{
					Name: "nondefault",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.ClusterRules{
					Name: "nondefault",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if err := c.Get(t.Context(), client.ObjectKeyFromObject(tc.obj), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want.Spec, tc.obj.Spec); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	})

	t.Run("GlobalRules", func(t *testing.T) {
		tests := map[string]struct {
			obj  *monitoringv1.GlobalRules
			want *monitoringv1.GlobalRules
		}{
			"Interval/default": {
				obj: &monitoringv1.GlobalRules{
					Name: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.GlobalRules{
					Name: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "1m",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
			"Interval/nondefault": {
				obj: &monitoringv1.GlobalRules{
					Name: "nondefault",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				want: &monitoringv1.GlobalRules{
					Name: "nondefault",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "30s",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_metric",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if err := c.Get(t.Context(), client.ObjectKeyFromObject(tc.obj), tc.obj); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want.Spec, tc.obj.Spec); diff != "" {
					t.Errorf("diff -want +got:\n%s", diff)
				}
			})
		}
	})
}

func TestCRDValidation(t *testing.T) {
	c, _ := createOrReuseKindCluster(t)

	type test struct {
		obj        client.Object
		wantErr    bool
		wantErrMsg string
	}

	run := func(t *testing.T, tests map[string]test) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				err := c.Create(t.Context(), tc.obj)
				if err == nil {
					deleteTestObject(t, c, tc.obj)
				}
				switch {
				case err == nil && !tc.wantErr:
					// OK.
				case err != nil && !tc.wantErr:
					t.Errorf("Unexpected error: %v", err)
				case err == nil && tc.wantErr:
					t.Error("Want error, but got none")
				case err != nil && tc.wantErr:
					t.Log(err)
					if tc.wantErrMsg != "" && !strings.Contains(err.Error(), tc.wantErrMsg) {
						t.Errorf("Expected error containing %q, got: %v", tc.wantErrMsg, err)
					}
				}
			})
		}
	}
	t.Run("ClusterNodeMonitoring", func(t *testing.T) {
		tests := map[string]test{
			"scrape interval missing": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					Name:      "scrape-interval-missing",
					Namespace: "default",
					Spec: monitoringv1.ClusterNodeMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeNodeEndpoint{
							{},
						},
					},
				},
			},
			"scrape interval malformed": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					Name:      "scrape-interval-malformed",
					Namespace: "default",
					Spec: monitoringv1.ClusterNodeMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeNodeEndpoint{
							{
								Interval: "foo",
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape timeout malformed": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					Name:      "scrape-timeout-malformed",
					Namespace: "default",
					Spec: monitoringv1.ClusterNodeMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeNodeEndpoint{
							{
								Interval: "1m",
								Timeout:  "foo",
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape timeout greater than interval": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					Name:      "scrape-timeout-greater-than-interval",
					Namespace: "default",
					Spec: monitoringv1.ClusterNodeMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeNodeEndpoint{
							{
								Interval: "1m",
								Timeout:  "5m",
							},
						},
					},
				},
				wantErr: true,
			},
		}
		run(t, tests)
	})

	t.Run("ClusterPodMonitoring", func(t *testing.T) {
		tests := map[string]test{
			"namespace on secret reference": {
				obj: &monitoringv1.ClusterPodMonitoring{
					Name: "namespace-on-secret-references",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								OAuth2: &monitoringv1.OAuth2{
									ClientSecret: &monitoringv1.SecretSelector{
										Secret: &monitoringv1.SecretKeySelector{
											Name:      "test",
											Namespace: "hack",
										},
									},
								},
							},
						},
					},
				},
			},
		}
		run(t, tests)
	})

	t.Run("OperatorConfig", func(t *testing.T) {
		tests := map[string]test{
			"empty": {
				obj:        &monitoringv1.OperatorConfig{},
				wantErr:    true,
				wantErrMsg: "an empty namespace may not be set during creation",
			},
			"invalid name": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "invalid-name",
					Namespace: "gmp-public",
				},
				wantErr:    true,
				wantErrMsg: "object.metadata.name == 'config'",
			},
			"invalid namespace": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "default",
				},
				wantErr:    true,
				wantErrMsg: "object.metadata.namespace == 'gmp-public'",
			},
			"bad scrape interval": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						KubeletScraping: &monitoringv1.KubeletScraping{
							Interval: "xyz",
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "collection.kubeletScraping.interval in body must be of type duration",
			},
			"missing scrape interval": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						KubeletScraping: &monitoringv1.KubeletScraping{
							Interval: "",
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "collection.kubeletScraping.interval in body must be of type duration",
			},
			"missing scrape interval (omitted)": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "OperatorConfig",
						"metadata": map[string]any{
							"name":      "config",
							"namespace": "gmp-public",
						},
						"collection": map[string]any{
							"kubeletScraping": map[string]any{},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "collection.kubeletScraping.interval: Required value",
			},
			"valid compression": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						Compression: monitoringv1.CompressionGzip,
					},
					Features: monitoringv1.OperatorFeatures{
						Config: monitoringv1.ConfigSpec{
							Compression: monitoringv1.CompressionNone,
						},
					},
				},
			},
			"bad collection compression": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						Compression: "snappy",
					},
				},
				wantErr:    true,
				wantErrMsg: "collection.compression: Unsupported value",
			},
			"bad features config compression": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Features: monitoringv1.OperatorFeatures{
						Config: monitoringv1.ConfigSpec{
							Compression: "snappy",
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "features.config.compression: Unsupported value",
			},
			"missing collection credentials secret key": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						Credentials: &corev1.SecretKeySelector{},
					},
				},
				wantErr:    true,
				wantErrMsg: "collection.credentials: Invalid value",
			},
			"bad generator URL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						GeneratorURL: "~:://example.com",
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.generatorUrl",
			},
			"missing rule manager credentials secret key": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Credentials: &corev1.SecretKeySelector{},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.credentials: Invalid value",
			},
			"missing managed alert manager config secret key": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ConfigSecret: &corev1.SecretKeySelector{},
					},
				},
				wantErr:    true,
				wantErrMsg: "managedAlertmanager.configSecret: Invalid value",
			},
			"rule manager authorization credentials secret key missing": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS:  &monitoringv1.TLSConfig{},
								Authorization: &monitoringv1.Authorization{
									Credentials: &corev1.SecretKeySelector{},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].authorization.credentials: Invalid value",
			},
			"rule manager TLS secret key missing": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS: &monitoringv1.TLSConfig{
									KeySecret: &corev1.SecretKeySelector{},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].tls.keySecret: Invalid value",
			},
			"rule manager TLS CA mutually exclusive": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS: &monitoringv1.TLSConfig{
									CA: &monitoringv1.SecretOrConfigMap{
										Secret: &corev1.SecretKeySelector{
											Name: "baz",
										},
										ConfigMap: &corev1.ConfigMapKeySelector{
											Name: "qux",
										},
									},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].tls.ca: Invalid value",
			},
			"rule manager TLS CA secret key missing": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS: &monitoringv1.TLSConfig{
									CA: &monitoringv1.SecretOrConfigMap{
										Secret: &corev1.SecretKeySelector{},
									},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].tls.ca.secret: Invalid value",
			},
			"rule manager TLS Cert mutually exclusive": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS: &monitoringv1.TLSConfig{
									Cert: &monitoringv1.SecretOrConfigMap{
										Secret: &corev1.SecretKeySelector{
											Name: "baz",
										},
										ConfigMap: &corev1.ConfigMapKeySelector{
											Name: "qux",
										},
									},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].tls.cert: Invalid value",
			},
			"rule manager TLS Cert secret key missing": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
								Name: "bar",
								TLS: &monitoringv1.TLSConfig{
									Cert: &monitoringv1.SecretOrConfigMap{
										Secret: &corev1.SecretKeySelector{},
									},
								},
							}},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].tls.cert.secret: Invalid value",
			},
			"minimal": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
				},
			},
			"valid credentials secret name (RFC 1123 subdomain)": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						Credentials: &corev1.SecretKeySelector{
							Name: "my-secret.v1",
							Key:  "key.json",
						},
					},
				},
			},
			"queryProjectID empty string": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "OperatorConfig",
						"metadata": map[string]any{
							"name":      "config",
							"namespace": "gmp-public",
						},
						"rules": map[string]any{
							"queryProjectID": "",
						},
					},
				},
				wantErr: false,
			},
			"queryProjectID valid": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "OperatorConfig",
						"metadata": map[string]any{
							"name":      "config",
							"namespace": "gmp-public",
						},
						"rules": map[string]any{
							"queryProjectID": "my-valid-project",
						},
					},
				},
				wantErr: false,
			},
			"valid externalLabels": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Collection: monitoringv1.CollectionSpec{
						ExternalLabels: map[string]string{
							"env": "production",
						},
					},
					Rules: monitoringv1.RuleEvaluatorSpec{
						ExternalLabels: map[string]string{
							"region": "us-central1",
						},
					},
				},
				wantErr: false,
			},
			"valid generator URL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						GeneratorURL: "https://example.com/graph",
					},
				},
				wantErr: false,
			},
			"valid exports URL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Exports: []monitoringv1.ExportSpec{
						{
							URL: "https://remote-write.example.com/api/v1/write",
						},
					},
				},
				wantErr: false,
			},
			"bad exports URL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Exports: []monitoringv1.ExportSpec{
						{
							URL: "~:://example.com",
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "exports[0].url",
			},
			"valid externalURL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ConfigSecret: &corev1.SecretKeySelector{
							Name: "alertmanager",
							Key:  "alertmanager.yaml",
						},
						ExternalURL: "https://alertmanager.example.com",
					},
				},
				wantErr: false,
			},
			"bad externalURL": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
						ConfigSecret: &corev1.SecretKeySelector{
							Name: "alertmanager",
							Key:  "alertmanager.yaml",
						},
						ExternalURL: "~:://example.com",
					},
				},
				wantErr:    true,
				wantErrMsg: "managedAlertmanager.externalURL",
			},
			"valid AlertmanagerEndpoints": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{
								{
									Namespace: "monitoring",
									Name:      "alertmanager-operated",
									Port:      intstr.FromString("web"),
									Scheme:    "https",
									Timeout:   "10s",
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"bad alertmanager timeout": {
				obj: &monitoringv1.OperatorConfig{
					Name:      "config",
					Namespace: "gmp-public",
					Rules: monitoringv1.RuleEvaluatorSpec{
						Alerting: monitoringv1.AlertingSpec{
							Alertmanagers: []monitoringv1.AlertmanagerEndpoints{
								{
									Namespace: "monitoring",
									Name:      "alertmanager-operated",
									Port:      intstr.FromString("web"),
									Timeout:   "xyz",
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].timeout in body must be of type duration",
			},
			"missing alertmanager required fields (omitted)": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "OperatorConfig",
						"metadata": map[string]any{
							"name":      "config",
							"namespace": "gmp-public",
						},
						"rules": map[string]any{
							"alerting": map[string]any{
								"alertmanagers": []any{
									map[string]any{
										"name": "alertmanager-operated",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "rules.alerting.alertmanagers[0].namespace: Required value",
			},
		}
		run(t, tests)
	})

	t.Run("PodMonitoring", func(t *testing.T) {
		tests := map[string]test{
			"empty": {
				obj:     &monitoringv1.PodMonitoring{},
				wantErr: true,
			},
			"minimal": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "minimal",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
			},
			"port missing": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "port-missing",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
							},
						},
					},
				},
				wantErr: true,
			},
			// Regression case for b/464455553.
			"port using regex": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "port-using-regex",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics|maybe|apply-to_this(.*)|and-maybe(s?)"),
							},
						},
					},
				},
				wantErr: false,
			},
			"duplicate port": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "duplicate-port",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape interval missing": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "scrape-interval-missing",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Port: intstr.FromString("metrics"),
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape interval malformed": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "scrape-interval-malformed",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "foo",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape timeout malformed": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "scrape-timeout-malformed",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Timeout:  "foo",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				wantErr: true,
			},
			"scrape timeout greater than interval": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "scrape-timeout-greater-than-interval",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Timeout:  "5m",
								Port:     intstr.FromString("metrics"),
							},
						},
					},
				},
				wantErr: true,
			},
			"remapping onto prometheus_target label": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "remapping-onto-prometheus-target-label",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "key-1", To: "cluster"},
							},
						},
					},
				},
				wantErr: true,
			},
			"remapping onto bad label name": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "remapping-onto-bad-label-name",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{From: "key1", To: "foo-bar"},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: valid": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "relabeling-valid",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval:         "1m",
								Port:             intstr.FromString("metrics1"),
								MetricRelabeling: generateRelabelingRules(250),
							},
							{
								Interval:         "30s",
								Port:             intstr.FromString("metrics2"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "3h",
								Port:             intstr.FromString("metrics3"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "24h",
								Port:             intstr.FromString("metrics4"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "168h",
								Port:             intstr.FromString("metrics5"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "8766h",
								Port:             intstr.FromString("metrics6"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "123456ms",
								Port:             intstr.FromString("metrics7"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "90s",
								Port:             intstr.FromString("metrics8"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "1m30s",
								Port:             intstr.FromString("metrics9"),
								MetricRelabeling: generateRelabelingRules(10),
							},
							{
								Interval:         "1m10s25ms",
								Port:             intstr.FromString("metrics10"),
								MetricRelabeling: generateRelabelingRules(10),
							},
						},
					},
				},
				wantErr: false,
			},
			"metric relabeling: labelmap forbidden": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "labelmap-forbidden",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"foo", "bar"},
										Action:       "labelmap",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: protected replace label": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "protected-replace-label",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action:      "replace",
										TargetLabel: "project_id",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: protected labelkeep": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "protected-labelkeep",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action: "labelkeep",
										Regex:  "(cluster|location|namespace|job|instance|__address__)",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: protected labeldrop": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "protected-labeldrop",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action: "labeldrop",
										Regex:  "n?amespace",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: labeldrop default regex": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "labeldrop-default-regex",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action: "labeldrop",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metric relabeling: labelkeep default regex": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "labelkeep-default-regex",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										Action: "labelkeep",
									},
								},
							},
						},
					},
				},
			},
			"metric relabeling: empty action is valid and defaults to replace": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "empty-action-valid",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								MetricRelabeling: []monitoringv1.RelabelingRule{
									{
										SourceLabels: []string{"foo"},
										TargetLabel:  "bar",
										Replacement:  "baz",
									},
								},
							},
						},
					},
				},
			},
			"invalid URL": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "invalid-url",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								ProxyURL: "_:_",
							},
						},
					},
				},
				wantErr: true,
			},
			"proxy URL with password": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "proxy-url-with-password",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								ProxyURL: "http://user:password@foo.bar/",
							},
						},
					},
				},
				wantErr: true,
			},
			"metadata labels empty": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "metadata-labels-empty",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.TargetLabels{
							Metadata: &[]string{},
						},
					},
				},
			},
			"TLS setting invalid": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "tls-setting-invalid",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								TLS: &monitoringv1.TLS{
									MinVersion: "TLS09",
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"TLS setting valid": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "proxy-url-with-password",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								TLS: &monitoringv1.TLS{
									MinVersion: "TLS13",
								},
							},
						},
					},
				},
			},
			"authentication basic header": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "authentication-basic-header",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								Authorization: &monitoringv1.Auth{
									Type: "Basic",
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"basic auth and authorization header": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "authentication-basic-header",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								Authorization: &monitoringv1.Auth{
									Type: "Bearer",
								},
								BasicAuth: &monitoringv1.BasicAuth{
									Username: "xyz",
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"authorization header and oauth2": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "authentication-basic-header",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								Authorization: &monitoringv1.Auth{
									Type: "Bearer",
								},
								OAuth2: &monitoringv1.OAuth2{
									ClientID: "xyz",
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"client cert only": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "client-cert-only",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								OAuth2: &monitoringv1.OAuth2{
									TLS: &monitoringv1.TLS{
										Cert: &monitoringv1.SecretSelector{
											Secret: &monitoringv1.SecretKeySelector{
												Name: "test",
											},
										},
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"client key only": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "client-key-only",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								OAuth2: &monitoringv1.OAuth2{
									TLS: &monitoringv1.TLS{
										Key: &monitoringv1.SecretSelector{
											Secret: &monitoringv1.SecretKeySelector{
												Name: "test",
											},
										},
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"client cert/key pair": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "client-cert-key-pair",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								OAuth2: &monitoringv1.OAuth2{
									TLS: &monitoringv1.TLS{
										Cert: &monitoringv1.SecretSelector{
											Secret: &monitoringv1.SecretKeySelector{
												Name: "test",
											},
										},
										Key: &monitoringv1.SecretSelector{
											Secret: &monitoringv1.SecretKeySelector{
												Name: "test",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"namespace on secret reference": {
				obj: &monitoringv1.PodMonitoring{
					Name:      "namespace-on-secret-references",
					Namespace: "default",
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								OAuth2: &monitoringv1.OAuth2{
									ClientSecret: &monitoringv1.SecretSelector{
										Secret: &monitoringv1.SecretKeySelector{
											Name:      "test",
											Namespace: "hack",
										},
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
		}
		run(t, tests)
	})
	t.Run("Rules", func(t *testing.T) {
		tests := map[string]test{
			"minimal-alerting": {
				obj: &monitoringv1.Rules{
					Name:      "minimal-alerting",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Alert: "Any-characters:Allowed!?#@",
										Expr:  "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"minimal-recording": {
				obj: &monitoringv1.Rules{
					Name:      "minimal-recording",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"prometheus-duration-units": {
				obj: &monitoringv1.Rules{
					Name:      "prometheus-duration-units",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "1d",
								Rules: []monitoringv1.Rule{
									{
										Alert: "SlowBurn",
										Expr:  "vector(1)",
										For:   "1w",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"valid-rule-name-leading-colon-and-underscore": {
				obj: &monitoringv1.Rules{
					Name:      "valid-leading-colon-underscore",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: ":leading_colon",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"_leading_underscore": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"empty-groups-slice": {
				obj: &monitoringv1.Rules{
					Name:      "empty-groups-slice",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{},
					},
				},
				wantErr: false,
			},
			"nil-groups-slice": {
				obj: &monitoringv1.Rules{
					Name:      "nil-groups-slice",
					Namespace: "default",
					Spec:      monitoringv1.RulesSpec{},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups",
			},
			"empty-group-name": {
				obj: &monitoringv1.Rules{
					Name:      "empty-group-name",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].name",
			},
			"duplicate-group-name": {
				obj: &monitoringv1.Rules{
					Name:      "duplicate-group-name",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_1",
										Expr:   "vector(1)",
									},
								},
							},
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_2",
										Expr:   "vector(2)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule group names must be unique",
			},
			"empty-expr": {
				obj: &monitoringv1.Rules{
					Name:      "empty-expr",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules[0].expr",
			},
			"invalid-interval": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-interval",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name:     "group-1",
								Interval: "foo",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].interval",
			},
			"invalid-rule-name": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-rule-name",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "dots.not.allowed",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules[0].record",
			},
			"invalid-rule-name-dash": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-rule-name-dash",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "dashes-not-allowed",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules[0].record",
			},
			"invalid-rule-name-leading-digit": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-rule-name-digit",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "0leading_digit",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules[0].record",
			},
			"valid-rule-name-colon": {
				obj: &monitoringv1.Rules{
					Name:      "valid-rule-name-colon",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "colon:allowed",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"invalid-annotation": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-annotation",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Annotations: map[string]string{
											"test": "annotation",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Annotations are only allowed for alerting rules",
			},
			"valid-annotation": {
				obj: &monitoringv1.Rules{
					Name:      "valid-annotation",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Alert: "test",
										Expr:  "vector(1)",
										Annotations: map[string]string{
											"test": "annotation",
										},
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"for-on-recording-rule": {
				obj: &monitoringv1.Rules{
					Name:      "for-on-recording-rule",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										For:    "5m",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "For is only allowed for alerting rules",
			},
			"for-0s-on-recording-rule": {
				obj: &monitoringv1.Rules{
					Name:      "for-0s-on-recording-rule",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										For:    "0s",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "For is only allowed for alerting rules",
			},
			"for-1d-on-recording-rule": {
				obj: &monitoringv1.Rules{
					Name:      "for-1d-on-recording-rule",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										For:    "1d",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "For is only allowed for alerting rules",
			},
			"invalid-label-key": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-label-key",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"invalid-key": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Label keys must match",
			},
			"invalid-label-key-colon": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-label-key-colon",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"colon:not_allowed": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Label keys must match",
			},
			"label-key-__name__": {
				obj: &monitoringv1.Rules{
					Name:      "label-key-name",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"__name__": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Label keys must match",
			},
			"invalid-annotation-key": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-annotation-key",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Alert: "test",
										Expr:  "vector(1)",
										Annotations: map[string]string{
											"invalid-key": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Annotation keys must match",
			},
			"invalid-annotation-key-colon": {
				obj: &monitoringv1.Rules{
					Name:      "invalid-annotation-key-colon",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Alert: "test",
										Expr:  "vector(1)",
										Annotations: map[string]string{
											"colon:not_allowed": "val",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Annotation keys must match",
			},
			"reserved-label-project-id": {
				obj: &monitoringv1.Rules{
					Name:      "reserved-label-project-id",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"project_id": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule labels cannot contain reserved keys",
			},
			"reserved-label-location": {
				obj: &monitoringv1.Rules{
					Name:      "reserved-label-location",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"location": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule labels cannot contain reserved keys",
			},
			"reserved-label-cluster": {
				obj: &monitoringv1.Rules{
					Name:      "reserved-label-cluster",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"cluster": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule labels cannot contain reserved keys",
			},
			"reserved-label-namespace": {
				obj: &monitoringv1.Rules{
					Name:      "reserved-label-namespace",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"namespace": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule labels cannot contain reserved keys",
			},
			"alert-and-record-both-set": {
				obj: &monitoringv1.Rules{
					Name:      "alert-and-record-set",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Alert:  "alert",
										Record: "record",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Must set exactly one of Record or Alert",
			},
			"neither-alert-nor-record-set": {
				obj: &monitoringv1.Rules{
					Name:      "neither-alert-nor-record-set",
					Namespace: "default",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Expr: "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Must set exactly one of Record or Alert",
			},
			"unstructured-missing-spec": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-missing-spec",
							"namespace": "default",
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec: Required value",
			},
			"unstructured-missing-spec-groups": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-missing-groups",
							"namespace": "default",
						},
						"spec": map[string]any{},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups: Required value",
			},
			"unstructured-missing-group-name": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-missing-group-name",
							"namespace": "default",
						},
						"spec": map[string]any{
							"groups": []any{
								map[string]any{
									"rules": []any{
										map[string]any{
											"record": "test",
											"expr":   "vector(1)",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].name: Required value",
			},
			"unstructured-missing-group-rules": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-missing-group-rules",
							"namespace": "default",
						},
						"spec": map[string]any{
							"groups": []any{
								map[string]any{
									"name": "group-1",
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules: Required value",
			},
			"unstructured-empty-group-rules": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-empty-group-rules",
							"namespace": "default",
						},
						"spec": map[string]any{
							"groups": []any{
								map[string]any{
									"name":  "group-1",
									"rules": []any{},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules",
			},
			"unstructured-missing-rule-expr": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-missing-rule-expr",
							"namespace": "default",
						},
						"spec": map[string]any{
							"groups": []any{
								map[string]any{
									"name": "group-1",
									"rules": []any{
										map[string]any{
											"record": "test",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "spec.groups[0].rules[0].expr: Required value",
			},
			"unstructured-recording-empty-annotations": {
				obj: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "monitoring.googleapis.com/v1",
						"kind":       "Rules",
						"metadata": map[string]any{
							"name":      "unstructured-recording-empty-annotations",
							"namespace": "default",
						},
						"spec": map[string]any{
							"groups": []any{
								map[string]any{
									"name": "group-1",
									"rules": []any{
										map[string]any{
											"record":      "test",
											"expr":        "vector(1)",
											"annotations": map[string]any{},
										},
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
		}
		run(t, tests)
	})
	t.Run("ClusterRules", func(t *testing.T) {
		tests := map[string]test{
			"minimal-recording": {
				obj: &monitoringv1.ClusterRules{
					Name: "minimal-recording",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"allowed-label-namespace": {
				obj: &monitoringv1.ClusterRules{
					Name: "allowed-label-namespace",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"namespace": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"reserved-label-project-id": {
				obj: &monitoringv1.ClusterRules{
					Name: "reserved-label-project-id",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"project_id": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "ClusterRule labels cannot contain reserved keys",
			},
			"reserved-label-location": {
				obj: &monitoringv1.ClusterRules{
					Name: "reserved-label-location",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"location": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "ClusterRule labels cannot contain reserved keys",
			},
			"reserved-label-cluster": {
				obj: &monitoringv1.ClusterRules{
					Name: "reserved-label-cluster",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"cluster": "other",
										},
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "ClusterRule labels cannot contain reserved keys",
			},
			"duplicate-group-name": {
				obj: &monitoringv1.ClusterRules{
					Name: "duplicate-group-name",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_1",
										Expr:   "vector(1)",
									},
								},
							},
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_2",
										Expr:   "vector(2)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule group names must be unique",
			},
		}
		run(t, tests)
	})
	t.Run("GlobalRules", func(t *testing.T) {
		tests := map[string]test{
			"minimal-recording": {
				obj: &monitoringv1.GlobalRules{
					Name: "minimal-recording",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"allowed-all-scope-labels": {
				obj: &monitoringv1.GlobalRules{
					Name: "allowed-all-scope-labels",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										Labels: map[string]string{
											"project_id": "other-project",
											"location":   "other-location",
											"cluster":    "other-cluster",
											"namespace":  "other-namespace",
										},
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"for-on-recording-rule": {
				obj: &monitoringv1.GlobalRules{
					Name: "for-on-recording-rule",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "group-1",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Expr:   "vector(1)",
										For:    "5m",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "For is only allowed for alerting rules",
			},
			"duplicate-group-name": {
				obj: &monitoringv1.GlobalRules{
					Name: "duplicate-group-name",
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_1",
										Expr:   "vector(1)",
									},
								},
							},
							{
								Name: "dup",
								Rules: []monitoringv1.Rule{
									{
										Record: "test_2",
										Expr:   "vector(2)",
									},
								},
							},
						},
					},
				},
				wantErr:    true,
				wantErrMsg: "Rule group names must be unique",
			},
		}
		run(t, tests)
	})
	t.Run("ClusterPodMonitoring-TargetLabels", func(t *testing.T) {
		tests := map[string]test{
			"TargetLabels/from-kubernetes-labels": {
				obj: &monitoringv1.ClusterPodMonitoring{
					Name:      "from-kubernetes-labels",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "app.kubernetes.io/name",
									To:   "k8s_app_name",
								},
								{
									From: "app.kubernetes.io/instance",
									To:   "k8s_instance",
								},
							},
						},
					},
				},
			},
			"TargetLabels/empty-to": {
				obj: &monitoringv1.ClusterPodMonitoring{
					Name:      "empty-to",
					Namespace: "default",
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
							},
						},
						TargetLabels: monitoringv1.ClusterTargetLabels{
							FromPod: []monitoringv1.LabelMapping{
								{
									From: "empty-to",
								},
							},
						},
					},
				},
			},
		}
		run(t, tests)
	})
}

func generateRelabelingRules(n uint) []monitoringv1.RelabelingRule {
	rules := make([]monitoringv1.RelabelingRule, n)
	actions := []string{"replace", "lowercase", "uppercase", "keep", "drop", "keepequal", "dropequal", "hashmod", "labeldrop", "labelkeep"}

	for i := range rules {
		rules[i] = monitoringv1.RelabelingRule{
			Regex:  rand.String(1000),
			Action: actions[rand.Intn(len(actions))],
		}
	}
	return rules
}
