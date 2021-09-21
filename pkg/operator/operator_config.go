package operator

import (
	"context"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	RuleEvaluatorName = "rule-evaluator"
)

// setupOperatorConfigControllers ensures a rule-evaluator
// deployment as part of managed collection.
func setupOperatorConfigControllers(op *Operator) error {
	// Canonical filter to only capture events for the generated
	// rule evaluator deployment.
	objFilter := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      RuleEvaluatorName,
	}

	err := ctrl.NewControllerManagedBy(op.manager).
		Named("operator-config").
		For(
			&monitoringv1alpha1.OperatorConfig{},
		).
		Owns(
			&corev1.ConfigMap{},
			builder.WithPredicates(objFilter)).
		Owns(
			&appsv1.Deployment{},
			builder.WithPredicates(objFilter)).
		Complete(newOperatorConfigReconciler(op.manager.GetClient(), op.opts))

	if err != nil {
		return errors.Wrap(err, "operator-config controller")
	}
	return nil
}

// operatorConfigReconciler reconciles the OperatorConfig CRD.
type operatorConfigReconciler struct {
	client client.Client
	opts   Options
}

// newOperatorConfigReconciler creates a new operatorConfigReconciler.
func newOperatorConfigReconciler(c client.Client, opts Options) *operatorConfigReconciler {
	return &operatorConfigReconciler{
		client: c,
		opts:   opts,
	}
}

// Reconcile ensures the OperatorConfig resource is reconciled.
func (r *operatorConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := logr.FromContext(ctx).WithValues("operatorconfig", req.NamespacedName)
	logger.Info("reconciling operatorconfig")

	var config = &monitoringv1alpha1.OperatorConfig{}
	if err := r.client.Get(ctx, req.NamespacedName, config); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get operatorconfig")
	}
	if err := r.ensureRuleEvaluatorConfig(ctx, config); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator config")
	}
	if err := r.ensureRuleEvaluatorDeployment(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator deploy")
	}

	return reconcile.Result{}, nil
}

// ensureRuleEvaluatorConfig reconciles the ConfigMap for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorConfig(ctx context.Context, config *monitoringv1alpha1.OperatorConfig) error {
	// TODO(pintohutch): fill out
	return nil
}

// ensureRuleEvaluatorDeployment reconciles the Deployment for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorDeployment(ctx context.Context) error {
	// TODO(pintohutch): fill out
	return nil
}
