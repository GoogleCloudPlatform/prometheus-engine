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
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	promconfig "github.com/prometheus/prometheus/config"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
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
	logger     logr.Logger
	opts       Options
	kubeClient kubernetes.Interface
	manager    manager.Manager
	certDir    string
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

func (o *Options) defaultAndValidate(logger logr.Logger) error {
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
		logger.Info("not using the canonical collector image",
			"expected", ImageCollector, "got", o.ImageCollector)
	}
	if o.ImageConfigReloader != ImageConfigReloader {
		logger.Info("not using the canonical config reloader image",
			"expected", ImageConfigReloader, "got", o.ImageConfigReloader)
	}
	return nil
}

// New instantiates a new Operator.
func New(logger logr.Logger, clientConfig *rest.Config, registry prometheus.Registerer, opts Options) (*Operator, error) {
	if err := opts.defaultAndValidate(logger); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build Kubernetes clientset")
	}
	// Create temporary directory to store webhook serving cert files.
	certDir, err := ioutil.TempDir("", "prometheus-engine-operator-certs")
	if err != nil {
		return nil, errors.Wrap(err, "create temporary certificate dir")
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
		CertDir:            certDir,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create controller manager")
	}

	if registry != nil {
		registry.MustRegister(metricOperatorSyncLatency)
	}

	op := &Operator{
		logger:     logger,
		opts:       opts,
		kubeClient: kubeClient,
		manager:    mgr,
		certDir:    certDir,
	}
	return op, nil
}

// setupAdmissionWebhooks configures validating webhooks for the operator-managed
// custom resources and registers handlers with the webhook server.
// The passsed owner references are set on the created WebhookConfiguration resources.
func (o *Operator) setupAdmissionWebhooks(ctx context.Context, ors ...metav1.OwnerReference) error {
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
		fqdn     = fmt.Sprintf("%s.%s.svc", NameOperator, o.opts.OperatorNamespace)
	)
	// Generate cert/key pair - self-signed CA or kube-apiserver CA.
	if o.opts.CASelfSign {
		crt, key, err = cert.GenerateSelfSignedCertKey(fqdn, nil, nil)
		if err != nil {
			return err
		}
	} else {
		crt, key, err = CreateSignedKeyPair(ctx, o.kubeClient, fqdn)
		if err != nil {
			return err
		}
	}

	if err := ioutil.WriteFile(filepath.Join(o.certDir, "tls.crt"), crt, 0666); err != nil {
		return errors.Wrap(err, "create cert file")
	}
	if err := ioutil.WriteFile(filepath.Join(o.certDir, "tls.key"), key, 0666); err != nil {
		return errors.Wrap(err, "create key file")
	}

	const podEp = "/podmonitorings/v1alpha1/validate"

	// Idempotently request validation webhook spec with caBundle and endpoints.
	_, err = UpsertValidatingWebhookConfig(ctx,
		o.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
		ValidatingWebhookConfig(NameOperator, o.opts.OperatorNamespace, crt, []string{podEp}, ors...))
	if err != nil {
		return err
	}

	s := o.manager.GetWebhookServer()
	s.Register(podEp, admission.ValidatingWebhookFor(&monitoringv1alpha1.PodMonitoring{}))

	return nil
}

// Run the reconciliation loop of the operator.
// The passed owner references are set on cluster-wide resources created by the
// operator.
func (o *Operator) Run(ctx context.Context,  ors ...metav1.OwnerReference) error {
	defer runtimeutil.HandleCrash()

	if err := o.setupAdmissionWebhooks(ctx, ors...); err != nil {
		return errors.Wrap(err, "init admission resources")
	}
	if err := setupCollectionControllers(o); err != nil {
		return errors.Wrap(err, "setup collection controllers")
	}
	if err := setupRulesControllers(o); err != nil {
		return errors.Wrap(err, "setup rules controllers")
	}

	o.logger.Info("starting GPE operator")

	return o.manager.Start(ctx)
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

func (r *rulesReconciler) ensureRuleConfigs(ctx context.Context) error {
	logger := logr.FromContext(ctx)

	// Re-generate the configmap that's loaded by the rule-evaluator.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      nameRulesGenerated,
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
	var rulesList monitoringv1alpha1.RulesList
	if err := r.client.List(ctx, &rulesList); err != nil {
		return errors.Wrap(err, "list rules")
	}
	for _, apiRules := range rulesList.Items {
		rulesLogger := logger.WithValues("rules_namespace", apiRules.Namespace, "rules_name", apiRules.Name)

		rs, err := rules.FromAPIRules(apiRules.Spec.Groups)
		if err != nil {
			rulesLogger.Error(err, "converting rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		lset := map[string]string{}
		// Populate isolation level from the defined scope.
		switch apiRules.Spec.Scope {
		case monitoringv1alpha1.ScopeCluster:
			lset[export.KeyProjectID] = r.opts.ProjectID
			lset[export.KeyCluster] = r.opts.Cluster
		case monitoringv1alpha1.ScopeNamespace:
			lset[export.KeyProjectID] = r.opts.ProjectID
			lset[export.KeyCluster] = r.opts.Cluster
			lset[export.KeyNamespace] = apiRules.Namespace
		default:
			rulesLogger.Error(errors.New("scope type is not defiend"), "unexpected scope", "scope", apiRules.Spec.Scope)
			// TODO(freinartz): update resource condition.
			continue
		}
		if err := rules.Scope(&rs, lset); err != nil {
			rulesLogger.Error(err, "isolating rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		result, err := yaml.Marshal(rs)
		if err != nil {
			rulesLogger.Error(err, "marshalling rules failed")
			// TODO(freinartz): update resource condition.
			continue
		}
		filename := fmt.Sprintf("%s__%s.yaml", apiRules.Namespace, apiRules.Name)
		cm.Data[filename] = string(result)
	}

	// Create or update generated rule ConfigMap.
	if err := r.client.Update(ctx, cm); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cm); err != nil {
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
