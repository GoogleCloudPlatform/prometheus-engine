// Copyright 2022 Google LLC
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
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
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
			&monitoringv1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		// Any update to a Rules object requires re-generating the config.
		Watches(
			&source.Kind{Type: &monitoringv1.GlobalRules{}},
			enqueueConst(objRequest),
		).
		Watches(
			&source.Kind{Type: &monitoringv1.ClusterRules{}},
			enqueueConst(objRequest),
		).
		Watches(
			&source.Kind{Type: &monitoringv1.Rules{}},
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
		return fmt.Errorf("create rules config controller: %w", err)
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

func (r *rulesReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger, _ := logr.FromContext(ctx)
	logger.Info("reconciling rules")

	var config monitoringv1.OperatorConfig
	// Fetch OperatorConfig if it exists.
	if err := r.client.Get(ctx, req.NamespacedName, &config); apierrors.IsNotFound(err) {
		logger.Info("no operatorconfig created yet")
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("get operatorconfig for incoming: %q: %w", req.String(), err)
	}

	var projectID, location, cluster = resolveLabels(r.opts, config.Rules.ExternalLabels)

	if err := r.ensureRuleConfigs(ctx, projectID, location, cluster); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure rule configmaps: %w", err)
	}
	return reconcile.Result{}, nil
}

func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context, projectID, location, cluster string) error {
	logger, _ := logr.FromContext(ctx)

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
	var rulesList monitoringv1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	for _, rs := range rulesList.Items {
		result, err := generateRules(&rs, projectID, location, cluster)
		if err != nil {
			// TODO(freinartz): update resource condition.
			logger.Error(err, "converting rules failed", "rules_namespace", rs.Namespace, "rules_name", rs.Name)
		}
		filename := fmt.Sprintf("rules__%s__%s.yaml", rs.Namespace, rs.Name)
		cm.Data[filename] = result
	}

	var clusterRulesList monitoringv1.ClusterRulesList
	if err := r.client.List(ctx, &clusterRulesList); err != nil {
		return fmt.Errorf("list cluster rules: %w", err)
	}
	for _, rs := range clusterRulesList.Items {
		result, err := generateClusterRules(&rs, projectID, location, cluster)
		if err != nil {
			// TODO(freinartz): update resource condition.
			logger.Error(err, "converting rules failed", "clusterrules_name", rs.Name)
		}
		filename := fmt.Sprintf("clusterrules__%s.yaml", rs.Name)
		cm.Data[filename] = string(result)
	}

	var globalRulesList monitoringv1.GlobalRulesList
	if err := r.client.List(ctx, &globalRulesList); err != nil {
		return fmt.Errorf("list global rules: %w", err)
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
			return fmt.Errorf("create generated rules: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update generated rules: %w", err)
	}
	return nil
}

func generateRules(apiRules *monitoringv1.Rules, projectID, location, cluster string) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", fmt.Errorf("converting rules failed: %w", err)
	}
	if err := rules.Scope(&rs, map[string]string{
		export.KeyProjectID: projectID,
		export.KeyLocation:  location,
		export.KeyCluster:   cluster,
		export.KeyNamespace: apiRules.Namespace,
	}); err != nil {
		return "", fmt.Errorf("isolating rules failed: %w", err)
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", fmt.Errorf("marshalling rules failed: %w", err)
	}
	return string(result), nil
}

func generateClusterRules(apiRules *monitoringv1.ClusterRules, projectID, location, cluster string) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", fmt.Errorf("converting rules failed: %w", err)
	}
	if err := rules.Scope(&rs, map[string]string{
		export.KeyProjectID: projectID,
		export.KeyLocation:  location,
		export.KeyCluster:   cluster,
	}); err != nil {
		return "", fmt.Errorf("isolating rules failed: %w", err)
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", fmt.Errorf("marshalling rules failed: %w", err)
	}
	return string(result), nil
}

func generateGlobalRules(apiRules *monitoringv1.GlobalRules) (string, error) {
	rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
	if err != nil {
		return "", fmt.Errorf("converting rules failed: %w", err)
	}
	result, err := yaml.Marshal(rs)
	if err != nil {
		return "", fmt.Errorf("marshalling rules failed: %w", err)
	}
	return string(result), nil
}

type rulesValidator struct {
	opts Options
}

func (v *rulesValidator) ValidateCreate(ctx context.Context, o runtime.Object) error {
	_, err := generateRules(o.(*monitoringv1.Rules), "test_project", "test_location", "test_cluster")
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
	_, err := generateClusterRules(o.(*monitoringv1.ClusterRules), "test_project", "test_location", "test_cluster")
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
	_, err := generateGlobalRules(o.(*monitoringv1.GlobalRules))
	return err
}

func (v *globalRulesValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) error {
	return v.ValidateCreate(ctx, o)
}

func (v *globalRulesValidator) ValidateDelete(ctx context.Context, o runtime.Object) error {
	return nil
}
