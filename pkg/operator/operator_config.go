package operator

import (
	"context"
	"fmt"
	"path"
	"strings"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	promcommonconfig "github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Base resource names which may be used for multiple different resource kinds
// related to the given component.
const (
	NameOperatorConfig = "config"
	NameRuleEvaluator  = "rule-evaluator"
	NameCollector      = "collector"
)

const (
	rulesVolumeName      = "rules"
	secretVolumeName     = "rules-secret"
	RulesSecretName      = "rules"
	CollectionSecretName = "collection"
	rulesDir             = "/etc/rules"
	secretsDir           = "/etc/secrets"
	RuleEvaluatorPort    = 19092
)

func rulesLabels() map[string]string {
	return map[string]string{
		LabelAppName:      NameRuleEvaluator,
		KubernetesAppName: RuleEvaluatorAppName,
	}
}

func rulesAnnotations() map[string]string {
	return map[string]string{
		AnnotationMetricName: componentName,
	}
}

// setupOperatorConfigControllers ensures a rule-evaluator
// deployment as part of managed collection.
func setupOperatorConfigControllers(op *Operator) error {
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
	// Rule-evaluator deployment filter.
	objFilterRuleEvaluator := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      NameRuleEvaluator,
	}

	// Reconcile operator-managed resources.
	err := ctrl.NewControllerManagedBy(op.manager).
		Named("operator-config").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		For(
			&monitoringv1alpha1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		// // Maintain the rule-evaluator deployment and configuration (as a secret).
		Watches(
			&source.Kind{Type: &appsv1.Deployment{}},
			enqueueConst(objRequest),
			builder.WithPredicates(
				objFilterRuleEvaluator,
				predicate.GenerationChangedPredicate{},
			)).
		// We must watch all secrets in the cluster as we copy and inline secrets referenced
		// in the OperatorConfig and need to repeat those steps if affected secrets change.
		// As the set of secrets to watch is not static, we've to watch them all.
		// A viable alternative could be for the rule-evaluator (or a sidecar) to generate
		// the configuration locally and maintain a more constrained watch.
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.NewPredicateFuncs(secretFilter(op.opts.PublicNamespace))),
		).
		Complete(newOperatorConfigReconciler(op.manager.GetClient(), op.opts))

	if err != nil {
		return errors.Wrap(err, "operator-config controller")
	}
	return nil
}

// secretFilter filters by non-default Secrets in specified namespace.
func secretFilter(ns string) func(object client.Object) bool {
	return func(object client.Object) bool {
		if object.GetNamespace() == ns {
			return !strings.HasPrefix(object.GetName(), "default-token")
		}
		return false
	}
}

// operatorConfigReconciler reconciles the OperatorConfig CRD.
type operatorConfigReconciler struct {
	client client.Client
	opts   Options
}

// newOperatorConfigReconciler creates a new operatorConfigReconciler.
func newOperatorConfigReconciler(c client.Client, opts Options) *operatorConfigReconciler {
	return &operatorConfigReconciler{
		client: c,
		opts:   opts,
	}
}

// Reconcile ensures the OperatorConfig resource is reconciled.
func (r *operatorConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := logr.FromContext(ctx).WithValues("operatorconfig", req.NamespacedName)
	logger.Info("reconciling operatorconfig")

	config := &monitoringv1alpha1.OperatorConfig{}

	// Fetch OperatorConfig.
	if err := r.client.Get(ctx, req.NamespacedName, config); apierrors.IsNotFound(err) {
		logger.Info("no operatorconfig created yet")
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "get operatorconfig for incoming: %q", req.String())
	}
	// Ensure the rule-evaluator config and grab any to-be-mirrored
	// secret data on the way.
	secretData, err := r.ensureRuleEvaluatorConfig(ctx, &config.Rules)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator config")
	}

	// Mirror the fetched secret data to where the rule-evaluator can
	// mount and access.
	if err := r.ensureRuleEvaluatorSecrets(ctx, secretData); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator secrets")
	}

	// Ensure the rule-evaluator deployment and volume mounts.
	if err := r.ensureRuleEvaluatorDeployment(ctx, &config.Rules); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator deploy")
	}

	return reconcile.Result{}, nil
}

// ensureRuleEvaluatorConfig reconciles the config for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorConfig(ctx context.Context, spec *monitoringv1alpha1.RuleEvaluatorSpec) (map[string][]byte, error) {
	cfg, secretData, err := r.makeRuleEvaluatorConfig(ctx, spec)
	if err != nil {
		return nil, errors.Wrap(err, "make rule-evaluator configmap")
	}

	// Upsert rule-evaluator config.
	if err := r.client.Update(ctx, cfg); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cfg); err != nil {
			return nil, errors.Wrap(err, "create rule-evaluator config")
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "update rule-evaluator config")
	}
	return secretData, nil
}

// makeRuleEvaluatorConfig creates the config for rule-evaluator.
// This is stored as a Secret rather than a ConfigMap as it could contain
// sensitive configuration information.
func (r *operatorConfigReconciler) makeRuleEvaluatorConfig(ctx context.Context, spec *monitoringv1alpha1.RuleEvaluatorSpec) (*corev1.ConfigMap, map[string][]byte, error) {
	amConfigs, secretData, err := r.makeAlertManagerConfigs(ctx, &spec.Alerting)
	if err != nil {
		return nil, nil, errors.Wrap(err, "make alertmanager config")
	}
	if spec.Credentials != nil {
		p := pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: spec.Credentials})
		b, err := getSecretKeyBytes(ctx, r.client, r.opts.PublicNamespace, spec.Credentials)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get service account credentials")
		}
		secretData[p] = b
	}

	cfg := &promconfig.Config{
		GlobalConfig: promconfig.GlobalConfig{
			ExternalLabels: labels.FromMap(spec.ExternalLabels),
		},
		AlertingConfig: promconfig.AlertingConfig{
			AlertmanagerConfigs: amConfigs,
		},
		RuleFiles: []string{path.Join(rulesDir, "*.yaml")},
	}
	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal Prometheus config")
	}

	// Create rule-evaluator Secret.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NameRuleEvaluator,
			Namespace: r.opts.OperatorNamespace,
		},
		Data: map[string]string{
			configFilename: string(cfgEncoded),
		},
	}
	return cm, secretData, nil
}

// ensureRuleEvaluatorSecrets reconciles the Secrets for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorSecrets(ctx context.Context, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        RulesSecretName,
			Namespace:   r.opts.OperatorNamespace,
			Annotations: rulesAnnotations(),
			Labels:      rulesLabels(),
		},
		Data: make(map[string][]byte),
	}
	for f, b := range data {
		secret.Data[f] = b
	}

	if err := r.client.Update(ctx, secret); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, secret); err != nil {
			return errors.Wrap(err, "create rule-evaluator secrets")
		}
	} else if err != nil {
		return errors.Wrap(err, "update rule-evaluator secrets")
	}
	return nil
}

// ensureRuleEvaluatorDeployment reconciles the Deployment for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorDeployment(ctx context.Context, spec *monitoringv1alpha1.RuleEvaluatorSpec) error {
	deploy := r.makeRuleEvaluatorDeployment(spec)

	// Upsert rule-evaluator Deployment.
	// We've to use Patch() as Update() with update we'll get stuck in an infinite loop as each
	// update increases the generation since we override the managed fields metadata, which is
	// set by the kube-controller-manager, which wants to own an annotation on deployments.
	if err := r.client.Patch(ctx, deploy, client.Merge); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, deploy); err != nil {
			return errors.Wrap(err, "create rule-evaluator deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "update rule-evaluator deployment")
	}
	return nil
}

// makeRuleEvaluatorDeployment creates the Deployment for rule-evaluator.
func (r *operatorConfigReconciler) makeRuleEvaluatorDeployment(spec *monitoringv1alpha1.RuleEvaluatorSpec) *appsv1.Deployment {
	evaluatorArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(configOutDir, configFilename)),
		fmt.Sprintf("--web.listen-address=:%d", RuleEvaluatorPort),
	}
	if r.opts.ProjectID != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.label.project-id=%s", r.opts.ProjectID))
	}
	if r.opts.Location != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.label.location=%s", r.opts.Location))
	}
	if r.opts.Cluster != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.label.cluster=%s", r.opts.Cluster))
	}
	if r.opts.DisableExport {
		evaluatorArgs = append(evaluatorArgs, "--export.disable")
	}
	if r.opts.CloudMonitoringEndpoint != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.endpoint=%s", r.opts.CloudMonitoringEndpoint))
	}
	// If no explicit project ID is set, use the one provided to the operator. On GKE the rule-evaluator
	// can also auto-detect the cluster's project but this won't work in other Kubernetes environments.
	queryProjectID := r.opts.ProjectID
	if spec.QueryProjectID != "" {
		queryProjectID = spec.QueryProjectID
	}
	evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--query.project-id=%s", queryProjectID))

	if spec.Credentials != nil {
		p := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: spec.Credentials}))
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.credentials-file=%s", p))
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--query.credentials-file=%s", p))
	}
	replicas := int32(1)

	deploy := appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: rulesLabels(),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      rulesLabels(),
				Annotations: rulesAnnotations(),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "evaluator",
						Image: r.opts.ImageRuleEvaluator,
						Args:  evaluatorArgs,
						Ports: []corev1.ContainerPort{
							{Name: "r-eval-metrics", ContainerPort: RuleEvaluatorPort},
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/healthy",
									Port: intstr.FromInt(RuleEvaluatorPort),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/-/ready",
									Port: intstr.FromInt(RuleEvaluatorPort),
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      configOutVolumeName,
								MountPath: configOutDir,
								ReadOnly:  true,
							},
							{
								Name:      rulesVolumeName,
								MountPath: rulesDir,
								ReadOnly:  true,
							},
							{
								Name:      secretVolumeName,
								MountPath: secretsDir,
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
								corev1.ResourceMemory: *resource.NewScaledQuantity(1, resource.Giga),
							},
						},
					}, {
						Name:  "config-reloader",
						Image: r.opts.ImageConfigReloader,
						Args: []string{
							fmt.Sprintf("--config-file=%s", path.Join(configDir, configFilename)),
							fmt.Sprintf("--config-file-output=%s", path.Join(configOutDir, configFilename)),
							fmt.Sprintf("--watched-dir=%s", rulesDir),
							fmt.Sprintf("--watched-dir=%s", secretsDir),
							fmt.Sprintf("--reload-url=http://localhost:%d/-/reload", RuleEvaluatorPort),
							fmt.Sprintf("--listen-address=:%d", RuleEvaluatorPort+1),
						},
						Ports: []corev1.ContainerPort{
							{Name: "cfg-rel-metrics", ContainerPort: RuleEvaluatorPort + 1},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      configVolumeName,
								MountPath: configDir,
								ReadOnly:  true,
							},
							{
								Name:      configOutVolumeName,
								MountPath: configOutDir,
							},
							{
								Name:      rulesVolumeName,
								MountPath: rulesDir,
								ReadOnly:  true,
							},
							{
								Name:      secretVolumeName,
								MountPath: secretsDir,
								ReadOnly:  true,
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
						// Rule-evaluator input Prometheus config.
						Name: configVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: NameRuleEvaluator,
								},
							},
						},
					}, {
						// Generated rule-evaluator output Prometheus config.
						Name: configOutVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}, {
						// Generated rules yaml files via the "rules" runtime controller.
						Name: rulesVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: nameRulesGenerated,
								},
							},
						},
					}, {
						// Mirrored config secrets (config specified as filepaths).
						Name: secretVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: RulesSecretName,
							},
						},
					},
				},
				// Collector service account used for K8s endpoints-based SD.
				// TODO(pintohutch): confirm minimum serviceAccount credentials needed for rule-evaluator
				// and create dedicated serviceAccount.
				ServiceAccountName: NameCollector,
				PriorityClassName:  r.opts.PriorityClass,
				HostNetwork:        r.opts.HostNetwork,
			},
		},
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      NameRuleEvaluator,
		},
		Spec: deploy,
	}
}

// makeAlertManagerConfigs creates the alertmanager_config entries as described in
// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alertmanager_config.
func (r *operatorConfigReconciler) makeAlertManagerConfigs(ctx context.Context, spec *monitoringv1alpha1.AlertingSpec) (promconfig.AlertmanagerConfigs, map[string][]byte, error) {
	var (
		err        error
		configs    promconfig.AlertmanagerConfigs
		secretData = make(map[string][]byte)
	)
	for _, am := range spec.Alertmanagers {
		// The upstream struct is lacking the omitempty field on the API version. Thus it looks
		// like we explicitly set it to empty (invalid) even if left empty after marshalling.
		// Thus we initialize the config with defaulting. Similar applies for the embedded HTTPConfig.
		cfg := promconfig.DefaultAlertmanagerConfig

		if am.PathPrefix != "" {
			cfg.PathPrefix = am.PathPrefix
		}
		if am.Scheme != "" {
			cfg.Scheme = am.Scheme
		}
		if am.APIVersion != "" {
			cfg.APIVersion = promconfig.AlertmanagerAPIVersion(am.APIVersion)
		}

		// Timeout, APIVersion, PathPrefix, and Scheme all resort to defaults if left unspecified.
		if am.Timeout != "" {
			cfg.Timeout, err = prommodel.ParseDuration(am.Timeout)
			if err != nil {
				return nil, nil, errors.Wrap(err, "invalid timeout")
			}
		}
		// Authorization.
		if am.Authorization != nil {
			cfg.HTTPClientConfig.Authorization = &promcommonconfig.Authorization{
				Type: am.Authorization.Type,
			}
			if c := am.Authorization.Credentials; c != nil {
				b, err := getSecretKeyBytes(ctx, r.client, r.opts.PublicNamespace, c)
				if err != nil {
					return nil, nil, err
				}
				p := pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: c})

				secretData[p] = b
				cfg.HTTPClientConfig.Authorization.CredentialsFile = path.Join(secretsDir, p)
			}
		}
		// TLS config.
		if am.TLS != nil {
			tlsCfg := promcommonconfig.TLSConfig{
				InsecureSkipVerify: am.TLS.InsecureSkipVerify,
				ServerName:         am.TLS.ServerName,
			}
			if am.TLS.CA != nil {
				p := pathForSelector(r.opts.PublicNamespace, am.TLS.CA)
				b, err := getSecretOrConfigMapBytes(ctx, r.client, r.opts.PublicNamespace, am.TLS.CA)
				if err != nil {
					return nil, nil, err
				}
				secretData[p] = b
				tlsCfg.CAFile = path.Join(secretsDir, p)
			}
			if am.TLS.Cert != nil {
				p := pathForSelector(r.opts.PublicNamespace, am.TLS.Cert)
				b, err := getSecretOrConfigMapBytes(ctx, r.client, r.opts.PublicNamespace, am.TLS.Cert)
				if err != nil {
					return nil, nil, err
				}
				secretData[p] = b
				tlsCfg.CertFile = path.Join(secretsDir, p)
			}
			if am.TLS.KeySecret != nil {
				p := pathForSelector(r.opts.PublicNamespace, &monitoringv1alpha1.SecretOrConfigMap{Secret: am.TLS.KeySecret})
				b, err := getSecretKeyBytes(ctx, r.client, r.opts.PublicNamespace, am.TLS.KeySecret)
				if err != nil {
					return nil, nil, err
				}
				secretData[p] = b
				tlsCfg.KeyFile = path.Join(secretsDir, p)
			}

			cfg.HTTPClientConfig.TLSConfig = tlsCfg
		}

		// Configure discovery of AM endpoints via Kubernetes API.
		cfg.ServiceDiscoveryConfigs = discovery.Configs{
			&discoverykube.SDConfig{
				// Must instantiate a default client config explicitly as the follow_redirects
				// field lacks the omitempty tag. Thus it looks like we explicitly set it to false
				// even if left empty after marshalling.
				HTTPClientConfig: promcommonconfig.DefaultHTTPClientConfig,
				Role:             discoverykube.RoleEndpoint,
				NamespaceDiscovery: discoverykube.NamespaceDiscovery{
					Names: []string{am.Namespace},
				},
			},
		}
		svcNameRE, err := relabel.NewRegexp(am.Name)
		if err != nil {
			return nil, nil, errors.Errorf("cannot build regex from service name %q: %s", am.Name, err)
		}
		cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_service_name"},
			Regex:        svcNameRE,
		})
		portRE, err := relabel.NewRegexp(am.Port.String())
		if err != nil {
			return nil, nil, errors.Errorf("cannot build regex from port %q: %s", am.Port, err)
		}
		if am.Port.StrVal != "" {
			cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_endpoint_port_name"},
				Regex:        portRE,
			})
		} else if am.Port.IntVal != 0 {
			cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_container_port_number"},
				Regex:        portRE,
			})
		}

		// TODO(pintohutch): add support for basic_auth, oauth2, proxy_url, follow_redirects.

		// Append to alertmanagers config array.
		configs = append(configs, &cfg)
	}

	return configs, secretData, nil
}

// getSecretOrConfigMapBytes is a helper function to conditionally fetch
// the secret or configmap selector payloads.
func getSecretOrConfigMapBytes(ctx context.Context, kClient client.Reader, namespace string, scm *monitoringv1alpha1.SecretOrConfigMap) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	if secret := scm.Secret; secret != nil {
		b, err = getSecretKeyBytes(ctx, kClient, namespace, secret)
		if err != nil {
			return b, err
		}
	} else if cm := scm.ConfigMap; cm != nil {
		b, err = getConfigMapKeyBytes(ctx, kClient, namespace, cm)
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

// getSecretKeyBytes processes the given NamespacedSecretKeySelector and returns the referenced data.
func getSecretKeyBytes(ctx context.Context, kClient client.Reader, namespace string, sel *v1.SecretKeySelector) ([]byte, error) {
	var (
		secret = &corev1.Secret{}
		nn     = types.NamespacedName{
			Namespace: namespace,
			Name:      sel.Name,
		}
		bytes []byte
	)
	err := kClient.Get(ctx, nn, secret)
	if err != nil {
		return bytes, errors.Wrapf(err, "unable to get secret %q", sel.Name)
	}
	bytes, ok := secret.Data[sel.Key]
	if !ok {
		return bytes, errors.Errorf("key %q in secret %q not found", sel.Key, sel.Name)
	}

	return bytes, nil
}

// getConfigMapKeyBytes processes the given NamespacedConfigMapKeySelector and returns the referenced data.
func getConfigMapKeyBytes(ctx context.Context, kClient client.Reader, namespace string, sel *v1.ConfigMapKeySelector) ([]byte, error) {
	var (
		cm = &corev1.ConfigMap{}
		nn = types.NamespacedName{
			Namespace: namespace,
			Name:      sel.Name,
		}
		b []byte
	)
	err := kClient.Get(ctx, nn, cm)
	if err != nil {
		return b, errors.Wrapf(err, "unable to get secret %q", sel.Name)
	}

	// Check 'data' first, then 'binaryData'.
	if s, ok := cm.Data[sel.Key]; ok {
		return []byte(s), nil
	} else if b, ok := cm.BinaryData[sel.Key]; ok {
		return b, nil
	} else {
		return b, errors.Errorf("key %q in secret %q not found", sel.Key, sel.Name)
	}
}

// pathForSelector cretes the filepath for the provided NamespacedSecretOrConfigMap.
// This can be used to avoid naming collisions of like-keys across K8s resources.
func pathForSelector(namespace string, scm *monitoringv1alpha1.SecretOrConfigMap) string {
	if scm == nil {
		return ""
	}
	if scm.ConfigMap != nil {
		return fmt.Sprintf("%s_%s_%s_%s", "configmap", namespace, scm.ConfigMap.Name, scm.ConfigMap.Key)
	}
	if scm.Secret != nil {
		return fmt.Sprintf("%s_%s_%s_%s", "secret", namespace, scm.Secret.Name, scm.Secret.Key)
	}
	return ""
}
