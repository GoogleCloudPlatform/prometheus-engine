// Copyright 2021 Google LLC
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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/rules"
)

func setupRulesControllers(op *Operator) error {
	// Canonical request for any events that require re-generating the rule config map.
	objRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: op.opts.OperatorNamespace,
			Name:      nameRulesGenerated,
		},
	}
	// Canonical filter to only capture events for the generated rules config map.
	objFilter := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      nameRulesGenerated,
	}
	// Reconcile the generated rules that are used by the rule-evaluator deployment.
	err := ctrl.NewControllerManagedBy(op.manager).
		Named("rules").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		For(
			&corev1.ConfigMap{},
			builder.WithPredicates(objFilter),
		).
		// Any update to a Rules object requires re-generating the config.
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.Rules{}},
			enqueueConst(objRequest),
		).
		Complete(newRulesReconciler(op.manager.GetClient(), op.opts))
	if err != nil {
		return errors.Wrap(err, "create rules config controller")
	}
	return nil
}

type rulesReconciler struct {
	client client.Client
	opts   Options
}

func newRulesReconciler(c client.Client, opts Options) *rulesReconciler {
	return &rulesReconciler{
		client: c,
		opts:   opts,
	}
}

func (r *rulesReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	if err := r.ensureRuleConfigs(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule configmaps")
	}
	return reconcile.Result{}, nil
}

func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context) error {
	logger := logr.FromContext(ctx)

	// Re-generate the configmap that's loaded by the rule-evaluator.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      nameRulesGenerated,
			Labels: map[string]string{
				"app.kubernetes.io/name": "rule-evaluator",
			},
		},
		// Ensure there's always at least an empty dummy file as the evaluator
		// expects at least one match.
		Data: map[string]string{
			"empty.yaml": "",
		},
	}

	// Generate a final rule file for each Rules resource.
	var rulesList monitoringv1alpha1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return errors.Wrap(err, "list rules")
	}
	for _, apiRules := range rulesList.Items {
		rulesLogger := logger.WithValues("rules_namespace", apiRules.Namespace, "rules_name", apiRules.Name)

		rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
		if err != nil {
			rulesLogger.Error(err, "converting rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		lset := map[string]string{}
		// Populate isolation level from the defined scope.
		switch apiRules.Spec.Scope {
		case monitoringv1alpha1.ScopeCluster:
			lset[export.KeyProjectID] = r.opts.ProjectID
			lset[export.KeyCluster] = r.opts.Cluster
		case monitoringv1alpha1.ScopeNamespace:
			lset[export.KeyProjectID] = r.opts.ProjectID
			lset[export.KeyCluster] = r.opts.Cluster
			lset[export.KeyNamespace] = apiRules.Namespace
		default:
			rulesLogger.Error(errors.New("scope type is not defiend"), "unexpected scope", "scope", apiRules.Spec.Scope)
			// TODO(freinartz): update resource condition.
			continue
		}
		if err := rules.Scope(&rs, lset); err != nil {
			rulesLogger.Error(err, "isolating rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		result, err := yaml.Marshal(rs)
		if err != nil {
			rulesLogger.Error(err, "marshalling rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		filename := fmt.Sprintf("%s__%s.yaml", apiRules.Namespace, apiRules.Name)
		cm.Data[filename] = string(result)
	}

	// Create or update generated rule ConfigMap.
	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return errors.Wrap(err, "create generated rules")
		}
	} else if err != nil {
		return errors.Wrap(err, "update generated rules")
	}
	return nil
}
