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
	"crypto/tls"
	"fmt"
	stdlog "log"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/pkg/relabel"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	clientset "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	informers "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/informers/externalversions"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/rules"
)

const (
	// DefaultOperatorNamespace is the namespace in which all resources owned by the operator are installed.
	DefaultOperatorNamespace = "gpe-system"

	// Fixed names used in various resources managed by the operator.
	NameOperator       = "gpe-operator"
	nameRulesGenerated = "rules-generated"

	// The official images to be used with this version of the operator. For debugging
	// and emergency use cases they may be overwritten through options.
	// TODO(freinartz): start setting official versioned images once we start releases.
	ImageCollector      = "gcr.io/gke-release-staging/prometheus-engine/prometheus:v2.26.1-gpe.2-gke.0"
	ImageConfigReloader = "gcr.io/gke-release-staging/prometheus-engine/gpe-config-reloader:v0.0.0.gke.0"

	// Kubernetes resource label mapping values.
	kubeLabelPrefix    = model.MetaLabelPrefix + "kubernetes_"
	podLabelPrefix     = kubeLabelPrefix + "pod_label_"
	serviceLabelPrefix = kubeLabelPrefix + "service_label_"

	// Keys for reconciling different sets of operator-managed resources.
	keyRules = "__rules__"
)

var (
	metricOperatorSyncLatency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "operator_sync_latency",
			Namespace: "gpe_operator",
			Help:      "The time it takes for operator to synchronize with managed CRDs (s).",
		},
	)
)

// Operator to implement managed collection for Google Prometheus Engine.
type Operator struct {
	logger         log.Logger
	opts           Options
	kubeClient     kubernetes.Interface
	operatorClient clientset.Interface

	manager manager.Manager

	// Informers that maintain a cache of cluster resources and call configured
	// event handlers on changes.
	informerRules cache.SharedIndexInformer
	// State changes are enqueued into a rate limited work queue, which ensures
	// the operator does not get overloaded and multiple changes to the same resource
	// are not handled in parallel, leading to semantic race conditions.
	queue workqueue.RateLimitingInterface

	// Internal bookkeeping for sending status updates to processed CRDs.
	statusState *CRDStatusState
}

// Options for the Operator.
type Options struct {
	// ID of the project of the cluster.
	ProjectID string
	// Name of the cluster the operator acts on.
	Cluster string
	// Namespace to which the operator deploys any associated resources.
	OperatorNamespace string
	// Listening port of the collector. Configurable to allow multiple
	// simultanious collector deployments for testing purposes while each
	// collector runs on the host network.
	CollectorPort int32
	// Image for the Prometheus collector container.
	ImageCollector string
	// Image for the Prometheus config reloader.
	ImageConfigReloader string
	// Priority class for the collector pods.
	PriorityClass string
	// Endpoint of the Cloud Monitoring API to be used by all collectors.
	CloudMonitoringEndpoint string
	// Self-sign or solicit kube-apiserver as CA to sign TLS certificate.
	CASelfSign bool
	// Webhook serving address.
	ListenAddr string
}

func (o *Options) defaultAndValidate(logger log.Logger) error {
	if o.OperatorNamespace == "" {
		o.OperatorNamespace = DefaultOperatorNamespace
	}
	if o.CollectorPort == 0 {
		o.CollectorPort = 9090
	}
	if o.ImageCollector == "" {
		o.ImageCollector = ImageCollector
	}
	if o.ImageConfigReloader == "" {
		o.ImageConfigReloader = ImageConfigReloader
	}

	if o.ProjectID == "" {
		return errors.New("ProjectID must be set")
	}
	if o.Cluster == "" {
		return errors.New("Cluster must be set")
	}
	if o.ImageCollector != ImageCollector {
		level.Warn(logger).Log("msg", "not using the canonical collector image",
			"expected", ImageCollector, "got", o.ImageCollector)
	}
	if o.ImageConfigReloader != ImageConfigReloader {
		level.Warn(logger).Log("msg", "not using the canonical config reloader image",
			"expected", ImageConfigReloader, "got", o.ImageConfigReloader)
	}
	return nil
}

// New instantiates a new Operator.
func New(logger log.Logger, clientConfig *rest.Config, registry prometheus.Registerer, opts Options) (*Operator, error) {
	if err := opts.defaultAndValidate(logger); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	sc := runtime.NewScheme()

	if err := scheme.AddToScheme(sc); err != nil {
		return nil, errors.Wrap(err, "add Kubernetes core scheme")
	}
	if err := monitoringv1alpha1.AddToScheme(sc); err != nil {
		return nil, errors.Wrap(err, "add monitoringv1alpha1 scheme")
	}
	mgr, err := ctrl.NewManager(clientConfig, manager.Options{
		Scheme: sc,
		// Don't run a metrics server with the manager. Metrics are being served
		// explicitly in the main routine.
		MetricsBindAddress: "0",
	})
	if err != nil {
		return nil, errors.Wrap(err, "create controller manager")
	}

	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build Kubernetes clientset")
	}
	operatorClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build operator clientset")
	}

	const syncInterval = 5 * time.Minute

	informerFactory := informers.NewSharedInformerFactory(operatorClient, syncInterval)

	if registry != nil {
		registry.MustRegister(metricOperatorSyncLatency)
	}

	op := &Operator{
		logger:         logger,
		opts:           opts,
		kubeClient:     kubeClient,
		operatorClient: operatorClient,
		manager:        mgr,
		informerRules:  informerFactory.Monitoring().V1alpha1().Rules().Informer(),
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "GPEOperator"),
		statusState:    NewCRDStatusState(metav1.Now),
	}

	// Reconcile rules ...
	// ... on changes to any Rules resource.
	op.informerRules.AddEventHandler(unifiedEventHandler(op.enqueueKey(keyRules)))

	// ... on changes to the generated rule ConfigMap.
	op.informerRules.AddEventHandler(unifiedEventHandler(func(o interface{}) {
		if cm := o.(*monitoringv1alpha1.Rules); cm.Name == nameRulesGenerated {
			op.queue.Add(keyRules)
		}
	}))

	return op, nil
}

func unifiedEventHandler(f func(o interface{})) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    f,
		DeleteFunc: f,
		UpdateFunc: ifResourceVersionChanged(f),
	}
}

// enqueueObject enqueues the object for reconciliation. Only the key is enqueued
// as the queue consumer should retrieve the most recent cache object once it gets to process
// to not process stale state.
func (o *Operator) enqueueObject(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtimeutil.HandleError(err)
		return
	}
	o.queue.Add(key)
}

// enqueueKey enqueues the given reconciliation key. The returned function takes
// an unused argument to make it easy to use with event handlers.
func (o *Operator) enqueueKey(key string) func(interface{}) {
	return func(interface{}) { o.queue.Add(key) }
}

// ifResourceVersionChanged returns an UpdateFunc handler that calls fn with the
// new object if the resource version changed between old and new.
// This prevents superfluous reconciliations as the cache is resynced periodically,
// which will trigger no-op updates.
func ifResourceVersionChanged(fn func(interface{})) func(oldObj, newObj interface{}) {
	return func(oldObj, newObj interface{}) {
		old := oldObj.(metav1.Object)
		new := newObj.(metav1.Object)
		if old.GetResourceVersion() != new.GetResourceVersion() {
			fn(newObj)
		}

	}
}

// InitAdmissionResources sets state for the operator before monitoring for resources.
// It returns a web server for handling Kubernetes admission controller webhooks.
func (o *Operator) InitAdmissionResources(ctx context.Context, ors ...metav1.OwnerReference) (*http.Server, error) {
	// Persisting TLS keypair to a k8s secret seems like unnecessary state to manage.
	// It's fairly trivial to re-generate the cert and private
	// key on each startup. Also no other GPE resources aside from the operator
	// rely on the keypair.
	// A downside to this approach is re-writing the validation webhook config
	// every time with the new caBundle. This should only happen when the operator
	// restarts, which should be infrequent.
	var (
		crt, key []byte
		err      error
		podEp    = "/validate/podmonitorings"
		fqdn     = fmt.Sprintf("%s.%s.svc", NameOperator, o.opts.OperatorNamespace)
	)

	// Generate cert/key pair - self-signed CA or kube-apiserver CA.
	if o.opts.CASelfSign {
		crt, key, err = cert.GenerateSelfSignedCertKey(fqdn, nil, nil)
		if err != nil {
			return nil, err
		}
	} else {
		crt, key, err = CreateSignedKeyPair(ctx, o.kubeClient, fqdn)
		if err != nil {
			return nil, err
		}
	}

	// Idempotently request validation webhook spec with caBundle and endpoints.
	_, err = UpsertValidatingWebhookConfig(ctx,
		o.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
		ValidatingWebhookConfig(NameOperator, o.opts.OperatorNamespace, crt, []string{podEp}, ors...))
	if err != nil {
		return nil, err
	}

	// Setup HTTPS server.
	var (
		tlsCfg = new(tls.Config)
		mux    = http.NewServeMux()
		as     = NewAdmissionServer(o.logger)
	)

	// Handle validation resource endpoints.
	mux.HandleFunc(podEp, as.serveAdmission(admitPodMonitoring))

	// Init TLS config with key pair.
	if c, err := tls.X509KeyPair(crt, key); err != nil {
		return nil, err
	} else {
		tlsCfg.Certificates = append(tlsCfg.Certificates, c)
	}

	return &http.Server{
		Addr:      o.opts.ListenAddr,
		ErrorLog:  stdlog.New(log.NewStdlibAdapter(o.logger), "", stdlog.LstdFlags),
		TLSConfig: tlsCfg,
		Handler:   mux,
	}, nil
}

// Run the reconciliation loop of the operator.
func (o *Operator) Run(ctx context.Context) error {
	defer runtimeutil.HandleCrash()

	if err := setupCollectionControllers(ctx, o, o.manager); err != nil {
		return errors.Wrap(err, "setup collection controllers")
	}
	// TODO(fabxc): start controller manager in the background for now. After migration to
	// controller manager is complete, it must be blocking.
	go o.manager.Start(ctx)

	level.Info(o.logger).Log("msg", "starting GPE operator")

	go o.informerRules.Run(ctx.Done())

	level.Info(o.logger).Log("msg", "waiting for informer caches to sync")

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	ok := cache.WaitForNamedCacheSync(
		"GPEOperator", syncCtx.Done(),
		o.informerRules.HasSynced,
	)
	cancel()
	if !ok {
		return errors.New("aborted while waiting for informer caches to sync (are the CRDs installed?)")
	}

	level.Info(o.logger).Log("msg", "informer cache sync complete")

	// Process work items until context is canceled.
	go func() {
		<-ctx.Done()
		o.queue.ShutDown()
	}()

	// Trigger an initial sync even if no instances of the watched resources exists yet.
	o.enqueueKey(keyRules)(nil)

	for o.processNextItem(ctx) {
	}
	return nil
}

func (o *Operator) processNextItem(ctx context.Context) bool {
	key, quit := o.queue.Get()
	if quit {
		return false
	}
	defer o.queue.Done(key)

	// For simplicity, we use a single timeout for the entire synchronization.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := o.sync(ctx, key.(string)); err == nil {
		// Drop item from rate limit tracking as we successfully processed it.
		// If the item is enqueued again, we'll immediately process it.
		o.queue.Forget(key)
	} else {
		runtimeutil.HandleError(errors.Wrap(err, fmt.Sprintf("sync for %q failed", key)))
		// Requeue the item with backoff to retry on transient errors.
		o.queue.AddRateLimited(key)
	}
	return true
}

func (o *Operator) sync(ctx context.Context, key string) error {
	// Record total time to sync resources.
	defer func(now time.Time) {
		metricOperatorSyncLatency.Set(float64(time.Since(now).Seconds()))
	}(time.Now())

	level.Info(o.logger).Log("msg", "syncing cluster state for key", "key", key)

	switch key {
	case keyRules:
		if err := o.ensureRuleConfigs(ctx); err != nil {
			return errors.Wrap(err, "ensure rule configmaps")
		}

	default:
		return errors.Errorf("expected global reconciliation but got key %q", key)
	}
	return nil
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
func (o *Operator) ensureCollectorDaemonSet(ctx context.Context, c client.Client) error {
	ds := o.makeCollectorDaemonSet()

	if err := c.Update(ctx, ds); apierrors.IsNotFound(err) {
		if err := c.Create(ctx, ds); err != nil {
			return errors.Wrap(err, "create collector DaemonSet")
		}
	} else if err != nil {
		return errors.Wrap(err, "update collector DaemonSet")
	}
	return nil
}

func (o *Operator) makeCollectorDaemonSet() *appsv1.DaemonSet {
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
		fmt.Sprintf("--web.listen-address=:%d", o.opts.CollectorPort),
		"--web.enable-lifecycle",
		"--web.route-prefix=/",
	}
	if o.opts.CloudMonitoringEndpoint != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--export.endpoint=%s", o.opts.CloudMonitoringEndpoint))
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
						Image: o.opts.ImageCollector,
						// Set an aggressive GC threshold (default is 100%). Since the collector has a lot of
						// long-lived allocations, this still doesn't result in a high GC rate (compared to stateless
						// RPC applications) and gives us a more balanced ratio of memory and CPU usage.
						Env: []corev1.EnvVar{
							{Name: "GOGC", Value: "25"},
						},
						Args: collectorArgs,
						Ports: []corev1.ContainerPort{
							{Name: "prometheus-http", ContainerPort: o.opts.CollectorPort},
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
						Image: o.opts.ImageConfigReloader,
						Args: []string{
							fmt.Sprintf("--config-file=%s", path.Join(collectorConfigDir, collectorConfigFilename)),
							fmt.Sprintf("--config-file-output=%s", path.Join(collectorConfigOutDir, collectorConfigFilename)),
							fmt.Sprintf("--reload-url=http://localhost:%d/-/reload", o.opts.CollectorPort),
							fmt.Sprintf("--listen-address=:%d", o.opts.CollectorPort+1),
						},
						// Pass node name so the config can filter for targets on the local node,
						Env: []corev1.EnvVar{
							{
								Name: envVarNodeName,
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							},
						},
						Ports: []corev1.ContainerPort{
							{Name: "reloader-http", ContainerPort: o.opts.CollectorPort + 1},
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
				PriorityClassName:  o.opts.PriorityClass,
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
			Namespace: o.opts.OperatorNamespace,
			Name:      CollectorName,
		},
		Spec: spec,
	}
	return ds
}

// updateCRDStatus iterates through parsed CRDs and updates their statuses.
// If an error is encountered from performing an update, the function returns
// the error immediately and does not attempt updates on subsequent CRDs.
func (o *Operator) updateCRDStatus(ctx context.Context, c client.Client) error {
	for _, pm := range o.statusState.PodMonitorings() {
		if err := c.Status().Update(ctx, &pm); err != nil {
			return err
		}
	}
	return nil
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (o *Operator) ensureCollectorConfig(ctx context.Context, c client.Client) error {
	cfg, err := o.makeCollectorConfig(ctx, c)
	if err != nil {
		return errors.Wrap(err, "generate Prometheus config")
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, "marshal Prometheus config")
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.opts.OperatorNamespace,
			Name:      CollectorName,
		},
		Data: map[string]string{
			collectorConfigFilename: string(cfgEncoded),
		},
	}

	if err := c.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := c.Create(ctx, cm); err != nil {
			return errors.Wrap(err, "create Prometheus config")
		}
	} else if err != nil {
		return errors.Wrap(err, "update Prometheus config")
	}
	return nil
}

func (o *Operator) makeCollectorConfig(ctx context.Context, c client.Client) (*promconfig.Config, error) {
	var scrapeCfgs []*promconfig.ScrapeConfig
	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var (
		podmons    monitoringv1alpha1.PodMonitoringList
		scrapecfgs corev1.ConfigMapList
	)
	if err := c.List(ctx, &podmons); err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
	}
	if err := c.List(ctx, &scrapecfgs, client.MatchingLabels{"type": "scrape-config"}); err != nil {
		return nil, errors.Wrap(err, "failed to list scrape ConfigMaps")
	}

	// Mark status updates in batch with single timestamp.
	for _, podmon := range podmons.Items {
		cond := &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		for i := range podmon.Spec.Endpoints {
			scrapeCfg, err := makePodScrapeConfig(&podmon, i)
			if err != nil {
				cond.Status = corev1.ConditionFalse
				level.Warn(o.logger).Log("msg", "generating scrape config failed for PodMonitoring endpoint",
					"err", err, "namespace", podmon.Namespace, "name", podmon.Name, "endpoint", i)
				continue
			}
			scrapeCfgs = append(scrapeCfgs, scrapeCfg)
		}
		err := o.statusState.SetPodMonitoringCondition(&podmon, cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			level.Error(o.logger).Log(
				"msg", "setting podmonitoring status state",
				"err", err)
		}
	}

	// Load additional, hard-coded scrape configs from configmaps in the oeprator's namespace.
	for _, cm := range scrapecfgs.Items {
		const key = "config.yaml"

		var promcfg promconfig.Config
		if err := yaml.Unmarshal([]byte(cm.Data[key]), &promcfg); err != nil {
			level.Error(o.logger).Log("msg", "cannot parse scrape config, skipping ...",
				"err", err, "namespace", cm.Namespace, "name", cm.Name)
			continue
		}
		for _, sc := range promcfg.ScrapeConfigs {
			// Make scrape config name unique and traceable.
			sc.JobName = fmt.Sprintf("ConfigMap/%s/%s/%s", o.opts.OperatorNamespace, cm.Name, sc.JobName)
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

// Environment variable interpolated by the config reloader sidecar.
const envVarNodeName = "NODE_NAME"

func makePodScrapeConfig(podmon *monitoringv1alpha1.PodMonitoring, index int) (*promconfig.ScrapeConfig, error) {
	// Configure how Prometheus talks to the Kubernetes API server to discover targets.
	// This configuration is the same for all scrape jobs (esp. selectors).
	// This ensures that Prometheus can reuse the underlying client and caches, which reduces
	// load on the Kubernetes API server.
	discoveryCfgs := discovery.Configs{
		&discoverykube.SDConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			Role:             discoverykube.RolePod,
			// Drop all potential targets not the same node as the collector. The $(NODE_NAME) variable
			// is interpolated by the config reloader sidecar before the config reaches the Prometheus collector.
			// Doing it through selectors rather than relabeling should substantially reduce the client and
			// server side load.
			Selectors: []discoverykube.SelectorConfig{
				{
					Role:  discoverykube.RolePod,
					Field: fmt.Sprintf("spec.nodeName=$(%s)", envVarNodeName),
				},
			},
		},
	}

	ep := podmon.Spec.Endpoints[index]

	// TODO(freinartz): validate all generated regular expressions.
	relabelCfgs := []*relabel.Config{
		// Filter targets by namespace of the PodMonitoring configuration.
		{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
			Regex:        relabel.MustNewRegexp(podmon.Namespace),
		},
	}

	// Filter targets that belong to selected services.

	// Simple equal matchers. Sort by keys first to ensure that generated configs are reproducible.
	// (Go map iteration is non-deterministic.)
	var selectorKeys []string
	for k := range podmon.Spec.Selector.MatchLabels {
		selectorKeys = append(selectorKeys, k)
	}
	sort.Strings(selectorKeys)

	for _, k := range selectorKeys {
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(k)},
			Regex:        relabel.MustNewRegexp(podmon.Spec.Selector.MatchLabels[k]),
		})
	}
	// Expression matchers are mapped to relabeling rules with the same behavior.
	for _, exp := range podmon.Spec.Selector.MatchExpressions {
		switch exp.Operator {
		case metav1.LabelSelectorOpIn:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp(strings.Join(exp.Values, "|")),
			})
		case metav1.LabelSelectorOpNotIn:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp(strings.Join(exp.Values, "|")),
			})
		case metav1.LabelSelectorOpExists:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_labelpresent_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		case metav1.LabelSelectorOpDoesNotExist:
			relabelCfgs = append(relabelCfgs, &relabel.Config{
				Action:       relabel.Drop,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_labelpresent_" + sanitizeLabelName(exp.Key)},
				Regex:        relabel.MustNewRegexp("true"),
			})
		}
	}
	// Filter targets by the configured port.
	var portLabel prommodel.LabelName
	var portValue string

	if ep.Port.StrVal != "" {
		portLabel = "__meta_kubernetes_pod_container_port_name"
		portValue = ep.Port.StrVal
	} else if ep.Port.IntVal != 0 {
		portLabel = "__meta_kubernetes_pod_container_port_number"
		portValue = strconv.FormatUint(uint64(ep.Port.IntVal), 10)
	} else {
		return nil, errors.New("port must be set for PodMonitoring")
	}

	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:       relabel.Keep,
		SourceLabels: prommodel.LabelNames{portLabel},
		Regex:        relabel.MustNewRegexp(portValue),
	})

	// Set a clean namespace, job, and instance label that provide sufficient uniqueness.
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:       relabel.Replace,
		SourceLabels: prommodel.LabelNames{"__meta_kubernetes_namespace"},
		TargetLabel:  "namespace",
	})
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:      relabel.Replace,
		Replacement: podmon.Name,
		TargetLabel: "job",
	})
	// The instance label being the pod name would be ideal UX-wise. But we cannot be certain
	// that multiple metrics endpoints on a pod don't expose metrics with the same name. Thus
	// we have to disambiguate along the port as well.
	relabelCfgs = append(relabelCfgs, &relabel.Config{
		Action:       relabel.Replace,
		SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_name", portLabel},
		Regex:        relabel.MustNewRegexp("(.+);(.+)"),
		Replacement:  "$1:$2",
		TargetLabel:  "instance",
	})

	// Incorporate k8s label remappings from CRD.
	if pCfgs, err := labelMappingRelabelConfigs(podmon.Spec.TargetLabels.FromPod, podLabelPrefix); err != nil {
		return nil, errors.Wrap(err, "invalid PodMonitoring target labels")
	} else {
		relabelCfgs = append(relabelCfgs, pCfgs...)
	}

	interval, err := prommodel.ParseDuration(ep.Interval)
	if err != nil {
		return nil, errors.Wrap(err, "invalid scrape interval")
	}
	timeout := interval
	if ep.Timeout != "" {
		timeout, err = prommodel.ParseDuration(ep.Timeout)
		if err != nil {
			return nil, errors.Wrap(err, "invalid scrape timeout")
		}
	}

	metricsPath := "/metrics"
	if ep.Path != "" {
		metricsPath = ep.Path
	}

	return &promconfig.ScrapeConfig{
		// Generate a job name to make it easy to track what generated the scrape configuration.
		// The actual job label attached to its metrics is overwritten via relabeling.
		JobName:                 fmt.Sprintf("PodMonitoring/%s/%s/%s", podmon.Namespace, podmon.Name, portValue),
		ServiceDiscoveryConfigs: discoveryCfgs,
		MetricsPath:             metricsPath,
		ScrapeInterval:          interval,
		ScrapeTimeout:           timeout,
		RelabelConfigs:          relabelCfgs,
	}, nil
}

// labelMappingRelabelConfigs generates relabel configs using a provided mapping and resource prefix.
func labelMappingRelabelConfigs(mappings []monitoringv1alpha1.LabelMapping, prefix model.LabelName) ([]*relabel.Config, error) {
	var relabelCfgs []*relabel.Config
	for _, m := range mappings {
		if collision := isPrometheusTargetLabel(m.To); collision {
			return nil, fmt.Errorf("relabel %q to %q conflicts with GPE target schema", m.From, m.To)
		}
		// `To` can be unset, default to `From`.
		if m.To == "" {
			m.To = m.From
		}
		relabelCfgs = append(relabelCfgs, &relabel.Config{
			Action:       relabel.Replace,
			SourceLabels: prommodel.LabelNames{prefix + sanitizeLabelName(m.From)},
			TargetLabel:  m.To,
		})
	}
	return relabelCfgs, nil
}

// isPrometheusTargetLabel returns true if the label argument is in use by the Prometheus target schema.
func isPrometheusTargetLabel(label string) bool {
	switch label {
	case export.KeyProjectID, export.KeyLocation, export.KeyCluster, export.KeyNamespace, export.KeyJob, export.KeyInstance:
		return true
	default:
		return false
	}
}

var invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeLabelName reproduces the label name cleanup Prometheus's service discovery applies.
func sanitizeLabelName(name string) prommodel.LabelName {
	return prommodel.LabelName(invalidLabelCharRE.ReplaceAllString(name, "_"))
}

func (o *Operator) ensureRuleConfigs(ctx context.Context) error {
	// Re-generate the configmap that's loaded by the rule-evaluator.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nameRulesGenerated,
			Namespace: o.opts.OperatorNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "rule-evaluator",
			},
		},
		// Ensure there's always at least an empty dummy file as the evaluator
		// expects at least one match.
		Data: map[string]string{
			"empty.yaml": "",
		},
	}

	// Generate a final rule file for each Rules resource.
	err := cache.ListAll(o.informerRules.GetStore(), labels.Everything(), func(obj interface{}) {
		apiRules := obj.(*monitoringv1alpha1.Rules)
		logger := log.With(o.logger, "namespace", apiRules.Namespace, "name", apiRules.Name)

		rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
		if err != nil {
			level.Warn(logger).Log("msg", "converting rules failed", "err", err)
			// TODO(freinartz): update resource condition.
			return
		}
		lset := map[string]string{}
		// Populate isolation level from the defined scope.
		switch apiRules.Spec.Scope {
		case monitoringv1alpha1.ScopeCluster:
			lset[export.KeyProjectID] = o.opts.ProjectID
			lset[export.KeyCluster] = o.opts.Cluster
		case monitoringv1alpha1.ScopeNamespace:
			lset[export.KeyProjectID] = o.opts.ProjectID
			lset[export.KeyCluster] = o.opts.Cluster
			lset[export.KeyNamespace] = apiRules.Namespace
		default:
			level.Warn(logger).Log("msg", "unexpected scope", "scope", apiRules.Spec.Scope)
			// TODO(freinartz): update resource condition.
			return
		}
		if err := rules.Scope(&rs, lset); err != nil {
			level.Warn(logger).Log("msg", "isolating rules failed", "err", err)
			// TODO(freinartz): update resource condition.
			return
		}
		result, err := yaml.Marshal(rs)
		if err != nil {
			level.Warn(logger).Log("msg", "marshalling rules failed", "err", err)
			// TODO(freinartz): update resource condition.
			return
		}
		filename := fmt.Sprintf("%s__%s.yaml", apiRules.Namespace, apiRules.Name)
		cm.Data[filename] = string(result)
	})
	if err != nil {
		return errors.Wrap(err, "failed to list Rules")
	}

	// Create or update generated rule ConfigMap.
	_, err = o.kubeClient.CoreV1().ConfigMaps(o.opts.OperatorNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		_, err = o.kubeClient.CoreV1().ConfigMaps(o.opts.OperatorNamespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "create generated rules")
		}
	} else if err != nil {
		return errors.Wrap(err, "update generated rules")
	}
	return nil
}

// namespacedNamePredicate is an event filter predicate that only allows events with
// a single object.
type namespacedNamePredicate struct {
	namespace string
	name      string
}

func (o namespacedNamePredicate) Create(e event.CreateEvent) bool {
	return e.Object.GetNamespace() == o.namespace && e.Object.GetName() == o.name
}
func (o namespacedNamePredicate) Update(e event.UpdateEvent) bool {
	return e.ObjectNew.GetNamespace() == o.namespace && e.ObjectNew.GetName() == o.name
}
func (o namespacedNamePredicate) Delete(e event.DeleteEvent) bool {
	return e.Object.GetNamespace() == o.namespace && e.Object.GetName() == o.name
}
func (o namespacedNamePredicate) Generic(e event.GenericEvent) bool {
	return e.Object.GetNamespace() == o.namespace && e.Object.GetName() == o.name
}

// enqueueConst always enqueues the same request regardless of the event.
type enqueueConst reconcile.Request

func (e enqueueConst) Create(_ event.CreateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Update(_ event.UpdateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Delete(_ event.DeleteEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Generic(_ event.GenericEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}
