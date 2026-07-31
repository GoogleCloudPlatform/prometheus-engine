// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package operator

import (
	"context"
	"fmt"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func collectorUser(operatorNamespace string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", operatorNamespace, NameCollector)
}

func collectorSecretRBACHint(collectorNamespace, secretNamespace, secretName string) string {
	return fmt.Sprintf(
		"collector ServiceAccount (%s) cannot get secret %q in namespace %q. Grant access with a Role in namespace %q containing rules: [{apiGroups: [\"\"], resources: [\"secrets\"], resourceNames: [%q], verbs: [\"get\"]}] and a RoleBinding subject: ServiceAccount %q in namespace %q",
		collectorUser(collectorNamespace),
		secretName,
		secretNamespace,
		secretNamespace,
		secretName,
		NameCollector,
		collectorNamespace,
	)
}

func (r *collectionReconciler) checkCollectorSecretAccess(ctx context.Context, secret monitoringv1.SecretReference) (corev1.ConditionStatus, string) {
	review := &authorizationv1.SubjectAccessReview{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "gmp-collector-secret-access-",
		},
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User: collectorUser(r.opts.OperatorNamespace),
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: secret.Namespace,
				Verb:      "get",
				Resource:  "secrets",
				Name:      secret.Name,
			},
		},
	}
	if err := r.client.Create(ctx, review); err != nil {
		return corev1.ConditionUnknown, fmt.Sprintf(
			"unable to verify collector access to secret %q in namespace %q: %v",
			secret.Name, secret.Namespace, err,
		)
	}
	if review.Status.Allowed {
		return corev1.ConditionTrue, ""
	}
	return corev1.ConditionFalse, collectorSecretRBACHint(r.opts.OperatorNamespace, secret.Namespace, secret.Name)
}

func (r *collectionReconciler) collectorSecretAccessCondition(ctx context.Context, m monitoringv1.PodMonitoringCRD) *monitoringv1.MonitoringCondition {
	secrets := m.ReferencedSecrets()
	if len(secrets) == 0 {
		return nil
	}
	var denied []string
	for _, secret := range secrets {
		status, msg := r.checkCollectorSecretAccess(ctx, secret)
		switch status {
		case corev1.ConditionUnknown:
			return &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.CollectorSecretAccess,
				Status:  corev1.ConditionUnknown,
				Message: msg,
			}
		case corev1.ConditionFalse:
			denied = append(denied, msg)
		}
	}
	if len(denied) > 0 {
		return &monitoringv1.MonitoringCondition{
			Type:    monitoringv1.CollectorSecretAccess,
			Status:  corev1.ConditionFalse,
			Reason:  "MissingRBAC",
			Message: strings.Join(denied, "; "),
		}
	}
	return &monitoringv1.MonitoringCondition{
		Type:   monitoringv1.CollectorSecretAccess,
		Status: corev1.ConditionTrue,
	}
}

func applyMonitoringConditions(gen int64, now metav1.Time, status *monitoringv1.MonitoringStatus, conds ...*monitoringv1.MonitoringCondition) bool {
	updated := false
	for _, cond := range conds {
		if cond == nil {
			continue
		}
		if status.SetMonitoringCondition(gen, now, cond) {
			updated = true
		}
	}
	return updated
}
