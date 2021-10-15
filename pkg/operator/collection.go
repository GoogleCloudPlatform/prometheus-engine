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
	"sort"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	promconfig "github.com/prometheus/prometheus/config"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
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

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

func setupCollectionControllers(op *Operator) error {
	// Canonical request for both the config map as well as the daemon set.
	objRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: op.opts.OperatorNamespace,
			Name:      CollectorName,
		},
	}
	// Canonical filter to only capture events for the config or collector object.
	objFilter := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      CollectorName,
	}
	// Predicate that filters for config maps containing hardcoded Prometheus scrape configs.
	staticScrapeConfigSelector, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{"type": "scrape-config"},
	})
	if err != nil {
		return err
	}
	// Reconcile the generated Prometheus configuration that is used by all collectors.
	err = ctrl.NewControllerManagedBy(op.manager).
		Named("collector-config").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		For(
			&corev1.ConfigMap{},
			builder.WithPredicates(objFilter),
		).
		// Any update to a PodMonitoring requires regenerating the config.
		Watches(
			&source.Kind{Type: &monitoringv1alpha1.PodMonitoring{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Specifically labeled ConfigMaps in the operator namespace allow to inject
		// hard-coded scrape configurations.
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			enqueueConst(objRequest),
			builder.WithPredicates(staticScrapeConfigSelector),
		).
		// Trigger for changes to the collector DaemonSet as well as we handle it as part
		// of the config controller for now.  This does not guarantee initial collector creation in
		// the absence of PodMonitorings or ConfigMaps.
		// TODO(freinartz): This is fine in principle but ultimately the collector should be
		// created along with other resources that are fixed for a given operator configuration.
		// An operator config CRD should act as the general trigger resource to deploy these
		// static resources.
		Watches(
			&source.Kind{Type: &appsv1.DaemonSet{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(newCollectionReconciler(op.manager.GetClient(), op.opts))
	if err != nil {
		return errors.Wrap(err, "create collector config controller")
	}
	return nil
}

type collectionReconciler struct {
	client client.Client
	opts   Options
	// Internal bookkeeping for sending status updates to processed CRDs.
	statusState *CRDStatusState
}

func newCollectionReconciler(c client.Client, opts Options) *collectionReconciler {
	return &collectionReconciler{
		client:      c,
		opts:        opts,
		statusState: NewCRDStatusState(metav1.Now),
	}
}

func (r *collectionReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	r.statusState.Reset()

	if err := r.ensureCollectorConfig(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure collector config")
	}
	if err := r.updateCRDStatus(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "update crd status")
	}
	return reconcile.Result{}, nil
}

// updateCRDStatus iterates through parsed CRDs and updates their statuses.
// If an error is encountered from performing an update, the function returns
// the error immediately and does not attempt updates on subsequent CRDs.
func (r *collectionReconciler) updateCRDStatus(ctx context.Context) error {
	for _, pm := range r.statusState.PodMonitorings() {
		if err := r.client.Status().Update(ctx, &pm); err != nil {
			return err
		}
	}
	return nil
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (r *collectionReconciler) ensureCollectorConfig(ctx context.Context) error {
	cfg, err := r.makeCollectorConfig(ctx)
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
			Name:      CollectorName,
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

func (r *collectionReconciler) makeCollectorConfig(ctx context.Context) (*promconfig.Config, error) {
	logger := logr.FromContext(ctx)

	var scrapeCfgs []*promconfig.ScrapeConfig
	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podmons    monitoringv1alpha1.PodMonitoringList
		scrapecfgs corev1.ConfigMapList
	)
	if err := r.client.List(ctx, &podmons); err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
	}
	if err := r.client.List(ctx, &scrapecfgs, client.MatchingLabels{"type": "scrape-config"}); err != nil {
		return nil, errors.Wrap(err, "failed to list scrape ConfigMaps")
	}

	// Mark status updates in batch with single timestamp.
	for _, pm := range podmons.Items {
		// Reassign so we can safely get a pointer.
		podmon := pm

		cond := &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := podmon.ScrapeConfigs()
		if err != nil {
			logger.Error(err, "generating scrape config failed for PodMonitoring endpoint",
				"namespace", podmon.Namespace, "name", podmon.Name)
			continue
		}
		scrapeCfgs = append(scrapeCfgs, cfgs...)

		if err := r.statusState.SetPodMonitoringCondition(&podmon, cond); err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting podmonitoring status state")
		}
	}

	// Load additional, hard-coded scrape configs from configmaps in the oeprator's namespace.
	for _, cm := range scrapecfgs.Items {
		const key = "config.yaml"

		var promcfg promconfig.Config
		if err := yaml.Unmarshal([]byte(cm.Data[key]), &promcfg); err != nil {
			logger.Error(err, "cannot parse scrape config, skipping ...",
				"namespace", cm.Namespace, "name", cm.Name)
			continue
		}
		for _, sc := range promcfg.ScrapeConfigs {
			// Make scrape config name unique and traceable.
			sc.JobName = fmt.Sprintf("ConfigMap/%s/%s/%s", r.opts.OperatorNamespace, cm.Name, sc.JobName)
			scrapeCfgs = append(scrapeCfgs, sc)
		}
	}

	// Sort to ensure reproducible configs.
	sort.Slice(scrapeCfgs, func(i, j int) bool {
		return scrapeCfgs[i].JobName < scrapeCfgs[j].JobName
	})
	return &promconfig.Config{
		ScrapeConfigs: scrapeCfgs,
	}, nil
}
