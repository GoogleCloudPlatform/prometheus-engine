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
	"sort"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
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

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

func ptr(b bool) *bool {
	return &b
}

func setupCollectionControllers(op *Operator) error {
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
	// Collector ConfigMap and Daemonset filter.
	objFilterCollector := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      NameCollector,
	}

	// Reconcile the generated Prometheus configuration that is used by all collectors.
	err := ctrl.NewControllerManagedBy(op.manager).
		Named("collector-config").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		// OperatorConfig is our root resource that ensures we reconcile
		// at least once initially.
		For(
			&monitoringv1alpha1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		// Any update to a PodMonitoring requires regenerating the config.
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.PodMonitoring{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Any update to a ClusterPodMonitoring requires regenerating the config.
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.ClusterPodMonitoring{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// The configuration we generate for the collectors.
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterCollector),
		).
		// Detect and undo changes to the daemon set.
		Watches(
			&source.Kind{Type: &appsv1.DaemonSet{}},
			enqueueConst(objRequest),
			builder.WithPredicates(
				objFilterCollector,
				predicate.GenerationChangedPredicate{},
			)).
		Complete(newCollectionReconciler(op.manager.GetClient(), op.opts))
	if err != nil {
		return errors.Wrap(err, "create collector config controller")
	}
	return nil
}

type collectionReconciler struct {
	client        client.Client
	opts          Options
	statusUpdates []client.Object
}

func newCollectionReconciler(c client.Client, opts Options) *collectionReconciler {
	return &collectionReconciler{
		client: c,
		opts:   opts,
	}
}

func (r *collectionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := logr.FromContext(ctx)
	logger.Info("reconciling collection")

	var config monitoringv1alpha1.OperatorConfig
	// Fetch OperatorConfig if it exists.
	if err := r.client.Get(ctx, req.NamespacedName, &config); apierrors.IsNotFound(err) {
		logr.FromContext(ctx).Info("no operatorconfig created yet")
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "get operatorconfig for incoming: %q", req.String())
	}

	if err := r.ensureCollectorSecrets(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure collector secrets")
	}
	// Deploy Prometheus collector as a node agent.
	if err := r.ensureCollectorDaemonSet(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure collector daemon set")
	}

	if err := r.ensureCollectorConfig(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure collector config")
	}

	// Reconcile any status updates.
	for _, obj := range r.statusUpdates {
		if err := r.client.Status().Update(ctx, obj); err != nil {
			logger.Error(err, "update status", "obj", obj)
		}
	}
	// Reset status updates for next reconcile loop.
	r.statusUpdates = r.statusUpdates[:0]

	return reconcile.Result{}, nil
}

func (r *collectionReconciler) ensureCollectorSecrets(ctx context.Context, spec *monitoringv1alpha1.CollectionSpec) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectionSecretName,
			Namespace: r.opts.OperatorNamespace,
			Labels: map[string]string{
				LabelAppName: NameCollector,
			},
			Annotations: map[string]string{
				AnnotationMetricName: componentName,
			},
		},
		Data: make(map[string][]byte),
	}
	if spec.Credentials != nil {
		p := pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: spec.Credentials})
		b, err := getSecretKeyBytes(ctx, r.client, r.opts.PublicNamespace, spec.Credentials)
		if err != nil {
			return err
		}
		secret.Data[p] = b
	}

	if err := r.client.Update(ctx, secret); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, secret); err != nil {
			return errors.Wrap(err, "create collector secrets")
		}
	} else if err != nil {
		return errors.Wrap(err, "update rule-evaluator secrets")
	}
	return nil
}

// ensureCollectorDaemonSet populates the collector DaemonSet with operator-provided values.
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context, spec *monitoringv1alpha1.CollectionSpec) error {
	var ds appsv1.DaemonSet
	err := r.client.Get(ctx, client.ObjectKey{Namespace: r.opts.OperatorNamespace, Name: NameCollector}, &ds)
	// Some users deliberately not want to run the collectors. Only emit a warning but don't cause
	// retries as this logic gets re-triggered anyway if the DaemonSet is created later.
	if apierrors.IsNotFound(err) {
		logr.FromContext(ctx).Error(err, "collector DaemonSet does not exist")
		return nil
	}
	if err != nil {
		return err
	}
	// Update the envvars that are expected by externally-provided the DaemonSet.
	vars := map[string]string{
		"PROJECT_ID": r.opts.ProjectID,
		"LOCATION":   r.opts.Location,
		"CLUSTER":    r.opts.Cluster,
	}
	for i, c := range ds.Spec.Template.Spec.Containers {
		if c.Name != "prometheus" {
			continue
		}
		var repl []corev1.EnvVar

		// Retain existing vars not controlled by the operator.
		for _, ev := range c.Env {
			if _, ok := vars[ev.Name]; !ok {
				repl = append(repl, ev)
			}
		}
		// Append vars controlled by the operator.
		for k, v := range vars {
			repl = append(repl, corev1.EnvVar{Name: k, Value: v})
		}

		ds.Spec.Template.Spec.Containers[i].Env = repl
	}
	return r.client.Update(ctx, &ds)
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (r *collectionReconciler) ensureCollectorConfig(ctx context.Context, spec *monitoringv1alpha1.CollectionSpec) error {
	cfg, err := r.makeCollectorConfig(ctx, spec)
	if err != nil {
		return errors.Wrap(err, "generate Prometheus config")
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, "marshal Prometheus config")
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      NameCollector,
		},
		Data: map[string]string{
			configFilename: string(cfgEncoded),
		},
	}

	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return errors.Wrap(err, "create Prometheus config")
		}
	} else if err != nil {
		return errors.Wrap(err, "update Prometheus config")
	}
	return nil
}

func (r *collectionReconciler) makeCollectorConfig(ctx context.Context, spec *monitoringv1alpha1.CollectionSpec) (*promconfig.Config, error) {
	logger := logr.FromContext(ctx)

	cfg := &promconfig.Config{
		GlobalConfig: promconfig.GlobalConfig{
			ExternalLabels: labels.FromMap(spec.ExternalLabels),
		},
	}

	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podMons        monitoringv1alpha1.PodMonitoringList
		clusterPodMons monitoringv1alpha1.ClusterPodMonitoringList
		cond           *monitoringv1alpha1.MonitoringCondition
	)
	if err := r.client.List(ctx, &podMons); err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
	}

	// Mark status updates in batch with single timestamp.
	for _, pm := range podMons.Items {
		// Reassign so we can safely get a pointer.
		pmon := pm

		cond = &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := pmon.ScrapeConfigs()
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			cond = &monitoringv1alpha1.MonitoringCondition{
				Type:    monitoringv1alpha1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", pmon.Namespace, "name", pmon.Name)
			continue
		}
		cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)

		change, err := pmon.Status.SetPodMonitoringCondition(pmon.GetGeneration(), metav1.Now(), cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting podmonitoring status state")
		}

		if change {
			r.statusUpdates = append(r.statusUpdates, &pmon)
		}
	}

	if err := r.client.List(ctx, &clusterPodMons); err != nil {
		return nil, errors.Wrap(err, "failed to list ClusterPodMonitorings")
	}

	// Mark status updates in batch with single timestamp.
	for _, cm := range clusterPodMons.Items {
		// Reassign so we can safely get a pointer.
		cmon := cm

		cond = &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cmon.ScrapeConfigs()
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			cond = &monitoringv1alpha1.MonitoringCondition{
				Type:    monitoringv1alpha1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", cmon.Namespace, "name", cmon.Name)
			continue
		}
		cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)

		change, err := cmon.Status.SetPodMonitoringCondition(cmon.GetGeneration(), metav1.Now(), cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting podmonitoring status state")
		}

		if change {
			r.statusUpdates = append(r.statusUpdates, &cmon)
		}
	}

	// Sort to ensure reproducible configs.
	sort.Slice(cfg.ScrapeConfigs, func(i, j int) bool {
		return cfg.ScrapeConfigs[i].JobName < cfg.ScrapeConfigs[j].JobName
	})

	return cfg, nil
}

type podMonitoringDefaulter struct{}

func (d *podMonitoringDefaulter) Default(ctx context.Context, o runtime.Object) error {
	pm := o.(*monitoringv1alpha1.PodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}

type clusterPodMonitoringDefaulter struct{}

func (d *clusterPodMonitoringDefaulter) Default(ctx context.Context, o runtime.Object) error {
	pm := o.(*monitoringv1alpha1.ClusterPodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"namespace", "pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}
