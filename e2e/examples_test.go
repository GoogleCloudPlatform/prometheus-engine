// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

func TestInjectGMPSidecarExample(t *testing.T) {
	ctx := contextWithDeadline(t)

	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error setting up cluster: %s", err)
	}

	// 1. Create example-deployment in default namespace.
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "example-service",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "example-service",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "example-service",
							Image: "nginx:latest", // Just a dummy image.
						},
					},
				},
			},
		},
	}

	if err := kubeClient.Create(ctx, deployment); err != nil {
		t.Fatalf("error creating example-deployment: %s", err)
	}
	defer func() {
		// Use a fresh context, so cleanup works even if ctx has already expired.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		_ = kubeClient.Delete(cleanupCtx, deployment)
		_ = kubeClient.Delete(cleanupCtx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			},
		})
	}()

	// The script reads the external labels from the collector configuration. The operator
	// only adds them after defaulting the OperatorConfig, which can happen after it's ready.
	if err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		cm := &corev1.ConfigMap{}
		if getErr := kubeClient.Get(ctx, client.ObjectKey{Name: operator.NameCollector, Namespace: operator.DefaultOperatorNamespace}, cm); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		}
		type globalConfig struct {
			ExternalLabels map[string]string `yaml:"external_labels"`
		}
		var cfg struct {
			Global globalConfig `yaml:"global"`
		}
		if yamlErr := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &cfg); yamlErr != nil {
			return false, yamlErr
		}
		labels := cfg.Global.ExternalLabels
		return labels["cluster"] == cluster && labels["location"] == location && labels["project_id"] == projectID, nil
	}); err != nil {
		t.Fatalf("error waiting for collector external labels: %s", err)
	}

	// 2. Run the bash script.
	cmd := exec.CommandContext(ctx, "../examples/inject-gmp-sidecar.sh")
	// Kind clusters have no Google Cloud credentials, so disable the export to let Prometheus start.
	cmd.Env = append(os.Environ(), "PROMETHEUS_EXTRA_ARGS=--export.disable")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("error running inject-gmp-sidecar.sh: %s\n%s", err, string(out))
	}
	t.Logf("script output: %s", string(out))

	// 3. Verify sidecars are injected and ConfigMap is created.
	if err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		cm := &corev1.ConfigMap{}
		if getErr := kubeClient.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}, cm); getErr != nil {
			return false, nil //nolint:nilerr
		}
		if cm.Data["config.yaml"] == "" {
			return false, nil //nolint:nilerr
		}

		dep := &appsv1.Deployment{}
		if getErr := kubeClient.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}, dep); getErr != nil {
			return false, nil //nolint:nilerr
		}

		containers := dep.Spec.Template.Spec.Containers
		if len(containers) != 3 {
			return false, nil //nolint:nilerr
		}

		hasProm := false
		hasReloader := false
		for _, c := range containers {
			if c.Name == "prometheus" {
				hasProm = true
			}
			if c.Name == "config-reloader" {
				hasReloader = true
			}
		}

		return hasProm && hasReloader, nil
	}); err != nil {
		t.Fatalf("failed to verify injected sidecars or config map: %s", err)
	}

	// 4. Verify the patched pods start and become ready.
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, deployment.Namespace, deployment.Name); err != nil {
		t.Errorf("example-deployment is not ready: %s", err)
		debugOut := strings.Builder{}
		if err := kube.Debug(t.Context(), restConfig, kubeClient, deployment, &debugOut); err != nil {
			t.Fatalf("unable to debug: %s", err)
		}
		t.Fatalf("debug:\n%s", debugOut.String())
	}
}
