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
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestWebhooksNoRBAC validates that the operator works without any webhook RBAC policies.
func TestWebhooksNoRBAC(t *testing.T) {
	ctx := context.Background()
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	if err := kubeClient.Delete(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gmp-system:operator:webhook-admin",
		},
	}); err != nil {
		t.Fatalf("error deleting cluster role: %s", err)
	}
	if err := kubeClient.Delete(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gmp-system:operator:webhook-admin",
		},
	}); err != nil {
		t.Fatalf("error deleting cluster role binding: %s", err)
	}

	// Restart the GMP operator since it is already healthy before we delete the RBAC policies.
	t.Log("restarting operator")
	if err := deploymentRestart(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		t.Fatalf("error restarting operator. err: %s", err)
	}

	t.Log("waiting for operator to be deployed")
	if err := deploy.WaitForOperatorReady(ctx, kubeClient); err != nil {
		t.Fatalf("error waiting for operator deployment to be ready: %s", err)
	}

	if err := wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		logs, err := deploy.OperatorLogs(ctx, restConfig, kubeClient, operator.DefaultOperatorNamespace)
		if err != nil {
			t.Logf("unable to get operator logs: %s", err)
			return false, nil
		}

		t.Logf("waiting for operator logs to contain RBAC message")
		return strings.Contains(logs, "delete legacy ValidatingWebHookConfiguration was not allowed"), nil
	}); err != nil {
		t.Fatalf("unable to check operator logs: %s", err)
	}
}

func deploymentRestart(ctx context.Context, kubeClient client.Client, namespace, name string) error {
	deploy := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&deploy), &deploy); err != nil {
		return err
	}
	deployPatch := deploy.DeepCopy()
	if deployPatch.Spec.Template.Annotations == nil {
		deployPatch.Spec.Template.Annotations = make(map[string]string)
	}
	deployPatch.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	if err := kubeClient.Patch(ctx, deployPatch, client.MergeFrom(&deploy)); err != nil {
		return err
	}

	return nil
}
