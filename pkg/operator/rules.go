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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
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
			&monitoringv1.GlobalRules{},
			enqueueConst(objRequest),
		).
		Watches(
			&monitoringv1.ClusterRules{},
			enqueueConst(objRequest),
		).
		Watches(
			&monitoringv1.Rules{},
			enqueueConst(objRequest),
		).
		// The configuration we generate for the rule-evaluator.
		Watches(
			&corev1.ConfigMap{},
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

	projectID, location, cluster := resolveLabels(r.opts.ProjectID, r.opts.Location, r.opts.Cluster, config.Rules.ExternalLabels)

	if err := r.ensureRuleConfigs(ctx, projectID, location, cluster, config.Features.Config.Compression); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure rule configmaps: %w", err)
	}

	if err := r.scaleRuleConsumers(ctx); err != nil {
		return reconcile.Result{}, fmt.Errorf("scale rule consumers: %w", err)
	}

	return reconcile.Result{}, nil
}

func (r *rulesReconciler) scaleRuleConsumers(ctx context.Context) error {
	logger, _ := logr.FromContext(ctx)

	var desiredReplicas int32

	var hasAnyRules bool
	for _, check := range []ruleCheck{hasRules, hasClusterRules, hasGlobalRules} {
		hasRules, err := check(ctx, r.client)
		if err != nil {
			return err
		}
		if hasRules {
			hasAnyRules = true
			break
		}
	}
	if hasAnyRules {
		desiredReplicas = 1
	}

	scaleClient := r.client.SubResource("scale")

	alertManagerStatefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      "alertmanager",
		},
	}
	alertManagerScale := autoscalingv1.Scale{}
	if err := scaleClient.Get(ctx, &alertManagerStatefulSet, &alertManagerScale); apierrors.IsNotFound(err) {
		msg := fmt.Sprintf("Alertmanager StatefulSet not found, cannot scale to %d. In-cluster Alertmanager will not function.", desiredReplicas)
		logger.Error(err, msg)
	} else if err != nil {
		return err
	} else if alertManagerScale.Spec.Replicas != desiredReplicas {
		alertManagerScale.Spec.Replicas = desiredReplicas
		if err := scaleClient.Update(ctx, &alertManagerStatefulSet, client.WithSubResourceBody(&alertManagerScale)); err != nil {
			return err
		}
	}

	ruleEvaluatorDeployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      "rule-evaluator",
		},
	}
	ruleEvaluatorScale := autoscalingv1.Scale{}
	if err := scaleClient.Get(ctx, &ruleEvaluatorDeployment, &ruleEvaluatorScale); apierrors.IsNotFound(err) {
		msg := fmt.Sprintf("Rule Evaluator Deployment not found, cannot scale to %d. In-cluster Rule Evaluator will not function.", desiredReplicas)
		logger.Error(err, msg)
	} else if err != nil {
		return err
	} else if ruleEvaluatorScale.Spec.Replicas != desiredReplicas {
		ruleEvaluatorScale.Spec.Replicas = desiredReplicas
		if err := scaleClient.Update(ctx, &ruleEvaluatorDeployment, client.WithSubResourceBody(&ruleEvaluatorScale)); err != nil {
			return err
		}
	}
	return nil
}

type ruleCheck func(context.Context, client.Client) (bool, error)

func hasRules(ctx context.Context, c client.Client) (bool, error) {
	var rules monitoringv1.RulesList
	if err := c.List(ctx, &rules); err != nil {
		return false, err
	}
	return len(rules.Items) > 0, nil
}

func hasClusterRules(ctx context.Context, c client.Client) (bool, error) {
	var rules monitoringv1.ClusterRulesList
	if err := c.List(ctx, &rules); err != nil {
		return false, err
	}
	return len(rules.Items) > 0, nil
}

func hasGlobalRules(ctx context.Context, c client.Client) (bool, error) {
	var rules monitoringv1.GlobalRulesList
	if err := c.List(ctx, &rules); err != nil {
		return false, err
	}
	return len(rules.Items) > 0, nil
}

// ensureRuleConfigs updates the Prometheus Rules ConfigMap.
type RulesConfigUpdateStatus struct {
	ConfigMapResults map[string]error
}

func retryOperation(op func() error, maxRetries int, delay time.Duration) error {
	var lastErr error
	for range maxRetries {
		if err := op(); err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return lastErr
}

func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context, projectID, location, cluster string, configCompression monitoringv1.CompressionType) error {
	logger, _ := logr.FromContext(ctx)

	const maxRetries = 3
	const retryDelay = 500 * time.Millisecond

	updateStatus := &RulesConfigUpdateStatus{ConfigMapResults: make(map[string]error)}

	// Create one ConfigMap per rule type (no splitting)
	// - rules (namespace-scoped Rules)
	// - clusterrules (cluster-scoped ClusterRules)
	// - globalrules (GlobalRules)

	// Process namespace-scoped Rules -> single "rules" ConfigMap
	var rulesList monitoringv1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	rulesData := make(map[string]string)
	for i := range rulesList.Items {
		rs := &rulesList.Items[i]
		result, err := rs.RuleGroupsConfig(projectID, location, cluster)
		if err != nil {
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("rules__%s__%s.yaml", rs.Namespace, rs.Name)
		var buf strings.Builder
		if err := setConfigMapDataRaw(&buf, configCompression, result); err != nil {
			return err
		}
		rulesData[filename] = buf.String()
	}
	if err := r.createOrUpdateConfigMap(ctx, "rules", rulesData, maxRetries, retryDelay, updateStatus); err != nil {
		return err
	}

	// Process cluster-scoped ClusterRules -> single "clusterrules" ConfigMap
	var clusterRulesList monitoringv1.ClusterRulesList
	if err := r.client.List(ctx, &clusterRulesList); err != nil {
		return fmt.Errorf("list cluster rules: %w", err)
	}
	clusterRulesData := make(map[string]string)
	for i := range clusterRulesList.Items {
		rs := &clusterRulesList.Items[i]
		result, err := rs.RuleGroupsConfig(projectID, location, cluster)
		if err != nil {
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("clusterrules__%s.yaml", rs.Name)
		var buf strings.Builder
		if err := setConfigMapDataRaw(&buf, configCompression, result); err != nil {
			return err
		}
		clusterRulesData[filename] = buf.String()
	}
	if err := r.createOrUpdateConfigMap(ctx, "clusterrules", clusterRulesData, maxRetries, retryDelay, updateStatus); err != nil {
		return err
	}

	// Process GlobalRules -> single "globalrules" ConfigMap
	var globalRulesList monitoringv1.GlobalRulesList
	if err := r.client.List(ctx, &globalRulesList); err != nil {
		return fmt.Errorf("list global rules: %w", err)
	}
	globalRulesData := make(map[string]string)
	for i := range globalRulesList.Items {
		rs := &globalRulesList.Items[i]
		result, err := rs.RuleGroupsConfig()
		if err != nil {
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("globalrules__%s.yaml", rs.Name)
		var buf strings.Builder
		if err := setConfigMapDataRaw(&buf, configCompression, result); err != nil {
			return err
		}
		globalRulesData[filename] = buf.String()
	}
	if err := r.createOrUpdateConfigMap(ctx, "globalrules", globalRulesData, maxRetries, retryDelay, updateStatus); err != nil {
		return err
	}

	// Log partial update status
	for name, err := range updateStatus.ConfigMapResults {
		if err != nil {
			logger.Error(err, "ConfigMap update failed", "configmap", name)
		}
	}

	// Return error if any operation failed
	var anyErr error
	for _, err := range updateStatus.ConfigMapResults {
		if err != nil {
			anyErr = err
		}
	}
	return anyErr
}

// createOrUpdateConfigMap creates or updates a single ConfigMap for a rule type.
func (r *rulesReconciler) createOrUpdateConfigMap(
	ctx context.Context,
	name string,
	data map[string]string,
	maxRetries int,
	retryDelay time.Duration,
	updateStatus *RulesConfigUpdateStatus,
) error {
	// If no data, create empty ConfigMap
	if len(data) == 0 {
		data = map[string]string{}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      name,
			Labels:    map[string]string{LabelAppName: NameRuleEvaluator},
		},
		Data: data,
	}

	// Create or update with retry
	op := func() error {
		if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
			if err := r.client.Create(ctx, cm); err != nil {
				return fmt.Errorf("create %s configmap: %w", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("update %s configmap: %w", name, err)
		}
		return nil
	}
	updateStatus.ConfigMapResults[name] = retryOperation(op, maxRetries, retryDelay)
	return updateStatus.ConfigMapResults[name]
}

// Helper to compress or not compress rule file content.
func setConfigMapDataRaw(buf *strings.Builder, compression monitoringv1.CompressionType, data string) error {
	if compression == monitoringv1.CompressionGzip {
		return errors.New("gzip compression not implemented in setConfigMapDataRaw")
	}
	buf.WriteString(data)
	return nil
}
