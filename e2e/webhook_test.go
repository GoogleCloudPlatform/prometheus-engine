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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// TestWebhooksNoRBAC validates that the operator works without any webhook RBAC policies.
func TestWebhooksNoRBAC(t *testing.T) {
	ctx := context.Background()
	clientSet, _, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	if err := clientSet.RbacV1().ClusterRoles().Delete(ctx, "gmp-system:operator:webhook-admin", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("error deleting cluster role: %s", err)
	}
	if err := clientSet.RbacV1().ClusterRoleBindings().Delete(ctx, "gmp-system:operator:webhook-admin", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("error deleting cluster role binding: %s", err)
	}

	restConfig, err := newRestConfig()
	if err != nil {
		t.Fatalf("error creating rest config: %s", err)
	}

	kubeClient, err := newKubeClient(restConfig)
	if err != nil {
		t.Fatalf("error creating client: %s", err)
	}

	// Restart the GMP operator since it is already healthy before we delete the RBAC policies.
	t.Log("restarting operator")
	if err := deploymentRestart(ctx, clientSet, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		t.Fatalf("error restarting operator. err: %s", err)
	}

	t.Log("waiting for operator to be deployed")
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		t.Fatalf("error waiting for operator deployment to be ready: %s", err)
	}

	t.Log("waiting for operator to be ready")
	if err := wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		logs, err := deploy.OperatorLogs(ctx, kubeClient, clientSet, operator.DefaultOperatorNamespace)
		if err != nil {
			t.Logf("unable to get operator logs: %s", err)
			return false, nil
		}
		t.Logf("waiting for operator logs to contain start message")
		return strings.Contains(logs, "starting GMP operator"), nil
	}); err != nil {
		t.Fatalf("unable to check operator ready: %s", err)
	}

	if err := wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		logs, err := deploy.OperatorLogs(ctx, kubeClient, clientSet, operator.DefaultOperatorNamespace)
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

func deploymentRestart(ctx context.Context, clientSet kubernetes.Interface, namespace, name string) error {
	restartAtPatch := `{"spec": {"template": {"metadata": {"annotations": {"kubectl.kubernetes.io/restartedAt": "%s"}}}}}`
	restartNowPatch := []byte(fmt.Sprintf(restartAtPatch, time.Now().Format(time.RFC3339)))
	_, err := clientSet.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, restartNowPatch, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	return nil
}
