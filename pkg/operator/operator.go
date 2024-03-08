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

// Package operator contains the Prometheus operator.
package operator

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/prometheus"
	arv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	// DefaultOperatorNamespace is the namespace in which all resources owned by the operator are installed.
	DefaultOperatorNamespace = "gmp-system"
	// DefaultPublicNamespace is the namespace where the operator will check for user-specified
	// configuration data.
	DefaultPublicNamespace = "gmp-public"

	// NameOperator is a fixed name used in various resources managed by the operator.
	NameOperator = "gmp-operator"
	// componentName is a fixed name used in various resources managed by the operator.
	componentName = "managed_prometheus"

	// Filename for configuration files.
	configFilename = "config.yaml"

	// LabelAppName is the well-known app name label.
	LabelAppName = "app.kubernetes.io/name"
	// LabelInstanceName is the well-known instance name label.
	LabelInstanceName = "app.kubernetes.io/instance"

	// AnnotationMetricName is the component name, will be exposed as metric name.
	AnnotationMetricName = "components.gke.io/component-name"
	// ClusterAutoscalerSafeEvictionLabel is the annotation label that determines
	// whether the cluster autoscaler can safely evict a Pod when the Pod doesn't
	// satisfy certain eviction criteria.
	ClusterAutoscalerSafeEvictionLabel = "cluster-autoscaler.kubernetes.io/safe-to-evict"

	// KubernetesAppName is the k8s Application, will be exposed as component name.
	KubernetesAppName = "app"
	// RuleEvaluatorAppName is the name of the rule-evaluator application.
	RuleEvaluatorAppName = "managed-prometheus-rule-evaluator"
	// AlertmanagerAppName is the name of the alert manager application.
	AlertmanagerAppName = "managed-prometheus-alertmanager"

	// The level of concurrency to use to fetch all targets.
	defaultTargetPollConcurrency = 4
)

// Operator to implement managed collection for Google Prometheus Engine.
type Operator struct {
	logger  logr.Logger
	opts    Options
	client  client.Client
	manager manager.Manager
}

// Options for the Operator.
type Options struct {
	// ID of the project of the cluster.
	ProjectID string
	// Location of the cluster.
	Location string
	// Name of the cluster the operator acts on.
	Cluster string
	// Namespace to which the operator deploys any associated resources.
	OperatorNamespace string
	// Namespace to which the operator looks for user-specified configuration
	// data, like Secrets and ConfigMaps.
	PublicNamespace string
	// Certificate of the server in base 64.
	TLSCert string
	// Key of the server in base 64.
	TLSKey string
	// Certificate authority in base 64.
	CACert string
	// Webhook serving address.
	ListenAddr string
	// Cleanup resources without this annotation.
	CleanupAnnotKey string
	// The number of upper bound threads to use for target polling otherwise
	// use the default.
	TargetPollConcurrency uint16
	// The HTTP client to use when targeting collector endpoints.
	CollectorHTTPClient *http.Client
}

func (o *Options) defaultAndValidate(_ logr.Logger) error {
	if o.OperatorNamespace == "" {
		o.OperatorNamespace = DefaultOperatorNamespace
	}
	if o.PublicNamespace == "" {
		// For non-managed deployments, default to same namespace
		// as operator, assuming cluster operators prefer consolidating
		// resources in a single namespace.
		o.PublicNamespace = DefaultOperatorNamespace
	}

	// ProjectID and Cluster must be always be set. Collectors and rule-evaluator can
	// auto-discover them but we need them in the operator to scope generated rules.
	if o.ProjectID == "" {
		return errors.New("projectID must be set")
	}
	if o.Cluster == "" {
		return errors.New("cluster must be set")
	}

	if o.TargetPollConcurrency == 0 {
		o.TargetPollConcurrency = defaultTargetPollConcurrency
	}
	if o.CollectorHTTPClient == nil {
		// Matches the default Prometheus API library HTTP client.
		o.CollectorHTTPClient = &http.Client{
			Transport: api.DefaultRoundTripper,
		}
	}
	return nil
}

// NewScheme creates a new Kubernetes runtime.Scheme for the GMP Operator.
func NewScheme() (*runtime.Scheme, error) {
	sc := runtime.NewScheme()

	if err := scheme.AddToScheme(sc); err != nil {
		return nil, fmt.Errorf("add Kubernetes core scheme: %w", err)
	}
	if err := monitoringv1.AddToScheme(sc); err != nil {
		return nil, fmt.Errorf("add monitoringv1 scheme: %w", err)
	}
	return sc, nil
}

// New instantiates a new Operator.
func New(logger logr.Logger, clientConfig *rest.Config, opts Options) (*Operator, error) {
	if err := opts.defaultAndValidate(logger); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	// Create temporary directory to store webhook serving cert files.
	certDir, err := os.MkdirTemp("", "operator-cert")
	if err != nil {
		return nil, fmt.Errorf("create temporary certificate dir: %w", err)
	}

	sc, err := NewScheme()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize Kubernetes scheme: %w", err)
	}

	host, portStr, err := net.SplitHostPort(opts.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}
	manager, err := ctrl.NewManager(clientConfig, manager.Options{
		Scheme: sc,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    host,
			Port:    port,
			CertDir: certDir,
		}),
		// Don't run a metrics server with the manager. Metrics are being served.
		// explicitly in the main routine.
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		// Manage cluster-wide and namespace resources at the same time.
		NewCache: cache.NewCacheFunc(func(_ *rest.Config, options cache.Options) (cache.Cache, error) {
			return cache.New(clientConfig, cache.Options{
				Scheme: options.Scheme,

				// The presence of metadata.namespace has special handling internally causing the
				// cache's watch-list to only watch that namespace.
				ByObject: map[client.Object]cache.ByObject{
					&corev1.Pod{}: {
						Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": opts.OperatorNamespace}),
					},
					&monitoringv1.PodMonitoring{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.ClusterPodMonitoring{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.ClusterNodeMonitoring{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.GlobalRules{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.ClusterRules{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.Rules{}: {
						Field: fields.Everything(),
					},
					&corev1.Secret{}: {
						Namespaces: map[string]cache.Config{
							opts.OperatorNamespace: {},
							opts.PublicNamespace:   {},
						},
					},
					&monitoringv1.OperatorConfig{}: {
						Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": opts.PublicNamespace}),
					},
					&corev1.Service{}: {
						Field: fields.SelectorFromSet(fields.Set{
							"metadata.namespace": opts.OperatorNamespace,
							"metadata.name":      NameAlertmanager,
						}),
					},
					&corev1.ConfigMap{}: {
						Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": opts.OperatorNamespace}),
					},
					&appsv1.DaemonSet{}: {
						Field: fields.SelectorFromSet(fields.Set{
							"metadata.namespace": opts.OperatorNamespace,
							"metadata.name":      NameCollector,
						}),
					},
					&appsv1.Deployment{}: {
						Field: fields.SelectorFromSet(fields.Set{
							"metadata.namespace": opts.OperatorNamespace,
							"metadata.name":      NameRuleEvaluator,
						}),
					},
				}})
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create controller manager: %w", err)
	}

	client, err := client.New(clientConfig, client.Options{Scheme: sc})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	op := &Operator{
		logger:  logger,
		opts:    opts,
		client:  client,
		manager: manager,
	}
	return op, nil
}

// setupAdmissionWebhooks configures validating webhooks for the operator-managed
// custom resources and registers handlers with the webhook server.
func (o *Operator) setupAdmissionWebhooks(ctx context.Context) error {
	// Write provided cert files.
	caBundle, err := o.ensureCerts(o.manager.GetWebhookServer().(*webhook.DefaultServer).Options.CertDir)
	if err != nil {
		return err
	}

	if len(caBundle) > 0 {
		// Keep setting the caBundle, if "ensureCerts" gives us those, in the expected webhook configurations.
		// In case of not enough permissions we will keep trying with error message.
		go o.continuouslySetCABundle(ctx, caBundle)
	}

	s := o.manager.GetWebhookServer()

	// Validating webhooks.
	s.Register(
		validatePath(monitoringv1.PodMonitoringResource()),
		admission.ValidatingWebhookFor(o.manager.GetScheme(), &monitoringv1.PodMonitoring{}),
	)
	s.Register(
		validatePath(monitoringv1.ClusterPodMonitoringResource()),
		admission.ValidatingWebhookFor(o.manager.GetScheme(), &monitoringv1.ClusterPodMonitoring{}),
	)
	s.Register(
		validatePath(monitoringv1.ClusterNodeMonitoringResource()),
		admission.ValidatingWebhookFor(o.manager.GetScheme(), &monitoringv1.ClusterNodeMonitoring{}),
	)
	s.Register(
		validatePath(monitoringv1.OperatorConfigResource()),
		admission.WithCustomValidator(o.manager.GetScheme(), &monitoringv1.OperatorConfig{}, &operatorConfigValidator{
			namespace: o.opts.PublicNamespace,
		}),
	)
	s.Register(
		validatePath(monitoringv1.RulesResource()),
		admission.WithCustomValidator(o.manager.GetScheme(), &monitoringv1.Rules{}, &rulesValidator{
			opts: o.opts,
		}),
	)
	s.Register(
		validatePath(monitoringv1.ClusterRulesResource()),
		admission.WithCustomValidator(o.manager.GetScheme(), &monitoringv1.ClusterRules{}, &clusterRulesValidator{
			opts: o.opts,
		}),
	)
	s.Register(
		validatePath(monitoringv1.GlobalRulesResource()),
		admission.WithCustomValidator(o.manager.GetScheme(), &monitoringv1.GlobalRules{}, &globalRulesValidator{}),
	)
	// Defaulting webhooks.
	s.Register(
		defaultPath(monitoringv1.PodMonitoringResource()),
		admission.WithCustomDefaulter(o.manager.GetScheme(), &monitoringv1.PodMonitoring{}, &podMonitoringDefaulter{}),
	)
	s.Register(
		defaultPath(monitoringv1.ClusterPodMonitoringResource()),
		admission.WithCustomDefaulter(o.manager.GetScheme(), &monitoringv1.ClusterPodMonitoring{}, &clusterPodMonitoringDefaulter{}),
	)
	return nil
}

// Run the reconciliation loop of the operator.
// The passed owner references are set on cluster-wide resources created by the
// operator.
func (o *Operator) Run(ctx context.Context, registry prometheus.Registerer) error {
	defer runtimeutil.HandleCrash()

	if err := o.cleanupOldResources(ctx); err != nil {
		return fmt.Errorf("cleanup old resources: %w", err)
	}
	if err := o.setupAdmissionWebhooks(ctx); err != nil {
		return fmt.Errorf("init admission resources: %w", err)
	}
	if err := setupCollectionControllers(o); err != nil {
		return fmt.Errorf("setup collection controllers: %w", err)
	}
	if err := setupRulesControllers(o); err != nil {
		return fmt.Errorf("setup rules controllers: %w", err)
	}
	if err := setupOperatorConfigControllers(o); err != nil {
		return fmt.Errorf("setup rule-evaluator controllers: %w", err)
	}
	if err := setupTargetStatusPoller(o, registry, o.opts.CollectorHTTPClient); err != nil {
		return fmt.Errorf("setup target status processor: %w", err)
	}

	o.logger.Info("starting GMP operator")
	return o.manager.Start(ctx)
}

func (o *Operator) cleanupOldResources(ctx context.Context) error {
	// Delete old ValidatingWebhookConfiguration that was installed directly by the operator.
	// in previous versions.
	validatingWebhookConfig := arv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "gmp-operator"},
	}
	if err := o.client.Delete(ctx, &validatingWebhookConfig); err != nil {
		switch {
		case apierrors.IsForbidden(err):
			o.logger.Info("delete legacy ValidatingWebHookConfiguration was not allowed. Please remove it manually")
		case !apierrors.IsNotFound(err):
			return fmt.Errorf("delete legacy ValidatingWebHookConfiguration failed: %w", err)
		}
	}

	// If cleanup annotations are not provided, do not clean up any further.
	if o.opts.CleanupAnnotKey == "" {
		return nil
	}

	// Cleanup resources without the provided annotation.
	// Check the collector DaemonSet.
	dsKey := client.ObjectKey{
		Name:      NameCollector,
		Namespace: o.opts.OperatorNamespace,
	}
	var ds appsv1.DaemonSet
	if err := o.client.Get(ctx, dsKey, &ds); apierrors.IsNotFound(err) {
		return fmt.Errorf("get collector DaemonSet: %w", err)
	}
	if _, ok := ds.Annotations[o.opts.CleanupAnnotKey]; !ok {
		if err := o.client.Delete(ctx, &ds); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete collector DaemonSet: %w", err)
		}
	}

	// Check the rule-evaluator Deployment.
	deployKey := client.ObjectKey{
		Name:      NameRuleEvaluator,
		Namespace: o.opts.OperatorNamespace,
	}
	var deploy appsv1.Deployment
	if err := o.client.Get(ctx, deployKey, &deploy); apierrors.IsNotFound(err) {
		return fmt.Errorf("get rule-evaluator Deployment: %w", err)
	}
	if _, ok := deploy.Annotations[o.opts.CleanupAnnotKey]; !ok {
		if err := o.client.Delete(ctx, &deploy); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete rule-evaluator Deployment: %w", err)
		}
	}

	return nil
}

// ensureCerts writes the cert/key files to the specified directory.
// If cert/key are not available, generate them.
func (o *Operator) ensureCerts(dir string) ([]byte, error) {
	var (
		crt, key, caData []byte
		err              error
	)
	if o.opts.TLSKey != "" && o.opts.TLSCert != "" {
		crt, err = base64.StdEncoding.DecodeString(o.opts.TLSCert)
		if err != nil {
			return nil, fmt.Errorf("decoding TLS certificate: %w", err)
		}
		key, err = base64.StdEncoding.DecodeString(o.opts.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("decoding TLS key: %w", err)
		}
		if o.opts.CACert != "" {
			caData, err = base64.StdEncoding.DecodeString(o.opts.CACert)
			if err != nil {
				return nil, fmt.Errorf("decoding certificate authority: %w", err)
			}
		}
	} else if o.opts.TLSKey == "" && o.opts.TLSCert == "" && o.opts.CACert == "" {
		// Generate a self-signed pair if none was explicitly provided. It will be valid
		// for 1 year.
		// TODO(freinartz): re-generate at runtime and update the ValidatingWebhookConfiguration
		// at runtime whenever the files change.
		fqdn := fmt.Sprintf("%s.%s.svc", NameOperator, o.opts.OperatorNamespace)

		crt, key, err = cert.GenerateSelfSignedCertKey(fqdn, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("generate self-signed TLS key pair: %w", err)
		}
		// Use crt as the ca in the self-sign case.
		caData = crt
	} else {
		return nil, errors.New("flags key-base64 and cert-base64 must both be set")
	}
	// Create cert/key files.
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), crt, 0666); err != nil {
		return nil, fmt.Errorf("create cert file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), key, 0666); err != nil {
		return nil, fmt.Errorf("create key file: %w", err)
	}
	return caData, nil
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

func (e enqueueConst) Create(_ context.Context, _ event.CreateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Update(_ context.Context, _ event.UpdateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Delete(_ context.Context, _ event.DeleteEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func (e enqueueConst) Generic(_ context.Context, _ event.GenericEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request(e))
}

func validatePath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/validate/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func defaultPath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/default/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func (o *Operator) webhookConfigName() string {
	return fmt.Sprintf("%s.%s.monitoring.googleapis.com", NameOperator, o.opts.OperatorNamespace)
}

func (o *Operator) setValidatingWebhookCABundle(ctx context.Context, caBundle []byte) error {
	var vwc arv1.ValidatingWebhookConfiguration
	err := o.client.Get(ctx, client.ObjectKey{Name: o.webhookConfigName()}, &vwc)
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	return o.client.Update(ctx, &vwc)
}

func (o *Operator) setMutatingWebhookCABundle(ctx context.Context, caBundle []byte) error {
	var mwc arv1.MutatingWebhookConfiguration
	err := o.client.Get(ctx, client.ObjectKey{Name: o.webhookConfigName()}, &mwc)
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	for i := range mwc.Webhooks {
		mwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	return o.client.Update(ctx, &mwc)
}

func (o *Operator) continuouslySetCABundle(ctx context.Context, caBundle []byte) {
	// Initial sleep for the client to initialize before our first calls.
	// Ideally we could explicitly wait for it.
	time.Sleep(5 * time.Second)

	for {
		if err := o.setValidatingWebhookCABundle(ctx, caBundle); err != nil {
			o.logger.Error(err, "Setting CA bundle for ValidatingWebhookConfiguration failed; retrying in 1m...")
		}
		if err := o.setMutatingWebhookCABundle(ctx, caBundle); err != nil {
			o.logger.Error(err, "Setting CA bundle for MutatingWebhookConfiguration failed; retrying in 1m...")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}
