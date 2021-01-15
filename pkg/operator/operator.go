package operator

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/pkg/relabel"
	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	clientset "github.com/google/gpe-collector/pkg/operator/generated/clientset/versioned"
	informers "github.com/google/gpe-collector/pkg/operator/generated/informers/externalversions"
)

// A key used for triggering reconciliation of global/cluster-wide resources.
const keyGlobal = "__global__"

// Operator to implement managed collection for Google Prometheus Engine.
type Operator struct {
	logger     log.Logger
	namespace  string
	kubeClient kubernetes.Interface

	// Informers that maintain a cache of cluster resources and call configured
	// event handlers on changes.
	informerServiceMonitoring cache.SharedIndexInformer
	// State changes are enqueued into a rate limited work queue, which ensures
	// the operator does not get overloaded and multiple changes to the same resource
	// are not handled in parallel, leading to semantic race conditions.
	queue workqueue.RateLimitingInterface
}

// New instantiates a new Operator.
func New(logger log.Logger, clientConfig *rest.Config, namespace string) (*Operator, error) {
	if namespace == "" {
		return nil, errors.New("namespace must not be empty")
	}
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build Kubernetes clientset")
	}
	operatorClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build operator clientset")
	}
	informerFactory := informers.NewSharedInformerFactory(operatorClient, time.Minute)

	op := &Operator{
		logger:                    logger,
		namespace:                 namespace,
		kubeClient:                kubeClient,
		informerServiceMonitoring: informerFactory.Monitoring().V1alpha1().ServiceMonitorings().Informer(),
		queue:                     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "GPEOperator"),
	}

	// We only trigger global reconciliation as the operator currently does not handle CRDs
	// that only affect a subset of resources managed by the operator.
	op.informerServiceMonitoring.AddEventHandler(cache.ResourceEventHandlerFuncs{
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

// Run the reconciliation loop of the operator.
func (o *Operator) Run(ctx context.Context) error {
	defer runtimeutil.HandleCrash()

	level.Info(o.logger).Log("msg", "starting GPE operator")

	go o.informerServiceMonitoring.Run(ctx.Done())

	level.Info(o.logger).Log("msg", "waiting for informer caches to sync")

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	ok := cache.WaitForNamedCacheSync("GPEOperator", syncCtx.Done(), o.informerServiceMonitoring.HasSynced)
	cancel()
	if !ok {
		return errors.New("aborted while waiting for informer caches to sync")
	}

	// Process work items until context is canceled.
	go func() {
		<-ctx.Done()
		o.queue.ShutDown()
	}()

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
	if key != keyGlobal {
		return errors.Errorf("expected global reconciliation but got key %q", key)
	}

	level.Info(o.logger).Log("msg", "syncing cluster state for key", "key", key)

	if err := o.ensureCollectorConfig(ctx); err != nil {
		return errors.Wrap(err, "ensure collector config")
	}
	if err := o.ensureCollectorDaemonSet(ctx); err != nil {
		return errors.Wrap(err, "ensure collector daemon set")
	}
	return nil
}

// Various constants generating resources.
const (
	// DefaultNamespace is the namespace in which all resources owned by the operator are installed.
	DefaultNamespace = "gpe-system"

	// collectorName is the base name of the collector used across various resources. Must match with
	// the static resources installed during the operator's base setup.
	collectorName = "collector"

	collectorConfigFilename   = "config.yaml"
	collectorConfigVolumeName = "config"
	collectorConfigMountPath  = "/prometheus/config"
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
		"app": collectorName,
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
						Name: "prometheus",
						// TODO(freinartz): use image of our own collector, make configurable.
						Image: "quay.io/prometheus/prometheus:v2.22.1",
						Args: []string{
							"--config.file=" + filepath.Join(collectorConfigMountPath, collectorConfigFilename),
							"--storage.tsdb.path=/prometheus/data",
							"--storage.tsdb.retention.time=24h",
							"--web.enable-lifecycle",
							"--storage.tsdb.no-lockfile",
							"--web.route-prefix=/",
						},
						// TODO(freinartz): make use of this in the Prometheus configuration so each collector
						// can only consider pods from its own node.
						// Using the field_selector config options should help substantially reduce load on Prometheus
						// as well as the Kubernetes API server in large setups.
						// Using an environment in Prometheus configs will require custom logic, either in our fork
						// or through a sidecar.
						Env: []corev1.EnvVar{
							{
								Name: "NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							},
						},
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: 9090},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      collectorConfigVolumeName,
								MountPath: collectorConfigMountPath,
								ReadOnly:  true,
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
									Name: collectorName,
								},
							},
						},
					},
				},
				ServiceAccountName: collectorName,
			},
		},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.namespace,
			Name:      collectorName,
		},
		Spec: spec,
	}
	return ds
}

// ensureCollectorConfig generates the collector config and creates or updates it.
func (o *Operator) ensureCollectorConfig(ctx context.Context) error {
	cfg, err := yaml.Marshal(o.makeCollectorConfig())
	if err != nil {
		return errors.Wrap(err, "marshal Prometheus config")
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectorName,
		},
		Data: map[string]string{
			collectorConfigFilename: string(cfg),
		},
	}
	_, err = o.kubeClient.CoreV1().ConfigMaps(o.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		_, err = o.kubeClient.CoreV1().ConfigMaps(o.namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "create Prometheus config")
		}
	} else if err != nil {
		return errors.Wrap(err, "update Prometheus config")
	}
	return nil
}

func (o *Operator) makeCollectorConfig() *promconfig.Config {
	// Generate a config that attempts to scrape all containers with a port named http on /metrics.
	// This will at least make the collector scrape itself.
	// TODO(freinartz): generate these based on the ServiceMonitoring and PodMonitoring resources.
	return &promconfig.Config{
		ScrapeConfigs: []*promconfig.ScrapeConfig{
			{
				JobName:        "test",
				ScrapeInterval: prommodel.Duration(30 * time.Second),
				ServiceDiscoveryConfigs: discovery.Configs{
					&discoverykube.SDConfig{
						Role: discoverykube.RolePod,
					},
				},
				RelabelConfigs: []*relabel.Config{
					{
						Action:       relabel.Keep,
						SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_container_port_name"},
						Regex:        relabel.MustNewRegexp("http"),
					},
				},
			},
		},
	}
}
