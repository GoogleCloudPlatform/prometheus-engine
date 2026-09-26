// Copyright 2024 Google LLC
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
	"os/exec"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestInjectGMPSidecarExample(t *testing.T) {
	ctx := contextWithDeadline(t)

	kubeClient, _, err := setupCluster(ctx, t)
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
		_ = kubeClient.Delete(ctx, deployment)
		_ = kubeClient.Delete(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-deployment",
				Namespace: deployment.Namespace,
			},
		})
	}()

	// 2. Run the bash script.
	cmd := exec.CommandContext(ctx, "../examples/inject-gmp-sidecar.sh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("error running inject-gmp-sidecar.sh: %s\n%s", err, string(out))
	}
	t.Logf("script output: %s", string(out))

	// 3. Verify sidecars are injected and ConfigMap is created.
	if err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		cm := &corev1.ConfigMap{}
		if getErr := kubeClient.Get(ctx, client.ObjectKey{Name: "example-deployment", Namespace: "default"}, cm); getErr != nil {
			return false, nil //nolint:nilerr
		}
		if cm.Data["config.yaml"] == "" {
			return false, nil //nolint:nilerr
		}

		dep := &appsv1.Deployment{}
		if getErr := kubeClient.Get(ctx, client.ObjectKey{Name: "example-deployment", Namespace: "default"}, dep); getErr != nil {
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
}
