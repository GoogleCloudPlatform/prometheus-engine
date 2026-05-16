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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	nameRulesGenerated       = "rules-generated"
	nameRulesGeneratedPrefix = "rules-generated-"
	maxShardDataBytes        = 800 * 1024 // headroom below 1MB etcd limit for metadata overhead
	labelRulesShardType      = "monitoring.googleapis.com/rules-shard"
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
	// Rule-evaluator rules shard ConfigMap filter.
	objFilterRuleShards := namespacedLabelPredicate{
		namespace: op.opts.OperatorNamespace,
		labels:    map[string]string{labelRulesShardType: "true"},
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
			builder.WithPredicates(objFilterRuleShards),
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

type ruleEntry struct {
	filename string
	data     string
}

func configMapDataSize(cm *corev1.ConfigMap) int {
	var n int
	for k, v := range cm.Data {
		n += len(k) + len(v)
	}
	for k, v := range cm.BinaryData {
		n += len(k) + len(v)
	}
	return n
}

func deleteConfigMapKey(cm *corev1.ConfigMap, compression monitoringv1.CompressionType, key string) {
	switch compression {
	case monitoringv1.CompressionGzip:
		delete(cm.BinaryData, key)
	default:
		delete(cm.Data, key)
	}
}

type namespacedLabelPredicate struct {
	namespace string
	labels    map[string]string
}

func (p namespacedLabelPredicate) Create(e event.CreateEvent) bool {
	return p.matches(e.Object)
}
func (p namespacedLabelPredicate) Update(e event.UpdateEvent) bool {
	return p.matches(e.ObjectNew)
}
func (p namespacedLabelPredicate) Delete(e event.DeleteEvent) bool {
	return p.matches(e.Object)
}
func (p namespacedLabelPredicate) Generic(e event.GenericEvent) bool {
	return p.matches(e.Object)
}

func (p namespacedLabelPredicate) matches(obj client.Object) bool {
	if obj.GetNamespace() != p.namespace {
		return false
	}
	objLabels := obj.GetLabels()
	for k, v := range p.labels {
		if objLabels[k] != v {
			return false
		}
	}
	return true
}

func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context, projectID, location, cluster string, configCompression monitoringv1.CompressionType) error {
	logger, _ := logr.FromContext(ctx)

	var rulesList monitoringv1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return fmt.Errorf("list rules: %w", err)
	}

	now := metav1.Now()
	conditionSuccess := &monitoringv1.MonitoringCondition{
		Type:   monitoringv1.ConfigurationCreateSuccess,
		Status: corev1.ConditionTrue,
	}
	var statusUpdates []monitoringv1.MonitoringCRD
	var entries []ruleEntry

	for i := range rulesList.Items {
		rs := &rulesList.Items[i]
		result, err := rs.RuleGroupsConfig(projectID, location, cluster)
		if err != nil {
			msg := "generating rule config failed"
			if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Message: msg,
				Reason:  err.Error(),
			}) {
				statusUpdates = append(statusUpdates, rs)
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("rules__%s__%s.yaml", rs.Namespace, rs.Name)
		entries = append(entries, ruleEntry{filename: filename, data: result})
		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, rs)
		}
	}

	var clusterRulesList monitoringv1.ClusterRulesList
	if err := r.client.List(ctx, &clusterRulesList); err != nil {
		return fmt.Errorf("list cluster rules: %w", err)
	}
	for i := range clusterRulesList.Items {
		rs := &clusterRulesList.Items[i]
		result, err := rs.RuleGroupsConfig(projectID, location, cluster)
		if err != nil {
			msg := "generating rule config failed"
			if rs.Status.SetMonitoringCondition(rs.Generation, now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Message: msg,
				Reason:  err.Error(),
			}) {
				statusUpdates = append(statusUpdates, rs)
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("clusterrules__%s.yaml", rs.Name)
		entries = append(entries, ruleEntry{filename: filename, data: result})
		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, rs)
		}
	}

	var globalRulesList monitoringv1.GlobalRulesList
	if err := r.client.List(ctx, &globalRulesList); err != nil {
		return fmt.Errorf("list global rules: %w", err)
	}
	for i := range globalRulesList.Items {
		rs := &globalRulesList.Items[i]
		result, err := rs.RuleGroupsConfig()
		if err != nil {
			msg := "generating rule config failed"
			if rs.Status.SetMonitoringCondition(rs.Generation, now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Message: msg,
				Reason:  err.Error(),
			}) {
				statusUpdates = append(statusUpdates, rs)
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("globalrules__%s.yaml", rs.Name)
		entries = append(entries, ruleEntry{filename: filename, data: result})
		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, rs)
		}
	}

	numShards, err := r.upsertShards(ctx, entries, configCompression)
	if err != nil {
		return err
	}
	if err := r.deleteStaleShards(ctx, logger, numShards); err != nil {
		return err
	}

	var errs []error
	for _, obj := range statusUpdates {
		if err := patchMonitoringStatus(ctx, r.client, obj, obj.GetMonitoringStatus()); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *rulesReconciler) newShardConfigMap(idx int) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      fmt.Sprintf("%s%d", nameRulesGeneratedPrefix, idx),
			Labels: map[string]string{
				LabelAppName:        NameRuleEvaluator,
				labelRulesShardType: "true",
			},
		},
	}
	if idx == 0 {
		cm.Data = map[string]string{"empty.yaml": ""}
	}
	return cm
}

func (r *rulesReconciler) createOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return fmt.Errorf("create shard %s: %w", cm.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("update shard %s: %w", cm.Name, err)
	}
	return nil
}

func (r *rulesReconciler) upsertShards(ctx context.Context, entries []ruleEntry, compression monitoringv1.CompressionType) (int, error) {
	shardIdx := 0
	cm := r.newShardConfigMap(shardIdx)
	entriesInShard := 0

	for _, e := range entries {
		if err := setConfigMapData(cm, compression, e.filename, e.data); err != nil {
			return 0, err
		}
		entriesInShard++

		if configMapDataSize(cm) > maxShardDataBytes && entriesInShard > 1 {
			deleteConfigMapKey(cm, compression, e.filename)
			if err := r.createOrUpdateConfigMap(ctx, cm); err != nil {
				return 0, err
			}
			shardIdx++
			cm = r.newShardConfigMap(shardIdx)
			if err := setConfigMapData(cm, compression, e.filename, e.data); err != nil {
				return 0, err
			}
			entriesInShard = 1
		}
	}

	if err := r.createOrUpdateConfigMap(ctx, cm); err != nil {
		return 0, err
	}
	return shardIdx + 1, nil
}

func (r *rulesReconciler) deleteStaleShards(ctx context.Context, logger logr.Logger, numShards int) error {
	active := make(map[string]bool, numShards)
	for i := range numShards {
		active[fmt.Sprintf("%s%d", nameRulesGeneratedPrefix, i)] = true
	}

	var cmList corev1.ConfigMapList
	if err := r.client.List(ctx, &cmList,
		client.InNamespace(r.opts.OperatorNamespace),
		client.MatchingLabels{labelRulesShardType: "true"},
	); err != nil {
		return fmt.Errorf("list rule configmaps: %w", err)
	}

	for i := range cmList.Items {
		cm := &cmList.Items[i]
		if !active[cm.Name] {
			if err := r.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "delete stale shard", "name", cm.Name)
			}
		}
	}

	// Clean up legacy single ConfigMap from before sharding was introduced.
	var legacy corev1.ConfigMap
	legacy.Namespace = r.opts.OperatorNamespace
	legacy.Name = nameRulesGenerated
	if err := r.client.Delete(ctx, &legacy); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "delete legacy rules-generated")
	}
	return nil
}
