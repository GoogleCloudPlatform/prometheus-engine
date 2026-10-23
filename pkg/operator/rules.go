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
	"hash/fnv"
	"strings"
	"unicode/utf8"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	nameRulesGenerated    = "rules-generated"
	reasonRuleConfigError = "RuleConfigError"
	reportingController   = "monitoring.googleapis.com/gmp-operator"
	maxEventNoteLength    = 1024
	maxEventObjNameLength = 140
)

func setupRulesControllers(op *Operator) error {
	// The singleton OperatorConfig is the request object we reconcile against.
	objRequest := reconcile.Request{
		Namespace: op.opts.PublicNamespace,
		Name:      NameOperatorConfig,
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
		Namespace: r.opts.OperatorNamespace,
		Name:      "alertmanager",
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
		Namespace: r.opts.OperatorNamespace,
		Name:      "rule-evaluator",
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
func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context, projectID, location, cluster string, configCompression monitoringv1.CompressionType) error {
	logger, _ := logr.FromContext(ctx)

	// Re-generate the configmap that's loaded by the rule-evaluator.
	cm := &corev1.ConfigMap{
		Namespace: r.opts.OperatorNamespace,
		Name:      nameRulesGenerated,
		Labels: map[string]string{
			LabelAppName: NameRuleEvaluator,
		},
		// Ensure there's always at least an empty, uncompressed dummy file as the evaluator
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

	now := metav1.Now()
	conditionSuccess := &monitoringv1.MonitoringCondition{
		Type:   monitoringv1.ConfigurationCreateSuccess,
		Status: corev1.ConditionTrue,
	}
	type statusUpdate struct {
		obj     monitoringv1.MonitoringCRD
		kind    string
		ruleErr error
	}
	var statusUpdates []statusUpdate

	for i := range rulesList.Items {
		rs := &rulesList.Items[i]
		result, err := rs.RuleGroupsConfig(projectID, location, cluster)
		if err != nil {
			if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  reasonRuleConfigError,
				Message: fmt.Sprintf("generating rule config failed: %s", err),
			}) {
				statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "Rules", ruleErr: err})
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("rules__%s__%s.yaml", rs.Namespace, rs.Name)
		if err := setConfigMapData(cm, configCompression, filename, result); err != nil {
			return err
		}

		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "Rules"})
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
			if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  reasonRuleConfigError,
				Message: fmt.Sprintf("generating rule config failed: %s", err),
			}) {
				statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "ClusterRules", ruleErr: err})
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("clusterrules__%s.yaml", rs.Name)
		if err := setConfigMapData(cm, configCompression, filename, result); err != nil {
			return err
		}

		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "ClusterRules"})
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
			if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  reasonRuleConfigError,
				Message: fmt.Sprintf("generating rule config failed: %s", err),
			}) {
				statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "GlobalRules", ruleErr: err})
			}
			logger.Error(err, "convert rules", "err", err, "namespace", rs.Namespace, "name", rs.Name)
			continue
		}
		filename := fmt.Sprintf("globalrules__%s.yaml", rs.Name)
		if err := setConfigMapData(cm, configCompression, filename, result); err != nil {
			return err
		}

		if rs.Status.SetMonitoringCondition(rs.GetGeneration(), now, conditionSuccess) {
			statusUpdates = append(statusUpdates, statusUpdate{obj: rs, kind: "GlobalRules"})
		}
	}

	// Create or update generated rule ConfigMap.
	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return fmt.Errorf("create generated rules: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update generated rules: %w", err)
	}

	var errs []error
	for _, u := range statusUpdates {
		if err := patchMonitoringStatus(ctx, r.client, u.obj, u.obj.GetMonitoringStatus()); err != nil {
			errs = append(errs, err)
			continue
		}
		if u.ruleErr != nil {
			r.recordInvalidRuleEvents(ctx, u.obj, u.kind, now, u.ruleErr)
		}
	}

	return errors.Join(errs...)
}

// recordInvalidRuleEvents emits Kubernetes Warning Events when a Rules, ClusterRules, or GlobalRules
// resource fails operator-side validation. An event is always emitted in the operator namespace with
// regarding set to Deployment/gmp-operator so that it surfaces in the Google Cloud Console workload
// events tab. For namespaced Rules resources, a second event is emitted in the resource's namespace
// with regarding set directly to the Rules object.
func (r *rulesReconciler) recordInvalidRuleEvents(ctx context.Context, obj monitoringv1.MonitoringCRD, kind string, now metav1.Time, ruleErr error) {
	logger, _ := logr.FromContext(ctx)

	opNamespace := r.opts.OperatorNamespace
	if opNamespace == "" {
		opNamespace = DefaultOperatorNamespace
	}

	note := truncateEventNote(ruleErr.Error())

	eventTime := metav1.NewMicroTime(now.Time)
	objRef := corev1.ObjectReference{
		APIVersion: monitoringv1.SchemeGroupVersion.String(),
		Kind:       kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}

	objName := truncateEventObjName(obj.GetName())
	workloadEventName := fmt.Sprintf("%s.%s.%s.%x", NameOperator, strings.ToLower(kind), objName, now.UnixNano())
	if obj.GetNamespace() != "" {
		workloadEventName = fmt.Sprintf("%s.%s.%s.%s.%x", NameOperator, strings.ToLower(kind), obj.GetNamespace(), objName, now.UnixNano())
	}

	workloadEvent := &eventsv1.Event{
		Name:                workloadEventName,
		Namespace:           opNamespace,
		EventTime:           eventTime,
		ReportingController: reportingController,
		ReportingInstance:   NameOperator,
		Action:              "Ignored",
		Reason:              reasonRuleConfigError,
		Note:                note,
		Type:                corev1.EventTypeWarning,
		Regarding: corev1.ObjectReference{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
			Namespace:  opNamespace,
			Name:       NameOperator,
		},
		Related: &objRef,
	}
	if err := r.client.Create(ctx, workloadEvent); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "create operator workload event for invalid rules", "kind", kind, "namespace", obj.GetNamespace(), "name", obj.GetName())
	}

	if kind == "Rules" {
		resourceEvent := &eventsv1.Event{
			Name:                fmt.Sprintf("rules.%s.%x", objName, now.UnixNano()),
			Namespace:           obj.GetNamespace(),
			EventTime:           eventTime,
			ReportingController: reportingController,
			ReportingInstance:   NameOperator,
			Action:              "Ignored",
			Reason:              reasonRuleConfigError,
			Note:                note,
			Type:                corev1.EventTypeWarning,
			Regarding:           objRef,
		}
		if err := r.client.Create(ctx, resourceEvent); err != nil && !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "create resource event for invalid rules", "namespace", obj.GetNamespace(), "name", obj.GetName())
		}
	}
}

// truncateEventNote caps an event note to maxEventNoteLength bytes without splitting
// a multi-byte UTF-8 rune at the cut boundary.
func truncateEventNote(note string) string {
	if len(note) <= maxEventNoteLength {
		return note
	}
	cut := maxEventNoteLength - 3
	for cut > 0 && !utf8.RuneStart(note[cut]) {
		cut--
	}
	return note[:cut] + "..."
}

// truncateEventObjName caps a resource name so generated Kubernetes Event names stay
// within the 253-character DNS subdomain limit and end on an alphanumeric character.
// When truncating, an 8-character FNV-1a hex digest suffix is appended so distinct long
// names that share a common prefix do not collide.
func truncateEventObjName(name string) string {
	if len(name) <= maxEventObjNameLength {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	prefix := strings.TrimRight(name[:maxEventObjNameLength-9], "-.")
	if prefix == "" {
		return fmt.Sprintf("%08x", h.Sum32())
	}
	return fmt.Sprintf("%s-%08x", prefix, h.Sum32())
}
