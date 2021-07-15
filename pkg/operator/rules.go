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

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

func setupRulesControllers(op *Operator) error {
	const name = "rules"
	logger := log.With(op.logger, "controller", name)

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
		Named(name).
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
		Complete(newRulesReconciler(logger, op.manager.GetClient(), op.opts))
	if err != nil {
		return errors.Wrap(err, "create collector config controller")
	}
	return nil
}

type rulesReconciler struct {
	logger log.Logger
	client client.Client
	opts   Options
}

func newRulesReconciler(logger log.Logger, c client.Client, opts Options) *rulesReconciler {
	return &rulesReconciler{
		logger: logger,
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
