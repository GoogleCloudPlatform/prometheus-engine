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
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
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

	// Filename for configuration files.
	configFilename = "config.yaml"

	// The well-known app name label.
	LabelAppName = "app.kubernetes.io/name"
	// The component name, will be exposed as metric name.
	AnnotationMetricName = "components.gke.io/component-name"
	// ClusterAutoscalerSafeEvictionLabel is the annotation label that determines
	// whether the cluster autoscaler can safely evict a Pod when the Pod doesn't
	// satisfy certain eviction criteria.
	ClusterAutoscalerSafeEvictionLabel = "cluster-autoscaler.kubernetes.io/safe-to-evict"

	// The k8s Application, will be exposed as component name.
	KubernetesAppName    = "app"
	RuleEvaluatorAppName = "managed-prometheus-rule-evaluator"
	AlertmanagerAppName  = "managed-prometheus-alertmanager"

	// The level of concurrency to use to fetch all targets.
	defaultTargetPollConcurrency = 4
)

// Operator to implement managed collection for Google Prometheus Engine.
type Operator struct {
	logger  logr.Logger
	opts    Options
	client  client.Client
	manager manager.Manager
	// Due to the RBAC, the manager can only handle a single namespace per
	// object at a time so this cache is used in cases where we want the same
	// resource from multiple namespaces (not to be confused with cluster-wide
	// resources).
	managedNamespacesCache cache.Cache
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
	// TLSKeyFile specifies the path to the client TLS key for the webhook server
	KeyFile string
	// TLSCertFile specifies the path to the client TLS cert for the webhook server
	CertFile string
	// ClientCAFile is the path to the CA used by webhook clients to establish trust with the webhook server
	ClientCAFile string
	// Webhook serving address.
	ListenAddr string
	// Cleanup resources without this annotation.
	CleanupAnnotKey string
	// The number of upper bound threads to use for target polling otherwise
	// use the default.
	TargetPollConcurrency uint16
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

	// ProjectID and Cluster must be always be set. Collectors and rule-evaluator can
	// auto-discover them but we need them in the operator to scope generated rules.
	if o.ProjectID == "" {
		return errors.New("ProjectID must be set")
	}
	if o.Cluster == "" {
		return errors.New("Cluster must be set")
	}

	if o.TargetPollConcurrency == 0 {
		o.TargetPollConcurrency = defaultTargetPollConcurrency
	}
	return nil
}

func getScheme() (*runtime.Scheme, error) {
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

	sc, err := getScheme()
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
		Host:   host,
		Port:   port,
		// Don't run a metrics server with the manager. Metrics are being served
		// explicitly in the main routine.
		MetricsBindAddress: "0",
		// Manage cluster-wide and namespace resources at the same time.
		NewCache: cache.NewCacheFunc(func(config *rest.Config, options cache.Options) (cache.Cache, error) {
			return cache.New(clientConfig, cache.Options{
				Scheme: options.Scheme,
				// The presence of metadata.namespace has special handling internally causing the
				// cache's watch-list to only watch that namespace.
				SelectorsByObject: cache.SelectorsByObject{
					&corev1.Pod{}: {
						Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": opts.OperatorNamespace}),
					},
					&monitoringv1.PodMonitoring{}: {
						Field: fields.Everything(),
					},
					&monitoringv1.ClusterPodMonitoring{}: {
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
						// We can only have 1 namespace specified here. While we
						// need to access secrets from multiple namespaces, we
						// specify one here so that the manager's client
						// accesses secrets from this namespace through a cache.
						Field: fields.SelectorFromSet(fields.Set{
							"metadata.namespace": opts.PublicNamespace,
						}),
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
		TLSOpts: []func(*tls.Config){
			func(c *tls.Config) {
				rawCA, err := os.ReadFile(opts.ClientCAFile)
				if err != nil {
					logger.Info("unable to read client CA for webhook server, skipping", "error", err.Error())
					return
				}
				ca, err := x509.ParseCertificate(rawCA)
				if err != nil {
					logger.Info("unable to parse client CA for webhook server, skipping", "error", err.Error())
					return
				}
				c.ClientCAs.AddCert(ca)
			},
			func(c *tls.Config) {
				cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
				if err != nil {
					logger.Info("unable to load TLS certificate/key pair for webhook server, skipping", "error", err.Error())
					return
				}
				c.Certificates = []tls.Certificate{cert}
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create controller manager: %w", err)
	}

	namespaces := []string{opts.OperatorNamespace, opts.PublicNamespace}
	managedNamespacesCache, err := cache.MultiNamespacedCacheBuilder(namespaces)(clientConfig, cache.Options{
		Scheme: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("create controller manager: %w", err)
	}

	client, err := client.New(clientConfig, client.Options{Scheme: sc})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	op := &Operator{
		logger:                 logger,
		opts:                   opts,
		client:                 client,
		manager:                manager,
		managedNamespacesCache: managedNamespacesCache,
	}
	return op, nil
}

// setupAdmissionWebhooks configures validating webhooks for the operator-managed
// custom resources and registers handlers with the webhook server.
func (o *Operator) setupAdmissionWebhooks(ctx context.Context) error {
	s := o.manager.GetWebhookServer()

	// Validating webhooks.
	s.Register(
		validatePath(monitoringv1.PodMonitoringResource()),
		admission.ValidatingWebhookFor(&monitoringv1.PodMonitoring{}),
	)
	s.Register(
		validatePath(monitoringv1.ClusterPodMonitoringResource()),
		admission.ValidatingWebhookFor(&monitoringv1.ClusterPodMonitoring{}),
	)
	s.Register(
		validatePath(monitoringv1.OperatorConfigResource()),
		admission.WithCustomValidator(&monitoringv1.OperatorConfig{}, &operatorConfigValidator{
			namespace: o.opts.PublicNamespace,
		}),
	)
	s.Register(
		validatePath(monitoringv1.RulesResource()),
		admission.WithCustomValidator(&monitoringv1.Rules{}, &rulesValidator{
			opts: o.opts,
		}),
	)
	s.Register(
		validatePath(monitoringv1.ClusterRulesResource()),
		admission.WithCustomValidator(&monitoringv1.ClusterRules{}, &clusterRulesValidator{
			opts: o.opts,
		}),
	)
	s.Register(
		validatePath(monitoringv1.GlobalRulesResource()),
		admission.WithCustomValidator(&monitoringv1.GlobalRules{}, &globalRulesValidator{}),
	)
	// Defaulting webhooks.
	s.Register(
		defaultPath(monitoringv1.PodMonitoringResource()),
		admission.WithCustomDefaulter(&monitoringv1.PodMonitoring{}, &podMonitoringDefaulter{}),
	)
	s.Register(
		defaultPath(monitoringv1.ClusterPodMonitoringResource()),
		admission.WithCustomDefaulter(&monitoringv1.ClusterPodMonitoring{}, &clusterPodMonitoringDefaulter{}),
	)
	return nil
}

// Run the reconciliation loop of the operator.
// The passed owner references are set on cluster-wide resources created by the
// operator.
func (o *Operator) Run(ctx context.Context, registry prometheus.Registerer) error {
	defer runtimeutil.HandleCrash()

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
	if err := setupTargetStatusPoller(o, registry); err != nil {
		return fmt.Errorf("setup target status processor: %w", err)
	}

	o.logger.Info("starting GMP operator")

	go func() {
		o.managedNamespacesCache.Start(ctx)
	}()
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

func validatePath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/validate/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func defaultPath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/default/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func (o *Operator) webhookConfigName() string {
	return fmt.Sprintf("%s.%s.monitoring.googleapis.com", NameOperator, o.opts.OperatorNamespace)
}
