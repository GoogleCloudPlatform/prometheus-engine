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
	"net"
	"path/filepath"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

const (
	// DefaultOperatorNamespace is the namespace in which all resources owned by the operator are installed.
	DefaultOperatorNamespace = "gmp-system"
	// DefaultPublicNamespace is the namespace where the operator will check for user-specified
	// configuration data.
	DefaultPublicNamespace = "gmp-public"

	// Fixed names used in various resources managed by the operator.
	NameOperator  = "gmp-operator"
	componentName = "managed_prometheus"

	// Prometheus configuration file and volume mounts.
	// Used in both collectors and rule-evaluator.
	configOutDir        = "/prometheus/config_out"
	configVolumeName    = "config"
	configDir           = "/prometheus/config"
	configOutVolumeName = "config-out"
	configFilename      = "config.yaml"

	// The well-known app name label.
	LabelAppName = "app.kubernetes.io/name"
	// The component name, will be exposed as metric name.
	AnnotationMetricName = "components.gke.io/component-name"

	// The official images to be used with this version of the operator. For debugging
	// and emergency use cases they may be overwritten through options.
	ImageCollector      = "gke.gcr.io/prometheus-engine/prometheus:v2.28.1-gmp.1-gke.1"
	ImageConfigReloader = "gke.gcr.io/prometheus-engine/config-reloader:v0.1.1-gke.0"
	ImageRuleEvaluator  = "gke.gcr.io/prometheus-engine/rule-evaluator:v0.1.1-gke.0"

	// The k8s Application, will be exposed as component name.
	KubernetesAppName    = "app"
	CollectorAppName     = "managed-prometheus-collector"
	RuleEvaluatorAppName = "managed-prometheus-rule-evaluator"
)

var (
	metricOperatorSyncLatency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "operator_sync_latency",
			Namespace: "gmp_operator",
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
}

// Options for the Operator.
type Options struct {
	// ID of the project of the cluster.
	ProjectID string
	// Location of the cluster.
	Location string
	// Name of the cluster the operator acts on.
	Cluster string
	// Disable exporting to GCM (mostly for testing).
	DisableExport bool
	// Namespace to which the operator deploys any associated resources.
	OperatorNamespace string
	// Namespace to which the operator looks for user-specified configuration
	// data, like Secrets and ConfigMaps.
	PublicNamespace string
	// Listening port of the collector. Configurable to allow multiple
	// simultanious collector deployments for testing purposes while each
	// collector runs on the host network.
	CollectorPort int32
	// Image for the Prometheus collector container.
	ImageCollector string
	// Image for the Prometheus config reloader.
	ImageConfigReloader string
	// Image for the Prometheus rule-evaluator.
	ImageRuleEvaluator string
	// Whether to deploy pods with hostNetwork enabled. This allow pods to run with the GCE compute
	// default service account even on GKE clusters with Workload Identity enabled.
	// It must be set to false for GKE Autopilot clusters.
	HostNetwork bool
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
	if o.PublicNamespace == "" {
		// For non-managed deployments, default to same namespace
		// as operator, assuming cluster operators prefer consolidating
		// resources in a single namespace.
		o.PublicNamespace = DefaultOperatorNamespace
	}
	if o.CollectorPort == 0 {
		o.CollectorPort = 19090
	}
	if o.ImageCollector == "" {
		o.ImageCollector = ImageCollector
	}
	if o.ImageConfigReloader == "" {
		o.ImageConfigReloader = ImageConfigReloader
	}
	if o.ImageRuleEvaluator == "" {
		o.ImageRuleEvaluator = ImageRuleEvaluator
	}

	// ProjectID and Cluster must be always be set. Collectors and rule-evaluator can
	// auto-discover them but we need them in the operator to scope generated rules.
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
	if o.ImageRuleEvaluator != ImageRuleEvaluator {
		logger.Info("not using the canonical rule-evaluator image",
			"expected", ImageRuleEvaluator, "got", o.ImageRuleEvaluator)
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
	certDir, err := ioutil.TempDir("", "operator-cert")
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
	host, portStr, err := net.SplitHostPort(opts.ListenAddr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid listen address")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid port")
	}
	mgr, err := ctrl.NewManager(clientConfig, manager.Options{
		Scheme: sc,
		Host:   host,
		Port:   port,
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
	}
	return op, nil
}

// setupAdmissionWebhooks configures validating webhooks for the operator-managed
// custom resources and registers handlers with the webhook server.
// The passsed owner references are set on the created WebhookConfiguration resources.
func (o *Operator) setupAdmissionWebhooks(ctx context.Context, ors ...metav1.OwnerReference) error {
	// Persisting TLS keypair to a k8s secret seems like unnecessary state to manage.
	// It's fairly trivial to re-generate the cert and private
	// key on each startup. Also no other GMP resources aside from the operator
	// rely on the keypair.
	// A downside to this approach is re-writing the validation webhook config
	// every time with the new caBundle. This should only happen when the operator
	// restarts, which should be infrequent.
	var (
		crt, key []byte
		err      error
		fqdn     = fmt.Sprintf("system:node:%s.%s.svc", NameOperator, o.opts.OperatorNamespace)
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

	if err := ioutil.WriteFile(filepath.Join(o.manager.GetWebhookServer().CertDir, "tls.crt"), crt, 0666); err != nil {
		return errors.Wrap(err, "create cert file")
	}
	if err := ioutil.WriteFile(filepath.Join(o.manager.GetWebhookServer().CertDir, "tls.key"), key, 0666); err != nil {
		return errors.Wrap(err, "create key file")
	}

	whCfg := validatingWebhookConfig(
		NameOperator,
		o.opts.OperatorNamespace,
		int32(o.manager.GetWebhookServer().Port),
		crt,
		[]metav1.GroupVersionResource{
			monitoringv1alpha1.PodMonitoringResource(),
		},
		ors...,
	)
	// Idempotently request validation webhook spec with caBundle and endpoints.
	_, err = upsertValidatingWebhookConfig(ctx, o.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations(), whCfg)
	if err != nil {
		return err
	}

	s := o.manager.GetWebhookServer()
	s.Register(
		validatePath(monitoringv1alpha1.PodMonitoringResource()),
		admission.ValidatingWebhookFor(&monitoringv1alpha1.PodMonitoring{}),
	)
	return nil
}

// Run the reconciliation loop of the operator.
// The passed owner references are set on cluster-wide resources created by the
// operator.
func (o *Operator) Run(ctx context.Context, ors ...metav1.OwnerReference) error {
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
	if err := setupOperatorConfigControllers(o); err != nil {
		return errors.Wrap(err, "setup rule-evaluator controllers")
	}

	o.logger.Info("starting GMP operator")

	return o.manager.Start(ctx)
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
