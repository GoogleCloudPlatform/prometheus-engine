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
	"github.com/prometheus/prometheus/pkg/labels"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

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

func (r *collectionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logr.FromContext(ctx).Info("reconciling collection")

	r.statusState.Reset()

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
	if err := r.updateCRDStatus(ctx); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "update crd status")
	}
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

// ensureCollectorDaemonSet generates the collector daemon set and creates or updates it.
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context, spec *monitoringv1alpha1.CollectionSpec) error {
	ds := r.makeCollectorDaemonSet(spec)

	if err := r.client.Update(ctx, ds); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, ds); err != nil {
			return errors.Wrap(err, "create collector DaemonSet")
		}
	} else if err != nil {
		return errors.Wrap(err, "update collector DaemonSet")
	}
	return nil
}

func (r *collectionReconciler) makeCollectorDaemonSet(spec *monitoringv1alpha1.CollectionSpec) *appsv1.DaemonSet {
	// TODO(freinartz): this just fills in the bare minimum to get semantics right.
	// Add more configuration of a full deployment: tolerations, resource request/limit,
	// health checks, priority context, security context, dynamic update strategy params...
	podLabels := map[string]string{
		LabelAppName:      NameCollector,
		KubernetesAppName: CollectorAppName,
	}

	podAnnotations := map[string]string{
		AnnotationMetricName: componentName,
	}

	collectorArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(configOutDir, configFilename)),
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

	// Check for explicitly-set pass-through args.
	if r.opts.ProjectID != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.label.project-id=%s", r.opts.ProjectID))
	}
	if r.opts.Location != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.label.location=%s", r.opts.Location))
	}
	if r.opts.Cluster != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.label.cluster=%s", r.opts.Cluster))
	}
	if r.opts.DisableExport {
		collectorArgs = append(collectorArgs, "--export.disable")
	}
	if r.opts.CloudMonitoringEndpoint != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.endpoint=%s", r.opts.CloudMonitoringEndpoint))
	}
	if spec.Credentials != nil {
		p := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: spec.Credentials}))
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.credentials-file=%s", p))
	}
	// Populate export filtering from OperatorConfig.
	for _, matcher := range spec.Filter.MatchOneOf {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.match=%s", matcher))
	}

	ds := appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: podLabels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      podLabels,
				Annotations: podAnnotations,
			},
			Spec: corev1.PodSpec{
				// We want to run on every node, even with taints present.
				Tolerations: []corev1.Toleration{
					{Effect: "NoExecute", Operator: "Exists"},
					{Effect: "NoSchedule", Operator: "Exists"},
				},
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
							{Name: "prom-metrics", ContainerPort: r.opts.CollectorPort},
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/healthy",
									Port: intstr.FromInt(int(r.opts.CollectorPort)),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/ready",
									Port: intstr.FromInt(int(r.opts.CollectorPort)),
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      configOutVolumeName,
								MountPath: configOutDir,
								ReadOnly:  true,
							}, {
								Name:      secretVolumeName,
								MountPath: secretsDir,
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
							fmt.Sprintf("--config-file=%s", path.Join(configDir, configFilename)),
							fmt.Sprintf("--config-file-output=%s", path.Join(configOutDir, configFilename)),
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
							{Name: "cfg-rel-metrics", ContainerPort: r.opts.CollectorPort + 1},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      configVolumeName,
								MountPath: configDir,
								ReadOnly:  true,
							}, {
								Name:      configOutVolumeName,
								MountPath: configOutDir,
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
						Name: configVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: NameCollector,
								},
							},
						},
					}, {
						Name: configOutVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}, {
						// Mirrored config secrets (config specified as filepaths).
						Name: secretVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: CollectionSecretName,
							},
						},
					},
				},
				ServiceAccountName: NameCollector,
				PriorityClassName:  r.opts.PriorityClass,
			},
		},
	}
	// DNS policy should be set explicitly when using hostNetwork.
	if r.opts.HostNetwork {
		ds.Template.Spec.HostNetwork = true
		ds.Template.Spec.DNSPolicy = "ClusterFirstWithHostNet"
	}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      NameCollector,
		},
		Spec: ds,
	}
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
		podmons monitoringv1alpha1.PodMonitoringList
	)
	if err := r.client.List(ctx, &podmons); err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
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
		cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)

		if err := r.statusState.SetPodMonitoringCondition(&podmon, cond); err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting podmonitoring status state")
		}
	}

	// Sort to ensure reproducible configs.
	sort.Slice(cfg.ScrapeConfigs, func(i, j int) bool {
		return cfg.ScrapeConfigs[i].JobName < cfg.ScrapeConfigs[j].JobName
	})

	return cfg, nil
}
