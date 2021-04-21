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
	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/pkg/relabel"
	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"

	"github.com/google/gpe-collector/pkg/export"
	monitoringv1alpha1 "github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"
	clientset "github.com/google/gpe-collector/pkg/operator/generated/clientset/versioned"
	informers "github.com/google/gpe-collector/pkg/operator/generated/informers/externalversions"
)

// DefaultNamespace is the namespace in which all resources owned by the operator are installed.
const (
	DefaultName      = "gpe-operator"
	DefaultNamespace = "gpe-system"
)

// The official images to be used with this version of the operator. For debugging
// and emergency use cases they may be overwritten through options.
// TODO(freinartz): start setting official versioned images once we start releases.
const (
	ImageCollector      = "gcr.io/gpe-test-1/prometheus:test-14"
	ImageConfigReloader = "gcr.io/gpe-test-1/config-reloader:0.0.9"
)

// Kubernetes resource label mapping values.
const (
	kubeLabelPrefix    = model.MetaLabelPrefix + "kubernetes_"
	podLabelPrefix     = kubeLabelPrefix + "pod_label_"
	serviceLabelPrefix = kubeLabelPrefix + "service_label_"
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

	// Informers that maintain a cache of cluster resources and call configured
	// event handlers on changes.
	informerPodMonitoring cache.SharedIndexInformer
	informerConfigMap     cache.SharedIndexInformer
	// State changes are enqueued into a rate limited work queue, which ensures
	// the operator does not get overloaded and multiple changes to the same resource
	// are not handled in parallel, leading to semantic race conditions.
	queue workqueue.RateLimitingInterface

	// Internal bookkeeping for sending status updates to processed CRDs.
	statusState *CRDStatusState
}

// Options for the Operator.
type Options struct {
	// Namespace to which the operator deploys any associated resources.
	Namespace string
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
	if o.Namespace == "" {
		o.Namespace = DefaultNamespace
	}
	if o.ImageCollector == "" {
		o.ImageCollector = ImageCollector
	}
	if o.ImageConfigReloader == "" {
		o.ImageConfigReloader = ImageConfigReloader
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
		logger:                logger,
		opts:                  opts,
		kubeClient:            kubeClient,
		operatorClient:        operatorClient,
		informerPodMonitoring: informerFactory.Monitoring().V1alpha1().PodMonitorings().Informer(),
		informerConfigMap:     corev1informers.NewConfigMapInformer(kubeClient, opts.Namespace, syncInterval, nil),
		queue:                 workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "GPEOperator"),
		statusState:           NewCRDStatusState(metav1.Now),
	}

	// We only trigger global reconciliation as the operator currently does not handle CRDs
	// that only affect a subset of resources managed by the operator.
	op.informerPodMonitoring.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.enqueueGlobal,
		DeleteFunc: op.enqueueGlobal,
		UpdateFunc: ifResourceVersionChanged(op.enqueueGlobal),
	})
	op.informerConfigMap.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.enqueueGlobal,
		DeleteFunc: op.enqueueGlobal,
		UpdateFunc: ifResourceVersionChanged(op.enqueueGlobal),
	})

	// TODO(freinartz): setup additional informers or at least periodic calls to sync()
	// to ensure that changes made by users or other components are immediately reverted.

	return op, nil
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

// A key used for triggering reconciliation of global/cluster-wide resources.
const keyGlobal = "__global__"

// enqueueGlobal enqueues the global reconcilation key. It takes an unused
// argument to avoid boilerplate when registering event handlers.
func (o *Operator) enqueueGlobal(interface{}) {
	o.queue.Add(keyGlobal)
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
		fqdn     = fmt.Sprintf("%s.%s.svc", DefaultName, o.opts.Namespace)
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
		ValidatingWebhookConfig(DefaultName, o.opts.Namespace, crt, []string{podEp}, ors...))
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

	level.Info(o.logger).Log("msg", "starting GPE operator")

	go o.informerPodMonitoring.Run(ctx.Done())
	go o.informerConfigMap.Run(ctx.Done())

	level.Info(o.logger).Log("msg", "waiting for informer caches to sync")

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	ok := cache.WaitForNamedCacheSync(
		"GPEOperator", syncCtx.Done(),
		o.informerPodMonitoring.HasSynced,
		o.informerConfigMap.HasSynced,
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
	o.enqueueGlobal(nil)

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

	if key != keyGlobal {
		return errors.Errorf("expected global reconciliation but got key %q", key)
	}

	level.Info(o.logger).Log("msg", "syncing cluster state for key", "key", key)

	// Reset parsed CRDs.
	o.statusState.Reset()

	if err := o.ensureCollectorConfig(ctx); err != nil {
		return errors.Wrap(err, "ensure collector config")
	}
	if err := o.ensureCollectorDaemonSet(ctx); err != nil {
		return errors.Wrap(err, "ensure collector daemon set")
	}
	if err := o.updateCRDStatus(ctx); err != nil {
		return errors.Wrap(err, "update crd status")
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
func (o *Operator) ensureCollectorDaemonSet(ctx context.Context) error {
	ds := o.makeCollectorDaemonSet()

	_, err := o.kubeClient.AppsV1().DaemonSets(ds.Namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		_, err = o.kubeClient.AppsV1().DaemonSets(ds.Namespace).Create(ctx, ds, metav1.CreateOptions{})
		if err != nil {
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
		"--web.listen-address=:9090",
		"--web.enable-lifecycle",
		"--web.route-prefix=/",
	}
	if o.opts.CloudMonitoringEndpoint != "" {
		collectorArgs = append(collectorArgs, fmt.Sprintf("--gcm.endpoint=%s", o.opts.CloudMonitoringEndpoint))
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
							{Name: "prometheus-http", ContainerPort: 9090},
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
							"--reload-url=http://localhost:9090/-/reload",
							"--listen-address=:9091",
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
							{Name: "reloader-http", ContainerPort: 9091},
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
			},
		},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.opts.Namespace,
			Name:      CollectorName,
		},
		Spec: spec,
	}
	return ds
}

// updateCRDStatus iterates through parsed CRDs and updates their statuses.
// If an error is encountered from performing an update, the function returns
// the error immediately and does not attempt updates on subsequent CRDs.
func (o *Operator) updateCRDStatus(ctx context.Context) error {
	for _, pm := range o.statusState.PodMonitorings() {
		_, err := o.operatorClient.MonitoringV1alpha1().PodMonitorings(pm.Namespace).UpdateStatus(ctx, &pm, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (o *Operator) ensureCollectorConfig(ctx context.Context) error {
	cfg, err := o.makeCollectorConfig()
	if err != nil {
		return errors.Wrap(err, "generate Prometheus config")
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, "marshal Prometheus config")
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: CollectorName,
		},
		Data: map[string]string{
			collectorConfigFilename: string(cfgEncoded),
		},
	}
	_, err = o.kubeClient.CoreV1().ConfigMaps(o.opts.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		_, err = o.kubeClient.CoreV1().ConfigMaps(o.opts.Namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "create Prometheus config")
		}
	} else if err != nil {
		return errors.Wrap(err, "update Prometheus config")
	}
	return nil
}

func (o *Operator) makeCollectorConfig() (*promconfig.Config, error) {
	var scrapeCfgs []*promconfig.ScrapeConfig
	// Generate a separate scrape job for every endpoint in every PodMonitoring.
	var podmons []*monitoringv1alpha1.PodMonitoring
	err := cache.ListAll(o.informerPodMonitoring.GetStore(), labels.Everything(), func(obj interface{}) {
		podmons = append(podmons, obj.(*monitoringv1alpha1.PodMonitoring))
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list PodMonitorings")
	}

	// Mark status updates in batch with single timestamp.
	for _, podmon := range podmons {
		cond := &monitoringv1alpha1.MonitoringCondition{
			Type:   monitoringv1alpha1.ConfigurationCreateSuccess,
			Status: corev1.ConditionTrue,
		}
		for i := range podmon.Spec.Endpoints {
			scrapeCfg, err := makePodScrapeConfig(podmon, i)
			if err != nil {
				cond.Status = corev1.ConditionFalse
				level.Warn(o.logger).Log("msg", "generating scrape config failed for PodMonitoring endpoint",
					"err", err, "namespace", podmon.Namespace, "name", podmon.Name, "endpoint", i)
				continue
			}
			scrapeCfgs = append(scrapeCfgs, scrapeCfg)
		}
		err := o.statusState.SetPodMonitoringCondition(podmon, cond)
		if err != nil {
			// Log an error but let operator continue to avoid getting stuck
			// on a potential bad resource.
			level.Error(o.logger).Log(
				"msg", "setting podmonitoring status state",
				"err", err)
		}
	}

	// Load additional, hard-coded scrape configs from configmaps in the oeprator's namespace.
	err = cache.ListAll(o.informerConfigMap.GetStore(), labels.SelectorFromSet(map[string]string{
		"type": "scrape-config",
	}), func(obj interface{}) {
		cm := obj.(*corev1.ConfigMap)
		const key = "config.yaml"

		var promcfg promconfig.Config
		if err := yaml.Unmarshal([]byte(cm.Data[key]), &promcfg); err != nil {
			level.Error(o.logger).Log("msg", "cannot parse scrape config, skipping ...",
				"err", err, "namespace", cm.Namespace, "name", cm.Name)
			return
		}
		for _, sc := range promcfg.ScrapeConfigs {
			// Make scrape config name unique and traceable.
			sc.JobName = fmt.Sprintf("ConfigMap/%s/%s/%s", o.opts.Namespace, cm.Name, sc.JobName)
			scrapeCfgs = append(scrapeCfgs, sc)
		}
	})
	if err != nil {
		return nil, errors.Wrap(err, "add hard-coded scrape configs")
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
			Role: discoverykube.RolePod,
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
	case export.KeyLocation:
		return true
	case export.KeyCluster:
		return true
	case export.KeyNamespace:
		return true
	case export.KeyJob:
		return true
	case export.KeyInstance:
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
