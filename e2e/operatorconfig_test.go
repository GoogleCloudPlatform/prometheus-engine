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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestOperatorConfigVPAUnavailable validates that enabling vertical pod autoscaling
// on a cluster without VPA support is admitted, reported through Warning Events,
// and does not block reconciliation of the remaining OperatorConfig fields.
func TestOperatorConfigVPAUnavailable(t *testing.T) {
	ctx := contextWithDeadline(t)
	kubeClient, _, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const (
		externalLabelKey   = "vpa_unavailable_test"
		externalLabelValue = "reconciled"
	)

	t.Log("enabling vertical pod autoscaling in operatorconfig")
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		config := monitoringv1.OperatorConfig{
			Name:      operator.NameOperatorConfig,
			Namespace: operator.DefaultPublicNamespace,
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			return err
		}
		config.Scaling.VPA.Enabled = true
		if config.Rules.ExternalLabels == nil {
			config.Rules.ExternalLabels = map[string]string{}
		}
		config.Rules.ExternalLabels[externalLabelKey] = externalLabelValue
		return kubeClient.Update(ctx, &config)
	}); err != nil {
		t.Fatalf("update operatorconfig: %s", err)
	}

	isVPAWarning := func(ev *eventsv1.Event) bool {
		return ev.Type == corev1.EventTypeWarning &&
			ev.Reason == "VPANotAvailable" &&
			ev.Action == "Ignored"
	}

	t.Log("waiting for warning events and rule-evaluator config")
	if err := wait.PollUntilContextCancel(ctx, pollDuration, true, func(ctx context.Context) (bool, error) {
		var sysEvents eventsv1.EventList
		if err := kubeClient.List(ctx, &sysEvents, client.InNamespace(operator.DefaultOperatorNamespace)); err != nil {
			return false, fmt.Errorf("list system events: %w", err)
		}
		hasWorkloadEvent := false
		for i := range sysEvents.Items {
			ev := &sysEvents.Items[i]
			if isVPAWarning(ev) &&
				ev.Regarding.Kind == "Deployment" &&
				ev.Regarding.Name == operator.NameOperator &&
				ev.Related != nil &&
				ev.Related.Kind == "OperatorConfig" &&
				ev.Related.Namespace == operator.DefaultPublicNamespace &&
				ev.Related.Name == operator.NameOperatorConfig {
				hasWorkloadEvent = true
				break
			}
		}
		if !hasWorkloadEvent {
			return false, nil
		}

		var publicEvents eventsv1.EventList
		if err := kubeClient.List(ctx, &publicEvents, client.InNamespace(operator.DefaultPublicNamespace)); err != nil {
			return false, fmt.Errorf("list public events: %w", err)
		}
		hasResourceEvent := false
		for i := range publicEvents.Items {
			ev := &publicEvents.Items[i]
			if isVPAWarning(ev) &&
				ev.Regarding.Kind == "OperatorConfig" &&
				ev.Regarding.Namespace == operator.DefaultPublicNamespace &&
				ev.Regarding.Name == operator.NameOperatorConfig {
				hasResourceEvent = true
				break
			}
		}
		if !hasResourceEvent {
			return false, nil
		}

		// The rest of the OperatorConfig must still be reconciled.
		var cm corev1.ConfigMap
		if err := kubeClient.Get(ctx, client.ObjectKey{
			Namespace: operator.DefaultOperatorNamespace,
			Name:      operator.NameRuleEvaluator,
		}, &cm); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("get rule-evaluator config: %w", err)
		}
		return strings.Contains(cm.Data["config.yaml"], fmt.Sprintf("%s: %s", externalLabelKey, externalLabelValue)), nil
	}); err != nil {
		t.Fatalf("waiting for vertical pod autoscaling warning events and rule-evaluator config: %s", err)
	}
}
