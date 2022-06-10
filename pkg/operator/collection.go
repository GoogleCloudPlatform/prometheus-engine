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
	"bytes"
	"context"
	"fmt"
	"path"
	"sort"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
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
			&monitoringv1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		// Any update to a PodMonitoring requires regenerating the config.
		Watches(
			&source.Kind{Type: &monitoringv1.PodMonitoring{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Any update to a ClusterPodMonitoring requires regenerating the config.
		Watches(
			&source.Kind{Type: &monitoringv1.ClusterPodMonitoring{}},
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
	logger, _ := logr.FromContext(ctx)
	logger.Info("reconciling collection")

	var config monitoringv1.OperatorConfig
	// Fetch OperatorConfig if it exists.
	if err := r.client.Get(ctx, req.NamespacedName, &config); apierrors.IsNotFound(err) {
		logger.Info("no operatorconfig created yet")
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

func (r *collectionReconciler) ensureCollectorSecrets(ctx context.Context, spec *monitoringv1.CollectionSpec) error {
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
		p := pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials})
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
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context, spec *monitoringv1.CollectionSpec) error {
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

func (r *collectionReconciler) makeCollectorDaemonSet(spec *monitoringv1.CollectionSpec) *appsv1.DaemonSet {
	// TODO(freinartz): this just fills in the bare minimum to get semantics right.
	// Add more configuration of a full deployment: tolerations, resource request/limit,
	// health checks, priority context, security context, dynamic update strategy params...

	// DO NOT MODIFY - label selectors are immutable by the Kubernetes API.
	// see: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#label-selector-updates.
	podLabelSelector := map[string]string{
		LabelAppName: NameCollector,
	}
	podLabels := map[string]string{
		LabelAppName:      NameCollector,
		KubernetesAppName: CollectorAppName,
	}

	podAnnotations := map[string]string{
		AnnotationMetricName: componentName,
		// Allow cluster autoscaler to evict collector Pods even though the Pods
		// have an emptyDir volume mounted. This is okay since the node where the
		// Pod runs will be scaled down and therefore does not need metrics reporting.
		ClusterAutoscalerSafeEvictionLabel: "true",
	}

	collectorArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(configOutDir, configFilename)),
		fmt.Sprintf("--storage.tsdb.path=%s", storageDir),
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
		p := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials}))
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.credentials-file=%s", p))
	}
	// Populate export filtering from OperatorConfig.
	for _, matcher := range spec.Filter.MatchOneOf {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.match=%s", matcher))
	}
	collectorArgs = append(collectorArgs, fmt.Sprintf("--export.user-agent=prometheus/%s (mode:%s)", CollectorVersion, r.opts.Mode))

	ds := appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: podLabelSelector,
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
				// The managed collection binaries are only being built for
				// amd64 arch on Linux.
				NodeSelector: map[string]string{
					corev1.LabelOSStable:   "linux",
					corev1.LabelArchStable: "amd64",
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
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/healthy",
									Port: intstr.FromInt(int(r.opts.CollectorPort)),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/ready",
									Port: intstr.FromInt(int(r.opts.CollectorPort)),
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      storageVolumeName,
								MountPath: storageDir,
							}, {
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
								corev1.ResourceCPU:    *resource.NewScaledQuantity(r.opts.CollectorCPUResource, resource.Milli),
								corev1.ResourceMemory: *resource.NewScaledQuantity(r.opts.CollectorMemoryResource, resource.Mega),
							},
							Limits: collectorResourceLimits(r.opts),
						},
						SecurityContext: minimalSecurityContext(),
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
								Name: monitoringv1.EnvVarNodeName,
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
							// Set sane default limit on CPU for config-reloader.
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    *resource.NewScaledQuantity(100, resource.Milli),
								corev1.ResourceMemory: *resource.NewScaledQuantity(32, resource.Mega),
							},
						},
						SecurityContext: minimalSecurityContext(),
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: storageVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}, {
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
				ServiceAccountName:           NameCollector,
				AutomountServiceAccountToken: ptr(true),
				PriorityClassName:            r.opts.PriorityClass,
				SecurityContext:              podSpecSecurityContext(),
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

func collectorResourceLimits(opts Options) corev1.ResourceList {
	limits := corev1.ResourceList{
		corev1.ResourceMemory: *resource.NewScaledQuantity(opts.CollectorMemoryLimit, resource.Mega),
	}
	if cpuLimit := opts.CollectorCPULimit; cpuLimit >= 0 {
		limits = corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewScaledQuantity(cpuLimit, resource.Milli),
			corev1.ResourceMemory: *resource.NewScaledQuantity(opts.CollectorMemoryLimit, resource.Mega),
		}
	}
	return limits
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (r *collectionReconciler) ensureCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec) error {
	cfg, err := r.makeCollectorConfig(ctx, spec)
	if err != nil {
		return errors.Wrap(err, "generate Prometheus config")
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, "marshal Prometheus config")
	}
	// We depend on a newer Prometheus config version, which generates some defaulted fields
	// not recognized by the collector/evaluator version assumed by the operator.
	// Thus we strip them from the generated YAML manually.
	// TODO(freinartz): remove this once the assumed Prometheus version is updated to Prometheus v2.35.
	var lines [][]byte
	for _, l := range bytes.SplitAfter(cfgEncoded, []byte("\n")) {
		if !bytes.Contains(l, []byte("enable_http2:")) && !bytes.Contains(l, []byte("own_namespace:")) && !bytes.Contains(l, []byte(`kubeconfig_file: ""`)) {
			lines = append(lines, l)
		}
	}
	cfgEncoded = bytes.Join(lines, nil)

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

func (r *collectionReconciler) makeCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec) (*promconfig.Config, error) {
	logger, _ := logr.FromContext(ctx)

	cfg := &promconfig.Config{
		GlobalConfig: promconfig.GlobalConfig{
			ExternalLabels: labels.FromMap(spec.ExternalLabels),
		},
	}

	var err error
	cfg.ScrapeConfigs, err = makeKubeletScrapeConfigs(spec.KubeletScraping)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubelet scrape config")
	}

	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podMons        monitoringv1.PodMonitoringList
		clusterPodMons monitoringv1.ClusterPodMonitoringList
		cond           *monitoringv1.MonitoringCondition
	)
	if err := r.client.List(ctx, &podMons); err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
	}

	// Mark status updates in batch with single timestamp.
	for _, pm := range podMons.Items {
		// Reassign so we can safely get a pointer.
		pmon := pm

		cond = &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := pmon.ScrapeConfigs(r.opts.ProjectID, r.opts.Location, r.opts.Cluster)
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
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

		cond = &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cmon.ScrapeConfigs(r.opts.ProjectID, r.opts.Location, r.opts.Cluster)
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
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
	pm := o.(*monitoringv1.PodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}

type clusterPodMonitoringDefaulter struct{}

func (d *clusterPodMonitoringDefaulter) Default(ctx context.Context, o runtime.Object) error {
	pm := o.(*monitoringv1.ClusterPodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"namespace", "pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}

func makeKubeletScrapeConfigs(cfg *monitoringv1.KubeletScraping) ([]*promconfig.ScrapeConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	discoveryCfgs := discovery.Configs{
		&discoverykube.SDConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			Role:             discoverykube.RoleNode,
			// Drop all potential targets not the same node as the collector. The $(NODE_NAME) variable
			// is interpolated by the config reloader sidecar before the config reaches the Prometheus collector.
			// Doing it through selectors rather than relabeling should substantially reduce the client and
			// server side load.
			Selectors: []discoverykube.SelectorConfig{
				{
					Role:  discoverykube.RoleNode,
					Field: fmt.Sprintf("metadata.name=$(%s)", monitoringv1.EnvVarNodeName),
				},
			},
		},
	}
	clientCfg := config.HTTPClientConfig{
		Authorization: &config.Authorization{
			CredentialsFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		},
		TLSConfig: config.TLSConfig{
			CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		},
	}
	interval, err := prommodel.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, errors.Wrap(err, "invalid scrape interval")
	}
	relabelCfgs := []*relabel.Config{
		{
			Action:      relabel.Replace,
			Replacement: "kubelet",
			TargetLabel: "job",
		},
		{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_node_name"},
			TargetLabel:  "node",
		},
	}
	dropByName := func(pattern string) *relabel.Config {
		return &relabel.Config{
			Action:       relabel.Drop,
			SourceLabels: prommodel.LabelNames{"__name__"},
			Regex:        relabel.MustNewRegexp(pattern),
		}
	}
	// We adopt the metric relabeling behavior of kube-prometheus as it's widely adopted and hence
	// will meet user expectations (e.g. dropping deprecated metrics).
	return []*promconfig.ScrapeConfig{
		{
			JobName:                 "kubelet/metrics",
			ServiceDiscoveryConfigs: discoveryCfgs,
			ScrapeInterval:          interval,
			Scheme:                  "https",
			MetricsPath:             "/metrics",
			HTTPClientConfig:        clientCfg,
			RelabelConfigs: append(relabelCfgs, &relabel.Config{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_node_name"},
				TargetLabel:  "instance",
				Replacement:  `$1:metrics`,
			}),
			MetricRelabelConfigs: []*relabel.Config{
				dropByName(`kubelet_(pod_worker_latency_microseconds|pod_start_latency_microseconds|cgroup_manager_latency_microseconds|pod_worker_start_latency_microseconds|pleg_relist_latency_microseconds|pleg_relist_interval_microseconds|runtime_operations|runtime_operations_latency_microseconds|runtime_operations_errors|eviction_stats_age_microseconds|device_plugin_registration_count|device_plugin_alloc_latency_microseconds|network_plugin_operations_latency_microseconds)`),
				dropByName(`scheduler_(e2e_scheduling_latency_microseconds|scheduling_algorithm_predicate_evaluation|scheduling_algorithm_priority_evaluation|scheduling_algorithm_preemption_evaluation|scheduling_algorithm_latency_microseconds|binding_latency_microseconds|scheduling_latency_seconds)`),
				dropByName(`apiserver_(request_count|request_latencies|request_latencies_summary|dropped_requests|storage_data_key_generation_latencies_microseconds|storage_transformation_failures_total|storage_transformation_latencies_microseconds|proxy_tunnel_sync_latency_secs|longrunning_gauge|registered_watchers)`),
				dropByName(`kubelet_docker_(operations|operations_latency_microseconds|operations_errors|operations_timeout)`),
				dropByName(`reflector_(items_per_list|items_per_watch|list_duration_seconds|lists_total|short_watches_total|watch_duration_seconds|watches_total)`),
				dropByName(`etcd_(helper_cache_hit_count|helper_cache_miss_count|helper_cache_entry_count|object_counts|request_cache_get_latencies_summary|request_cache_add_latencies_summary|request_latencies_summary)`),
				dropByName(`transformation_(transformation_latencies_microseconds|failures_total)`),
				dropByName(`(admission_quota_controller_adds|admission_quota_controller_depth|admission_quota_controller_longest_running_processor_microseconds|admission_quota_controller_queue_latency|admission_quota_controller_unfinished_work_seconds|admission_quota_controller_work_duration|APIServiceOpenAPIAggregationControllerQueue1_adds|APIServiceOpenAPIAggregationControllerQueue1_depth|APIServiceOpenAPIAggregationControllerQueue1_longest_running_processor_microseconds|APIServiceOpenAPIAggregationControllerQueue1_queue_latency|APIServiceOpenAPIAggregationControllerQueue1_retries|APIServiceOpenAPIAggregationControllerQueue1_unfinished_work_seconds|APIServiceOpenAPIAggregationControllerQueue1_work_duration|APIServiceRegistrationController_adds|APIServiceRegistrationController_depth|APIServiceRegistrationController_longest_running_processor_microseconds|APIServiceRegistrationController_queue_latency|APIServiceRegistrationController_retries|APIServiceRegistrationController_unfinished_work_seconds|APIServiceRegistrationController_work_duration|autoregister_adds|autoregister_depth|autoregister_longest_running_processor_microseconds|autoregister_queue_latency|autoregister_retries|autoregister_unfinished_work_seconds|autoregister_work_duration|AvailableConditionController_adds|AvailableConditionController_depth|AvailableConditionController_longest_running_processor_microseconds|AvailableConditionController_queue_latency|AvailableConditionController_retries|AvailableConditionController_unfinished_work_seconds|AvailableConditionController_work_duration|crd_autoregistration_controller_adds|crd_autoregistration_controller_depth|crd_autoregistration_controller_longest_running_processor_microseconds|crd_autoregistration_controller_queue_latency|crd_autoregistration_controller_retries|crd_autoregistration_controller_unfinished_work_seconds|crd_autoregistration_controller_work_duration|crdEstablishing_adds|crdEstablishing_depth|crdEstablishing_longest_running_processor_microseconds|crdEstablishing_queue_latency|crdEstablishing_retries|crdEstablishing_unfinished_work_seconds|crdEstablishing_work_duration|crd_finalizer_adds|crd_finalizer_depth|crd_finalizer_longest_running_processor_microseconds|crd_finalizer_queue_latency|crd_finalizer_retries|crd_finalizer_unfinished_work_seconds|crd_finalizer_work_duration|crd_naming_condition_controller_adds|crd_naming_condition_controller_depth|crd_naming_condition_controller_longest_running_processor_microseconds|crd_naming_condition_controller_queue_latency|crd_naming_condition_controller_retries|crd_naming_condition_controller_unfinished_work_seconds|crd_naming_condition_controller_work_duration|crd_openapi_controller_adds|crd_openapi_controller_depth|crd_openapi_controller_longest_running_processor_microseconds|crd_openapi_controller_queue_latency|crd_openapi_controller_retries|crd_openapi_controller_unfinished_work_seconds|crd_openapi_controller_work_duration|DiscoveryController_adds|DiscoveryController_depth|DiscoveryController_longest_running_processor_microseconds|DiscoveryController_queue_latency|DiscoveryController_retries|DiscoveryController_unfinished_work_seconds|DiscoveryController_work_duration|kubeproxy_sync_proxy_rules_latency_microseconds|non_structural_schema_condition_controller_adds|non_structural_schema_condition_controller_depth|non_structural_schema_condition_controller_longest_running_processor_microseconds|non_structural_schema_condition_controller_queue_latency|non_structural_schema_condition_controller_retries|non_structural_schema_condition_controller_unfinished_work_seconds|non_structural_schema_condition_controller_work_duration|rest_client_request_latency_seconds|storage_operation_errors_total|storage_operation_status_count)`),
			},
		}, {
			JobName:                 "kubelet/cadvisor",
			ServiceDiscoveryConfigs: discoveryCfgs,
			ScrapeInterval:          interval,
			Scheme:                  "https",
			MetricsPath:             "/metrics/cadvisor",
			HTTPClientConfig:        clientCfg,
			RelabelConfigs: append(relabelCfgs, &relabel.Config{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_node_name"},
				TargetLabel:  "instance",
				Replacement:  `$1:cadvisor`,
			}),
			MetricRelabelConfigs: []*relabel.Config{
				dropByName(`container_(network_tcp_usage_total|network_udp_usage_total|tasks_state|cpu_load_average_10s)`),
			},
		},
	}, nil
}
