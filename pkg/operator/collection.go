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
	"path"
	"sort"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	promconfig "github.com/prometheus/prometheus/config"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	if err := r.ensureCollectorDaemonSet(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure collector daemon set")
	}
	return reconcile.Result{}, nil
}

// Various constants generating resources.
const (
	// CollectorName is the base name of the collector used across various resources. Must match with
	// the static resources installed during the operator's base setup.
	CollectorName = "collector"

	collectorConfigVolumeName    = "config"
	collectorConfigDir           = "/prometheus/config"
	collectorConfigOutVolumeName = "config-out"
	collectorConfigOutDir        = "/prometheus/config_out"
	collectorConfigFilename      = "config.yaml"

	// The well-known app name label.
	LabelAppName = "app.kubernetes.io/name"
)

// ensureCollectorDaemonSet generates the collector daemon set and creates or updates it.
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context) error {
	ds := r.makeCollectorDaemonSet()

	if err := r.client.Update(ctx, ds); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, ds); err != nil {
			return errors.Wrap(err, "create collector DaemonSet")
		}
	} else if err != nil {
		return errors.Wrap(err, "update collector DaemonSet")
	}
	return nil
}

func (r *collectionReconciler) makeCollectorDaemonSet() *appsv1.DaemonSet {
	// TODO(freinartz): this just fills in the bare minimum to get semantics right.
	// Add more configuration of a full deployment: tolerations, resource request/limit,
	// health checks, priority context, security context, dynamic update strategy params...
	podLabels := map[string]string{
		LabelAppName: CollectorName,
	}

	collectorArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(collectorConfigOutDir, collectorConfigFilename)),
		"--storage.tsdb.path=/prometheus/data",
		"--storage.tsdb.no-lockfile",
		// Keep 30 minutes of data. As we are backed by an emptyDir volume, this will count towards
		// the containers memory usage. We could lower it further if this becomes problematic, but
		// it the window for local data is quite convenient for debugging.
		"--storage.tsdb.retention.time=30m",
		"--storage.tsdb.wal-compression",
		// Effectively disable compaction and make blocks short enough so that our retention window
		// can be kept in practice.
		"--storage.tsdb.min-block-duration=10m",
		"--storage.tsdb.max-block-duration=10m",
		fmt.Sprintf("--web.listen-address=:%d", r.opts.CollectorPort),
		"--web.enable-lifecycle",
		"--web.route-prefix=/",
	}
	if r.opts.CloudMonitoringEndpoint != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.endpoint=%s", r.opts.CloudMonitoringEndpoint))
	}

	spec := appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: podLabels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: podLabels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "prometheus",
						Image: r.opts.ImageCollector,
						// Set an aggressive GC threshold (default is 100%). Since the collector has a lot of
						// long-lived allocations, this still doesn't result in a high GC rate (compared to stateless
						// RPC applications) and gives us a more balanced ratio of memory and CPU usage.
						Env: []corev1.EnvVar{
							{Name: "GOGC", Value: "25"},
						},
						Args: collectorArgs,
						Ports: []corev1.ContainerPort{
							{Name: "prometheus-http", ContainerPort: r.opts.CollectorPort},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      collectorConfigOutVolumeName,
								MountPath: collectorConfigOutDir,
								ReadOnly:  true,
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    *resource.NewScaledQuantity(100, resource.Milli),
								corev1.ResourceMemory: *resource.NewScaledQuantity(200, resource.Mega),
							},
							// Set no limit on CPU as it's a throttled resource.
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: *resource.NewScaledQuantity(3000, resource.Mega),
							},
						},
					}, {
						Name:  "config-reloader",
						Image: r.opts.ImageConfigReloader,
						Args: []string{
							fmt.Sprintf("--config-file=%s", path.Join(collectorConfigDir, collectorConfigFilename)),
							fmt.Sprintf("--config-file-output=%s", path.Join(collectorConfigOutDir, collectorConfigFilename)),
							fmt.Sprintf("--reload-url=http://localhost:%d/-/reload", r.opts.CollectorPort),
							fmt.Sprintf("--listen-address=:%d", r.opts.CollectorPort+1),
						},
						// Pass node name so the config can filter for targets on the local node,
						Env: []corev1.EnvVar{
							{
								Name: monitoringv1alpha1.EnvVarNodeName,
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							},
						},
						Ports: []corev1.ContainerPort{
							{Name: "reloader-http", ContainerPort: r.opts.CollectorPort + 1},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      collectorConfigVolumeName,
								MountPath: collectorConfigDir,
								ReadOnly:  true,
							}, {
								Name:      collectorConfigOutVolumeName,
								MountPath: collectorConfigOutDir,
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    *resource.NewScaledQuantity(5, resource.Milli),
								corev1.ResourceMemory: *resource.NewScaledQuantity(16, resource.Mega),
							},
							// Set no limit on CPU as it's a throttled resource.
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: *resource.NewScaledQuantity(32, resource.Mega),
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: collectorConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: CollectorName,
								},
							},
						},
					}, {
						Name: collectorConfigOutVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				ServiceAccountName: CollectorName,
				PriorityClassName:  r.opts.PriorityClass,
				// When a cluster has Workload Identity enabled, the default GCP service account
				// of the node is no longer accessible. That is unless the pod runs on the host network,
				// in which case it keeps accessing the GCE metadata agent, rather than the GKE metadata
				// agent.
				// We run the collector in the host network for now to match behavior of other GKE
				// telemetry agents and not require an additional permission setup step for collection.
				// This relies on the default GCP service account to have write permissions for Cloud
				// Monitoring set, which generally is the case.
				HostNetwork: true,
			},
		},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      CollectorName,
		},
		Spec: spec,
	}
	return ds
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
			collectorConfigFilename: string(cfgEncoded),
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
