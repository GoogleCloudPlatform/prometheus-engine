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
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createKindCluster(t *testing.T) client.Client {
	ctx := context.Background()

	t.Helper()

	// Add a randomized suffix to the test cluster name to reduce collisions
	clusterName := fmt.Sprintf("crd-test-%s", rand.String(6))

	tmp := t.TempDir()
	kubeconfigPath := filepath.Join(tmp, "kubeconfig")

	// Create a cluster with a randomized name, and save the kubeconfig in a temporary directory scoped to this test
	createClusterOutput, err := exec.CommandContext(ctx, "kind", "create", "cluster", "--name", clusterName, "--kubeconfig", kubeconfigPath).CombinedOutput()
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("%s\n", createClusterOutput)

	// After the cluster has been created successfully, enqueue deletion of the cluster when the test concludes
	t.Cleanup(cleanupKindCluster(t, clusterName))

	// Apply GMP CRDs
	applyCRDsOutput, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "../manifests/setup.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("%s\b%v", applyCRDsOutput, err)
	}
	t.Logf("%s\n", applyCRDsOutput)

	// Wait for CRDs to be created - there seems to be race condition without this wait
	if _, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "wait", "customresourcedefinition.apiextensions.k8s.io/clusternodemonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/clusterpodmonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/clusterrules.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/globalrules.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/operatorconfigs.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/podmonitorings.monitoring.googleapis.com", "customresourcedefinition.apiextensions.k8s.io/rules.monitoring.googleapis.com", "--for=create").CombinedOutput(); err != nil {
		t.Fatalf("%v", err)
	}

	// Load the test cluster kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Create a client for the test cluster
	c, err := newKubeClient(config)
	if err != nil {
		t.Error(err)
	}
	return c
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

func TestPodMonitoringValidation(t *testing.T) {
	ctx := context.Background()

	t.Parallel()

	c := createKindCluster(t)

	type test struct {
		pm      *monitoringv1.PodMonitoring
		wantErr bool
	}
	tests := map[string]test{
		"empty": {
			pm:      &monitoringv1.PodMonitoring{},
			wantErr: true,
		},
		"minimal": {
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
		"scrape interval missing": {
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
		"metric relabeling: labelmap forbidden": {
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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
			pm: &monitoringv1.PodMonitoring{
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := c.Create(ctx, tc.pm)
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
