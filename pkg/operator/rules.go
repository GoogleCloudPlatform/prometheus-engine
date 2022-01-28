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
	"k8s.io/apimachinery/pkg/runtime"
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

const (
	nameRulesGenerated = "rules-generated"
)

func setupRulesControllers(op *Operator) error {
	// The singleton OperatorConfig is the request object we reconcile against.
	objRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: op.opts.PublicNamespace,
			Name:      NameOperatorConfig,
		},
	}
	// Default OperatorConfig filter.
	objFilterOperatorConfig := namespacedNamePredicate{
		namespace: op.opts.PublicNamespace,
		name:      NameOperatorConfig,
	}
	// Rule-evaluator rules ConfigMap filter.
	objFilterRulesGenerated := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      nameRulesGenerated,
	}

	// Reconcile the generated rules that are used by the rule-evaluator deployment.
	err := ctrl.NewControllerManagedBy(op.manager).
		Named("rules").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		// OperatorConfig is our root resource that ensures we reconcile
		// at least once initially.
		For(
			&monitoringv1alpha1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		// Any update to a Rules object requires re-generating the config.
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.Rules{}},
			enqueueConst(objRequest),
		).
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.ClusterRules{}},
			enqueueConst(objRequest),
		).
		// The configuration we generate for the rule-evaluator.
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterRulesGenerated),
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
	logr.FromContext(ctx).Info("reconciling rules")

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
				LabelAppName: NameRuleEvaluator,
			},
		},
		// Ensure there's always at least an empty dummy file as the evaluator
		// expects at least one match.
		Data: map[string]string{
			"empty.yaml": "",
		},
	}

	// Generate a final rule file for each Rules resource.
	//
	// Depending on the scope level (global, cluster, namespace) the rules will be generated
	// so that queries are constrained to the appropriate project_id, cluster, and namespace
	// labels and that they are preserved through query aggregations and appear on the
	// output data.
	//
	// The location is not scoped as it's not a meaningful boundary for "human access"
	// to data as clusters may span locations.
	var rulesList monitoringv1alpha1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return errors.Wrap(err, "list rules")
	}
	for _, rs := range rulesList.Items {
		result, err := generateRules(&rs, r.opts)
		if err != nil {
			// TODO(freinartz): update resource condition.
			logger.Error(err, "converting rules failed", "rules_namespace", rs.Namespace, "rules_name", rs.Name)
		}
		filename := fmt.Sprintf("rules__%s__%s.yaml", rs.Namespace, rs.Name)
		cm.Data[filename] = result
	}

	var clusterRulesList monitoringv1alpha1.ClusterRulesList
	if err := r.client.List(ctx, &clusterRulesList); err != nil {
		return errors.Wrap(err, "list cluster rules")
	}
	for _, rs := range clusterRulesList.Items {
		result, err := generateClusterRules(&rs, r.opts)
		if err != nil {
			// TODO(freinartz): update resource condition.
			logger.Error(err, "converting rules failed", "clusterrules_name", rs.Name)
		}
		filename := fmt.Sprintf("clusterrules__%s.yaml", rs.Name)
		cm.Data[filename] = string(result)
	}

	var globalRulesList monitoringv1alpha1.GlobalRulesList
	if err := r.client.List(ctx, &globalRulesList); err != nil {
		return errors.Wrap(err, "list global rules")
	}
	for _, rs := range globalRulesList.Items {
		result, err := generateGlobalRules(&rs)
		if err != nil {
			// TODO(freinartz): update resource condition.
			logger.Error(err, "converting rules failed", "globalrules_name", rs.Name)
		}
		filename := fmt.Sprintf("globalrules__%s.yaml", rs.Name)
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

func generateRules(apiRules *monitoringv1alpha1.Rules, opts Options) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", errors.Wrap(err, "converting rules failed")
	}
	if err := rules.Scope(&rs, map[string]string{
		export.KeyProjectID: opts.ProjectID,
		export.KeyCluster:   opts.Cluster,
		export.KeyLocation:  opts.Location,
		export.KeyNamespace: apiRules.Namespace,
	}); err != nil {
		return "", errors.Wrap(err, "isolating rules failed")
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", errors.Wrap(err, "marshalling rules failed")
	}
	return string(result), nil
}

func generateClusterRules(apiRules *monitoringv1alpha1.ClusterRules, opts Options) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", errors.Wrap(err, "converting rules failed")
	}
	if err := rules.Scope(&rs, map[string]string{
		export.KeyProjectID: opts.ProjectID,
		export.KeyLocation:  opts.Location,
		export.KeyCluster:   opts.Cluster,
	}); err != nil {
		return "", errors.Wrap(err, "isolating rules failed")
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", errors.Wrap(err, "marshalling rules failed")
	}
	return string(result), nil
}

func generateGlobalRules(apiRules *monitoringv1alpha1.GlobalRules) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", errors.Wrap(err, "converting rules failed")
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", errors.Wrap(err, "marshalling rules failed")
	}
	return string(result), nil
}

type rulesValidator struct {
	opts Options
}

func (v *rulesValidator) ValidateCreate(ctx context.Context, o runtime.Object) error {
	_, err := generateRules(o.(*monitoringv1alpha1.Rules), v.opts)
	return err
}

func (v *rulesValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) error {
	return v.ValidateCreate(ctx, o)
}

func (v *rulesValidator) ValidateDelete(ctx context.Context, o runtime.Object) error {
	return nil
}

type clusterRulesValidator struct {
	opts Options
}

func (v *clusterRulesValidator) ValidateCreate(ctx context.Context, o runtime.Object) error {
	_, err := generateClusterRules(o.(*monitoringv1alpha1.ClusterRules), v.opts)
	return err
}

func (v *clusterRulesValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) error {
	return v.ValidateCreate(ctx, o)
}

func (v *clusterRulesValidator) ValidateDelete(ctx context.Context, o runtime.Object) error {
	return nil
}

type globalRulesValidator struct{}

func (v *globalRulesValidator) ValidateCreate(ctx context.Context, o runtime.Object) error {
	_, err := generateGlobalRules(o.(*monitoringv1alpha1.GlobalRules))
	return err
}

func (v *globalRulesValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) error {
	return v.ValidateCreate(ctx, o)
}

func (v *globalRulesValidator) ValidateDelete(ctx context.Context, o runtime.Object) error {
	return nil
}
