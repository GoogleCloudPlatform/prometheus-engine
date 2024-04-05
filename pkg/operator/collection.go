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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/go-logr/logr"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/secrets"
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
	// Collector secret.
	objFilterSecret := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      CollectionSecretName,
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
			&monitoringv1.PodMonitoring{},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Any update to a ClusterPodMonitoring requires regenerating the config.
		Watches(
			&monitoringv1.ClusterPodMonitoring{},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Any update to a ClusterNodeMonitoring requires regenerating the config.
		Watches(
			&monitoringv1.ClusterNodeMonitoring{},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// The configuration we generate for the collectors.
		Watches(
			&corev1.ConfigMap{},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterCollector),
		).
		// Detect and undo changes to the daemon set.
		Watches(
			&appsv1.DaemonSet{},
			enqueueConst(objRequest),
			builder.WithPredicates(
				objFilterCollector,
				predicate.GenerationChangedPredicate{},
			)).
		// Detect and undo changes to the secret.
		Watches(
			&corev1.Secret{},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterSecret)).
		Complete(newCollectionReconciler(op.manager.GetClient(), op.opts))
	if err != nil {
		return fmt.Errorf("create collector config controller: %w", err)
	}
	return nil
}

type collectionReconciler struct {
	client        client.Client
	opts          Options
	statusUpdates []monitoringv1.MonitoringCRD
}

func newCollectionReconciler(c client.Client, opts Options) *collectionReconciler {
	return &collectionReconciler{
		client: c,
		opts:   opts,
	}
}

func patchMonitoringStatus(ctx context.Context, kubeClient client.Client, obj client.Object, status *monitoringv1.MonitoringStatus) error {
	// TODO(TheSpiritXIII): In the future, change this to server side apply as opposed to patch.
	patchStatus := map[string]interface{}{
		"conditions":         status.Conditions,
		"observedGeneration": status.ObservedGeneration,
	}
	patchObject := map[string]interface{}{"status": patchStatus}

	patchBytes, err := json.Marshal(patchObject)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.MergePatchType, patchBytes)
	if err := kubeClient.Status().Patch(ctx, obj, patch); err != nil {
		return err
	}
	return nil
}

func (r *collectionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger, _ := logr.FromContext(ctx)
	logger.Info("reconciling collection")

	var config monitoringv1.OperatorConfig
	// Fetch OperatorConfig if it exists.
	if err := r.client.Get(ctx, req.NamespacedName, &config); apierrors.IsNotFound(err) {
		logger.Info("no operatorconfig created yet")
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("get operatorconfig for incoming: %q: %w", req.String(), err)
	}

	if err := r.ensureCollectorSecrets(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector secrets: %w", err)
	}
	// Deploy Prometheus collector as a node agent.
	if err := r.ensureCollectorDaemonSet(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector daemon set: %w", err)
	}

	if err := r.ensureCollectorConfig(ctx, &config.Collection, config.Features.Config.Compression, config.Exports); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector config: %w", err)
	}

	// Reconcile any status updates.
	for _, obj := range r.statusUpdates {
		if err := patchMonitoringStatus(ctx, r.client, obj, obj.GetMonitoringStatus()); err != nil {
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
			return fmt.Errorf("create collector secrets: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update collector secrets: %w", err)
	}
	return nil
}

// ensureCollectorDaemonSet populates the collector DaemonSet with operator-provided values.
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context, spec *monitoringv1.CollectionSpec) error {
	logger, _ := logr.FromContext(ctx)

	var ds appsv1.DaemonSet
	err := r.client.Get(ctx, client.ObjectKey{Namespace: r.opts.OperatorNamespace, Name: NameCollector}, &ds)
	// Some users deliberately not want to run the collectors. Only emit a warning but don't cause
	// retries as this logic gets re-triggered anyway if the DaemonSet is created later.
	if apierrors.IsNotFound(err) {
		logger.Error(err, "collector DaemonSet does not exist")
		return nil
	}
	if err != nil {
		return err
	}

	var projectID, location, cluster = resolveLabels(r.opts, spec.ExternalLabels)

	flags := []string{
		fmt.Sprintf("--export.label.project-id=%q", projectID),
		fmt.Sprintf("--export.label.location=%q", location),
		fmt.Sprintf("--export.label.cluster=%q", cluster),
	}
	// Populate export filtering from OperatorConfig.
	for _, matcher := range spec.Filter.MatchOneOf {
		flags = append(flags, fmt.Sprintf("--export.match=%q", matcher))
	}
	if spec.Credentials != nil {
		p := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials}))
		flags = append(flags, fmt.Sprintf("--export.credentials-file=%q", p))
	}

	if len(spec.Compression) > 0 && spec.Compression != monitoringv1.CompressionNone {
		flags = append(flags, fmt.Sprintf("--export.compression=%s", spec.Compression))
	}
	setContainerExtraArgs(ds.Spec.Template.Spec.Containers, CollectorPrometheusContainerName, strings.Join(flags, " "))

	return r.client.Update(ctx, &ds)
}

func resolveLabels(opts Options, externalLabels map[string]string) (projectID string, location string, cluster string) {
	// Prioritize OperatorConfig's external labels over operator's flags
	// to be consistent with our export layer's priorities.
	// This is to avoid confusion if users specify a project_id, location, and
	// cluster in the OperatorConfig's external labels but not in flags passed
	// to the operator - since on GKE environnments, these values are autopopulated
	// without user intervention.
	projectID = opts.ProjectID
	if p, ok := externalLabels[export.KeyProjectID]; ok {
		projectID = p
	}
	location = opts.Location
	if l, ok := externalLabels[export.KeyLocation]; ok {
		location = l
	}
	cluster = opts.Cluster
	if c, ok := externalLabels[export.KeyCluster]; ok {
		cluster = c
	}
	return
}

func gzipData(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func setConfigMapData(cm *corev1.ConfigMap, c monitoringv1.CompressionType, key string, data string) error {
	// Thanos config-reloader detects gzip compression automatically, so no sync with
	// config-reloaders is needed when switching between these.
	switch c {
	case monitoringv1.CompressionGzip:
		compressed, err := gzipData([]byte(data))
		if err != nil {
			return fmt.Errorf("gzip Prometheus config: %w", err)
		}

		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[key] = compressed
	case "", monitoringv1.CompressionNone:
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[key] = data
	default:
		return fmt.Errorf("unknown compression type: %q", c)
	}
	return nil
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (r *collectionReconciler) ensureCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec, compression monitoringv1.CompressionType, exports []monitoringv1.ExportSpec) error {
	cfg, err := r.makeCollectorConfig(ctx, spec, exports)
	if err != nil {
		return fmt.Errorf("generate Prometheus config: %w", err)
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal Prometheus config: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      NameCollector,
		},
	}
	if err := setConfigMapData(cm, compression, configFilename, string(cfgEncoded)); err != nil {
		return err
	}

	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return fmt.Errorf("create Prometheus config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update Prometheus config: %w", err)
	}
	return nil
}

type prometheusConfig struct {
	promconfig.Config `yaml:",inline"`

	// Secret management. Matches our fork's configuration.
	SecretConfigs []secrets.SecretConfig `yaml:"kubernetes_secrets,omitempty"`
}

func (r *collectionReconciler) makeCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec, exports []monitoringv1.ExportSpec) (*prometheusConfig, error) {
	logger, _ := logr.FromContext(ctx)

	cfg := &promconfig.Config{
		GlobalConfig: promconfig.GlobalConfig{
			ExternalLabels: labels.FromMap(spec.ExternalLabels),
		},
	}

	var err error
	cfg.ScrapeConfigs, err = makeKubeletScrapeConfigs(spec.KubeletScraping)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubelet scrape config: %w", err)
	}

	cfg.RemoteWriteConfigs, err = makeRemoteWriteConfig(exports)
	if err != nil {
		return nil, fmt.Errorf("failed to create export config: %w", err)
	}

	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podMons         monitoringv1.PodMonitoringList
		clusterPodMons  monitoringv1.ClusterPodMonitoringList
		clusterNodeMons monitoringv1.ClusterNodeMonitoringList
	)
	if err := r.client.List(ctx, &podMons); err != nil {
		return nil, fmt.Errorf("failed to list PodMonitorings: %w", err)
	}

	usedSecrets := monitoringv1.PrometheusSecretConfigs{}
	var projectID, location, cluster = resolveLabels(r.opts, spec.ExternalLabels)

	// Mark status updates in batch with single timestamp.
	for _, pm := range podMons.Items {
		// Reassign so we can safely get a pointer.
		pmon := pm

		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := pmon.ScrapeConfigs(projectID, location, cluster, usedSecrets)
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			//TODO: Fix ineffectual assignment. Intended behavior is unclear.
			//nolint:ineffassign
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

		change, err := pmon.Status.SetMonitoringCondition(pmon.GetGeneration(), metav1.Now(), cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting podmonitoring status state", "namespace", pmon.Namespace, "name", pmon.Name)
		}

		if change {
			r.statusUpdates = append(r.statusUpdates, &pmon)
		}
	}

	if err := r.client.List(ctx, &clusterPodMons); err != nil {
		return nil, fmt.Errorf("failed to list ClusterPodMonitorings: %w", err)
	}

	// Mark status updates in batch with single timestamp.
	for _, cm := range clusterPodMons.Items {
		// Reassign so we can safely get a pointer.
		cmon := cm

		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cmon.ScrapeConfigs(projectID, location, cluster, usedSecrets)
		if err != nil {
			msg := "generating scrape config failed for ClusterPodMonitoring endpoint"
			//TODO: Fix ineffectual assignment. Intended behavior is unclear.
			//nolint:ineffassign
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

		change, err := cmon.Status.SetMonitoringCondition(cmon.GetGeneration(), metav1.Now(), cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting clusterpodmonitoring status state", "namespace", cmon.Namespace, "name", cmon.Name)
		}

		if change {
			r.statusUpdates = append(r.statusUpdates, &cmon)
		}
	}

	// TODO(bwplotka): Warn about missing RBAC policies.
	// https://github.com/GoogleCloudPlatform/prometheus-engine/issues/789
	secretConfigs := usedSecrets.SecretConfigs()

	if err := r.client.List(ctx, &clusterNodeMons); err != nil {
		return nil, fmt.Errorf("failed to list ClusterNodeMonitorings: %w", err)
	}
	// The following job names are reserved by GMP for ClusterNodeMonitoring in the
	// gmp-system namespace. They will not be generated if kubeletScraping is enabled.
	var (
		reservedCAdvisorJobName = "gmp-kubelet-cadvisor"
		reservedKubeletJobName  = "gmp-kubelet-metrics"
	)
	// Mark status updates in batch with single timestamp.
	for _, cm := range clusterNodeMons.Items {
		if spec.KubeletScraping != nil && (cm.Name == reservedKubeletJobName || cm.Name == reservedCAdvisorJobName) {
			logger.Info("ClusterNodeMonitoring job %s was not applied because OperatorConfig.collector.kubeletScraping is enabled. kubeletScraping already includes the metrics in this job.", "name", cm.Name)
			continue
		}
		// Reassign so we can safely get a pointer.
		cm := cm
		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cm.ScrapeConfigs(projectID, location, cluster)
		if err != nil {
			msg := "generating scrape config failed for ClusterNodeMonitoring endpoint"
			//TODO: Fix ineffectual assignment. Intended behavior is unclear.
			//nolint:ineffassign
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", cm.Namespace, "name", cm.Name)
			continue
		}
		cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)

		change, err := cm.Status.SetMonitoringCondition(cm.GetGeneration(), metav1.Now(), cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			logger.Error(err, "setting clusternodemonitoring status state", "namespace", cm.Namespace, "name", cm.Name)
		}

		if change {
			r.statusUpdates = append(r.statusUpdates, &cm)
		}
	}

	// Sort to ensure reproducible configs.
	sort.Slice(cfg.ScrapeConfigs, func(i, j int) bool {
		return cfg.ScrapeConfigs[i].JobName < cfg.ScrapeConfigs[j].JobName
	})

	return &prometheusConfig{
		Config:        *cfg,
		SecretConfigs: secretConfigs,
	}, nil
}

type podMonitoringDefaulter struct{}

func (d *podMonitoringDefaulter) Default(_ context.Context, o runtime.Object) error {
	pm := o.(*monitoringv1.PodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}

type clusterPodMonitoringDefaulter struct{}

func (d *clusterPodMonitoringDefaulter) Default(_ context.Context, o runtime.Object) error {
	pm := o.(*monitoringv1.ClusterPodMonitoring)

	if pm.Spec.TargetLabels.Metadata == nil {
		md := []string{"namespace", "pod", "container"}
		pm.Spec.TargetLabels.Metadata = &md
	}
	return nil
}

// makeRemoteWriteConfig generate the configs for the Prometheus remote_write feature.
func makeRemoteWriteConfig(exports []monitoringv1.ExportSpec) ([]*promconfig.RemoteWriteConfig, error) {
	var exportConfigs []*promconfig.RemoteWriteConfig
	for _, export := range exports {
		url, err := url.Parse(export.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url: %w", err)
		}
		exportConfigs = append(exportConfigs,
			&promconfig.RemoteWriteConfig{
				URL: &config.URL{URL: url},
			})
	}
	return exportConfigs, nil
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
			CAFile:             "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		},
	}
	interval, err := prommodel.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid scrape interval: %w", err)
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
				dropByName(`container_(network_tcp_usage_total|network_udp_usage_total|tasks_state|cpu_load_average_10s|blkio_device_usage_total|memory_failures_total)`),
			},
		},
	}, nil
}
