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
	_ "embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createKindCluster(t *testing.T) (client.Client, string) {
	// Add a randomized suffix to the test cluster name to reduce collisions.
	clusterName := fmt.Sprintf("crd-test-%s", rand.String(6))

	tmp := t.TempDir()
	kubeconfigPath := filepath.Join(tmp, "kubeconfig")

	// Create a cluster with a randomized name, and save the kubeconfig in a temporary directory scoped to this test.
	createClusterOutput, err := exec.CommandContext(t.Context(), "kind", "create", "cluster", "--name", clusterName, "--kubeconfig", kubeconfigPath).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s\n", createClusterOutput)

	t.Cleanup(cleanupKindCluster(t, clusterName))

	// Apply GMP CRDs.
	applyCRDsOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "../manifests/setup.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("%s\b%v", applyCRDsOutput, err)
	}
	t.Logf("%s\n", applyCRDsOutput)

	// Create Public namespace for OperatorConfig.
	applyPublicNamespaceOutput, err := exec.CommandContext(t.Context(), "kubectl", "--kubeconfig", kubeconfigPath, "create", "namespace", "gmp-public").CombinedOutput()
	if err != nil {
		t.Fatalf("%s\b%v", applyPublicNamespaceOutput, err)
	}
	t.Logf("%s\n", applyPublicNamespaceOutput)

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

	// Load the test cluster kubeconfig.
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Create a client for the test cluster.
	c, err := newKubeClient(config)
	if err != nil {
		t.Error(err)
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

func TestClusterPodMonitoringDefaultingYAML(t *testing.T) {
	t.Parallel()

	c, kubeconfigPath := createKindCluster(t)

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
		}

		if diff := cmp.Diff(tc.want, got.Spec); diff != "" {
			t.Error(diff)
		}
	}
}

func TestPodMonitoringDefaultingYAML(t *testing.T) {
	t.Parallel()

	c, kubeconfigPath := createKindCluster(t)

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
		}

		if diff := cmp.Diff(tc.want, got.Spec); diff != "" {
			t.Error(diff)
		}
	}
}

func TestCRDDefaulting(t *testing.T) {
	t.Parallel()

	c, _ := createKindCluster(t)

	type cpmTest struct {
		obj  *monitoringv1.ClusterPodMonitoring
		want *monitoringv1.ClusterPodMonitoring
	}
	runCPM := func(t *testing.T, tests map[string]cpmTest) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := c.Create(t.Context(), tc.obj); err != nil {
					t.Errorf("Unexpected error: %v", err)
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nondefault",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nondefault",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nondefault",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nondefault",
						Namespace: "default",
					},
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
}

func TestCRDValidation(t *testing.T) {
	t.Parallel()

	c, _ := createKindCluster(t)

	type test struct {
		obj     client.Object
		wantErr bool
	}

	run := func(t *testing.T, tests map[string]test) {
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				err := c.Create(t.Context(), tc.obj)
				switch {
				case err == nil && !tc.wantErr:
					// OK
				case err != nil && !tc.wantErr:
					t.Errorf("Unexpected error: %v", err)
				case err == nil && tc.wantErr:
					t.Errorf("Want error, but got none")
				case err != nil && tc.wantErr:
					t.Log(err)
					// OK
				}
			})
		}
	}
	t.Run("ClusterNodeMonitoring", func(t *testing.T) {
		tests := map[string]test{
			"scrape interval missing": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-interval-missing",
						Namespace: "default",
					},
					Spec: monitoringv1.ClusterNodeMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeNodeEndpoint{
							{},
						},
					},
				},
			},
			"scrape interval malformed": {
				obj: &monitoringv1.ClusterNodeMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-interval-malformed",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-timeout-malformed",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-timeout-greater-than-interval",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name: "namespace-on-secret-references",
					},
					Spec: monitoringv1.ClusterPodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
			},
		}
		run(t, tests)
	})

	t.Run("OperatorConfig", func(t *testing.T) {
		tests := map[string]test{
			"empty": {
				obj:     &monitoringv1.OperatorConfig{},
				wantErr: true,
			},
			"invalid name": {
				obj: &monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-name",
						Namespace: "gmp-public",
					},
				},
				wantErr: true,
			},
			"invalid namespace": {
				obj: &monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "invalid-namespace",
					},
				},
				wantErr: true,
			},
			"minimal": {
				obj: &monitoringv1.OperatorConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config",
						Namespace: "gmp-public",
					},
				},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "minimal",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "port-missing",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "port-using-regex",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "duplicate-port",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-interval-missing",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-interval-malformed",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-timeout-malformed",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scrape-timeout-greater-than-interval",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "remapping-onto-prometheus-target-label",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "remapping-onto-bad-label-name",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "relabeling-valid",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labelmap-forbidden",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "protected-replace-label",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "protected-labelkeep",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "protected-labeldrop",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labeldrop-default-regex",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labelkeep-default-regex",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "empty-action-valid",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-url",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									ProxyConfig: monitoringv1.ProxyConfig{
										ProxyURL: "_:_",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"proxy URL with password": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "proxy-url-with-password",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									ProxyConfig: monitoringv1.ProxyConfig{
										ProxyURL: "http://user:password@foo.bar/",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"metadata labels empty": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "metadata-labels-empty",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tls-setting-invalid",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									TLS: &monitoringv1.TLS{
										MinVersion: "TLS09",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"TLS setting valid": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "proxy-url-with-password",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									TLS: &monitoringv1.TLS{
										MinVersion: "TLS13",
									},
								},
							},
						},
					},
				},
			},
			"authentication basic header": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "authentication-basic-header",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
									Authorization: &monitoringv1.Auth{
										Type: "Basic",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"basic auth and authorization header": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "authentication-basic-header",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
				},
				wantErr: true,
			},
			"authorization header and oauth2": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "authentication-basic-header",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
				},
				wantErr: true,
			},
			"client cert only": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "client-cert-only",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
				},
				wantErr: true,
			},
			"client key only": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "client-key-only",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
				},
				wantErr: true,
			},
			"client cert/key pair": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "client-cert-key-pair",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
			},
			"namespace on secret reference": {
				obj: &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "namespace-on-secret-references",
						Namespace: "default",
					},
					Spec: monitoringv1.PodMonitoringSpec{
						Endpoints: []monitoringv1.ScrapeEndpoint{
							{
								Interval: "1m",
								Port:     intstr.FromString("metrics"),
								HTTPClientConfig: monitoringv1.HTTPClientConfig{
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
				wantErr: true,
			},
		}
		run(t, tests)
	})
	t.Run("Rules", func(t *testing.T) {
		tests := map[string]test{
			"minimal-alerting": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "minimal-alerting",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Alert: "Any-characters:Allowed!?#@",
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "minimal-recording",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
									},
								},
							},
						},
					},
				},
				wantErr: false,
			},
			"invalid-interval": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-interval",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Interval: "foo",
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"invalid-rule-name": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-rule-name",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Record: "dots.not.allowed",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"invalid-rule-name-dash": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-rule-name-dash",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Record: "dashes-not-allowed",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"valid-rule-name-colon": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "valid-rule-name-colon",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Record: "colon:allowed",
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-annotation",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Record: "test",
										Annotations: map[string]string{
											"test": "annotation",
										},
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"valid-annotation": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "valid-annotation",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Alert: "test",
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
			"alert-and-record-both-set": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "alert-and-record-set",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{
										Alert:  "alert",
										Record: "record",
									},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"neither-alert-nor-record-set": {
				obj: &monitoringv1.Rules{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "neither-alert-nor-record-set",
						Namespace: "default",
					},
					Spec: monitoringv1.RulesSpec{
						Groups: []monitoringv1.RuleGroup{
							{
								Rules: []monitoringv1.Rule{
									{},
								},
							},
						},
					},
				},
				wantErr: true,
			},
			"TargetLabels/from-kubernetes-labels": {
				obj: &monitoringv1.ClusterPodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "from-kubernetes-labels",
						Namespace: "default",
					},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "empty-to",
						Namespace: "default",
					},
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
