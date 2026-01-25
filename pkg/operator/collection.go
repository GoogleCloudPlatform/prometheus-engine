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
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/config"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
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

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
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
			builder.WithPredicates(
				objFilterOperatorConfig,
				predicate.GenerationChangedPredicate{},
			),
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
	client client.Client
	opts   Options
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
		return fmt.Errorf("patch status: %w", err)
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
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("get operatorconfig for incoming: %q: %w", req.String(), err)
	}

	if err := r.ensureCollectorSecrets(ctx, &config.Collection); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector secrets: %w", err)
	}
	// Deploy Prometheus collector as a node agent.
	if changed, err := r.ensureCollectorDaemonSet(ctx, &config); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector daemon set: %w", err)
	} else if changed {
		if err := patchMonitoringStatus(ctx, r.client, &config, config.GetMonitoringStatus()); err != nil {
			return reconcile.Result{}, fmt.Errorf("patch operatorconfig status: %w", err)
		}
	}
	if err := r.ensureCollectorConfig(ctx, &config.Collection, config.Features.Config.Compression, config.Exports); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure collector config: %w", err)
	}

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
func (r *collectionReconciler) ensureCollectorDaemonSet(ctx context.Context, config *monitoringv1.OperatorConfig) (bool, error) {
	logger, _ := logr.FromContext(ctx)

	// Build the DaemonSet.
	// For now, we only ensure the DaemonSet exists.
	// In the future, we might want to reconcile the DaemonSet spec as well.
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NameCollector,
			Namespace: r.opts.OperatorNamespace,
		},
	}

	err := r.client.Get(ctx, client.ObjectKeyFromObject(ds), ds)
	// Some users deliberately not want to run the collectors. Only emit a warning but don't cause
	// retries as this logic gets re-triggered anyway if the DaemonSet is created later.
	cond := &monitoringv1.MonitoringCondition{
		Type:   monitoringv1.CollectorDaemonSetExists,
		Status: corev1.ConditionTrue,
	}
	if apierrors.IsNotFound(err) {
		logger.Error(err, "collector DaemonSet does not exist")
		cond.Status = corev1.ConditionFalse
		cond.Reason = "DaemonSetMissing"
		cond.Message = "Collector DaemonSet does not exist."
		err = nil
	}
	return config.Status.SetMonitoringCondition(config.GetGeneration(), metav1.Now(), cond), err
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
func (r *collectionReconciler) ensureCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec, configCompression monitoringv1.CompressionType, exports []monitoringv1.ExportSpec) error {
	cfg, updates, err := r.makeCollectorConfig(ctx, spec, exports)
	if err != nil {
		return fmt.Errorf("generate Prometheus config: %w", err)
	}

	// NOTE(bwplotka): Match logic will be removed in https://github.com/GoogleCloudPlatform/prometheus-engine/pull/1688
	// nolint:staticcheck
	cfg.GoogleCloud.Export.Match = spec.Filter.MatchOneOf
	if string(spec.Compression) != "" {
		cfg.GoogleCloud.Export.Compression = string(spec.Compression)
	}
	if spec.Credentials != nil {
		credentialsFile := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials}))
		cfg.GoogleCloud.Export.CredentialsFile = credentialsFile
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
	if err := setConfigMapData(cm, configCompression, configFilename, string(cfgEncoded)); err != nil {
		return err
	}

	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
			return fmt.Errorf("create Prometheus config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update Prometheus config: %w", err)
	}

	// Reconcile any status updates.
	var errs []error
	for _, update := range updates {
		// Copy status in case we update both spec and status. Updating one reverts the other.
		statusUpdate := update.object.DeepCopyObject().(monitoringv1.MonitoringCRD).GetMonitoringStatus()
		if update.spec {
			// The status will be reverted, but our generation ID will be updated.
			if err := r.client.Update(ctx, update.object); err != nil {
				errs = append(errs, err)
			}
		}

		if update.status {
			// Use the object with the latest generation ID.
			if err := patchMonitoringStatus(ctx, r.client, update.object, statusUpdate); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

type update struct {
	object monitoringv1.MonitoringCRD
	spec   bool
	status bool
}

// makeCollectorConfig returns the Prometheus configuration based on the scrape configurations, the
// list of objects to update and any error.
func (r *collectionReconciler) makeCollectorConfig(ctx context.Context, spec *monitoringv1.CollectionSpec, exports []monitoringv1.ExportSpec) (*promconfig.Config, []update, error) {
	logger, _ := logr.FromContext(ctx)

	cfg := &promconfig.Config{
		GlobalConfig: promconfig.GlobalConfig{
			ExternalLabels: labels.FromMap(spec.ExternalLabels),
		},
	}

	var err error
	cfg.ScrapeConfigs, err = spec.ScrapeConfigs()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubelet scrape config: %w", err)
	}

	cfg.RemoteWriteConfigs, err = makeRemoteWriteConfig(exports)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create export config: %w", err)
	}

	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podMons         monitoringv1.PodMonitoringList
		clusterPodMons  monitoringv1.ClusterPodMonitoringList
		clusterNodeMons monitoringv1.ClusterNodeMonitoringList
	)
	if err := r.client.List(ctx, &podMons); err != nil {
		return nil, nil, fmt.Errorf("failed to list PodMonitorings: %w", err)
	}

	usedSecrets := monitoringv1.PrometheusSecretConfigs{}
	projectID, location, cluster := resolveLabels(r.opts.ProjectID, r.opts.Location, r.opts.Cluster, spec.ExternalLabels)
	var updates []update

	// Mark status updates in batch with single timestamp.
	for _, pmon := range podMons.Items {
		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := pmon.ScrapeConfigs(projectID, location, cluster, usedSecrets)
		if err != nil {
			msg := "generating scrape config failed for PodMonitoring endpoint"
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", pmon.Namespace, "name", pmon.Name)
		} else {
			cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)
		}

		updateStatus := pmon.Status.SetMonitoringCondition(pmon.GetGeneration(), metav1.Now(), cond)
		if updateStatus {
			updates = append(updates, update{
				object: &pmon,
				status: updateStatus,
			})
		}
	}

	if err := r.client.List(ctx, &clusterPodMons); err != nil {
		return nil, nil, fmt.Errorf("failed to list ClusterPodMonitorings: %w", err)
	}

	// Mark status updates in batch with single timestamp.
	for _, cmon := range clusterPodMons.Items {
		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cmon.ScrapeConfigs(projectID, location, cluster, usedSecrets)
		if err != nil {
			msg := "generating scrape config failed for ClusterPodMonitoring endpoint"
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", cmon.Namespace, "name", cmon.Name)
		} else {
			cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)
		}

		updateStatus := cmon.Status.SetMonitoringCondition(cmon.GetGeneration(), metav1.Now(), cond)
		if updateStatus {
			updates = append(updates, update{
				object: &cmon,
				status: updateStatus,
			})
		}
	}

	// TODO(bwplotka): Warn about missing RBAC policies.
	// https://github.com/GoogleCloudPlatform/prometheus-engine/issues/789
	cfg.SecretConfigs = usedSecrets.SecretConfigs()

	if err := r.client.List(ctx, &clusterNodeMons); err != nil {
		return nil, nil, fmt.Errorf("failed to list ClusterNodeMonitorings: %w", err)
	}
	// The following job names are reserved by GMP for ClusterNodeMonitoring in the
	// gmp-system namespace. They will not be generated if kubeletScraping is enabled.
	var (
		reservedCAdvisorJobName = "gmp-kubelet-cadvisor"
		reservedKubeletJobName  = "gmp-kubelet-metrics"
	)
	// Mark status updates in batch with single timestamp.
	for _, cnmon := range clusterNodeMons.Items {
		if spec.KubeletScraping != nil && (cnmon.Name == reservedKubeletJobName || cnmon.Name == reservedCAdvisorJobName) {
			logger.Info("ClusterNodeMonitoring job %s was not applied because OperatorConfig.collector.kubeletScraping is enabled. kubeletScraping already includes the metrics in this job.", "name", cnmon.Name)
			continue
		}
		cond := &monitoringv1.MonitoringCondition{
			Type:   monitoringv1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		cfgs, err := cnmon.ScrapeConfigs(projectID, location, cluster)
		if err != nil {
			msg := "generating scrape config failed for ClusterNodeMonitoring endpoint"
			cond = &monitoringv1.MonitoringCondition{
				Type:    monitoringv1.ConfigurationCreateSuccess,
				Status:  corev1.ConditionFalse,
				Reason:  "ScrapeConfigError",
				Message: msg,
			}
			logger.Error(err, msg, "namespace", cnmon.Namespace, "name", cnmon.Name)
		} else {
			cfg.ScrapeConfigs = append(cfg.ScrapeConfigs, cfgs...)
		}
		if cnmon.Status.SetMonitoringCondition(cnmon.GetGeneration(), metav1.Now(), cond) {
			updates = append(updates, update{
				object: &cnmon,
				status: true,
			})
		}
	}

	// Sort to ensure reproducible configs.
	sort.Slice(cfg.ScrapeConfigs, func(i, j int) bool {
		return cfg.ScrapeConfigs[i].JobName < cfg.ScrapeConfigs[j].JobName
	})

	return cfg, updates, nil
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
