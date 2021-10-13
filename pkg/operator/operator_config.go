package operator

import (
	"context"
	"fmt"
	"path"
	"strings"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TODO(pintohutch): move these into the operator Options and pass in.
const (
	NameRuleEvaluator = "rule-evaluator"
	rulesVolumeName   = "rules"
	secretVolumeName  = "rules-secret"
	RulesSecretName   = "rules"
	rulesDir          = "/etc/rules"
	secretsDir        = "/etc/secrets"
	RuleEvaluatorPort = 19092
)

func rulesLabels() map[string]string {
	return map[string]string{
		LabelAppName: NameRuleEvaluator,
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
	// Canonical filter to only capture events for the generated
	// rule evaluator deployment.
	objFilter := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      NameRuleEvaluator,
	}

	err := ctrl.NewControllerManagedBy(op.manager).
		Named("operator-config").
		For(
			&monitoringv1alpha1.OperatorConfig{},
		).
		Owns(
			&corev1.Secret{},
			builder.WithPredicates(objFilter)).
		Owns(
			&appsv1.Deployment{},
			builder.WithPredicates(objFilter)).
		Complete(newOperatorConfigReconciler(op.manager.GetClient(), op.opts))

	if err != nil {
		return errors.Wrap(err, "operator-config controller")
	}
	return nil
}

// operatorConfigReconciler reconciles the OperatorConfig CRD.
type operatorConfigReconciler struct {
	client     client.Client
	opts       Options
	secretData map[string][]byte
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

	var (
		config     = &monitoringv1alpha1.OperatorConfig{}
		secretData map[string][]byte
	)

	// Fetch OperatorConfig.
	if err := r.client.Get(ctx, req.NamespacedName, config); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get operatorconfig")
	}

	// Ensure the rule-evaluator config and grab any to-be-mirrored
	// secret data on the way.
	secretData, err := r.ensureRuleEvaluatorConfig(ctx, config)
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
func (r *operatorConfigReconciler) ensureRuleEvaluatorConfig(ctx context.Context, oc *monitoringv1alpha1.OperatorConfig) (map[string][]byte, error) {
	amConfigs, secretData, err := r.makeAlertManagerConfigs(ctx, &oc.Rules.Alerting)
	if err != nil {
		return secretData, errors.Wrap(err, "make alertmanager config")
	}
	cfg, err := makeRuleEvaluatorConfig(amConfigs, NameRuleEvaluator, r.opts.OperatorNamespace, configFilename)
	if err != nil {
		return secretData, errors.Wrap(err, "make rule-evaluator configmap")
	}

	// Upsert rule-evaluator config.
	if err := r.client.Update(ctx, cfg); err != nil {
		if err := r.client.Create(ctx, cfg); err != nil {
			return secretData, errors.Wrap(err, "create rule-evaluator config")
		}
	} else if err != nil {
		return secretData, errors.Wrap(err, "update rule-evaluator config")
	}
	return secretData, nil
}

// makeRuleEvaluatorConfig creates the config for rule-evaluator.
// This is stored as a Secret rather than a ConfigMap as it could contain
// sensitive configuration information.
// TODO(pintohutch): change function signature to use native Promethues go structs
// over k8s configmap.
func makeRuleEvaluatorConfig(amConfigs []yaml.MapSlice, name, namespace, filename string) (*corev1.Secret, error) {
	// Prepare and encode the Prometheus config used in rule-evaluator.
	pmConfig := yaml.MapSlice{}

	// Add alertmanager configuration.
	pmConfig = append(pmConfig,
		yaml.MapItem{
			Key: "alerting",
			Value: yaml.MapSlice{
				{
					Key:   "alertmanagers",
					Value: amConfigs,
				},
			},
		},
	)

	// Add rules configuration.
	pmConfig = append(pmConfig,
		yaml.MapItem{
			Key:   "rule_files",
			Value: []string{path.Join(rulesDir, "*.yaml")},
		},
	)
	cfgEncoded, err := yaml.Marshal(pmConfig)
	if err != nil {
		return nil, errors.Wrap(err, "marshal Prometheus config")
	}

	// Create rule-evaluator Secret.
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			filename: cfgEncoded,
		},
	}
	return s, nil
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
func (r *operatorConfigReconciler) ensureRuleEvaluatorDeployment(ctx context.Context, rules *monitoringv1alpha1.RuleEvaluatorSpec) error {
	deploy := r.makeRuleEvaluatorDeployment(rules)

	// Upsert rule-evaluator ConfigMap.
	if err := r.client.Update(ctx, deploy); err != nil {
		if err := r.client.Create(ctx, deploy); err != nil {
			return errors.Wrap(err, "create rule-evaluator deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "update rule-evaluator deployment")
	}
	return nil
}

// makeRuleEvaluatorDeployment creates the Deployment for rule-evaluator.
func (r *operatorConfigReconciler) makeRuleEvaluatorDeployment(rules *monitoringv1alpha1.RuleEvaluatorSpec) *appsv1.Deployment {
	evaluatorArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(configOutDir, configFilename)),
		fmt.Sprintf("--web.listen-address=:%d", RuleEvaluatorPort),
	}
	if rules.ProjectID != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--query.project-id=%s", rules.ProjectID))
	}
	if rules.LabelProjectID != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.label.project-id=%s", rules.LabelProjectID))
	}
	if rules.LabelLocation != "" {
		evaluatorArgs = append(evaluatorArgs, fmt.Sprintf("--export.label.location=%s", rules.LabelLocation))
	}
	spec := appsv1.DeploymentSpec{
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
							Secret: &corev1.SecretVolumeSource{
								SecretName: NameRuleEvaluator,
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
						// TODO(pintohutch): create dummy Rules resource on startup.
						// At this time, the operator-config runtime controller
						// does not guarantee this configmap exists. So unless a Rules
						// resource is created separately, the rule-evaluator deployment
						// will not be in a Running state.
						// Though empirically, it seems the operator creates this configmap
						// when it's created and running in a k8s cluster...?
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
				ServiceAccountName: CollectorName,
				PriorityClassName:  r.opts.PriorityClass,
				// When a cluster has Workload Identity enabled, the default GCP service account
				// of the node is no longer accessible. That is unless the pod runs on the host network,
				// in which case it keeps accessing the GCE metadata agent, rather than the GKE metadata
				// agent.
				// We run in the host network for now to match behavior of other GKE
				// telemetry agents and not require an additional permission setup step for collection.
				// This relies on the default GCP service account to have write permissions for Cloud
				// Monitoring set, which generally is the case.
				HostNetwork: true,
			},
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.opts.OperatorNamespace,
			Name:      NameRuleEvaluator,
		},
		Spec: spec,
	}
	return deploy
}

// makeAlertManagerConfigs creates the alertmanager_config entries as described in
// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alertmanager_config.
// TODO(pintohutch): change function signature to use native Promethues go structs
// over []yaml.MapSlice.
func (r *operatorConfigReconciler) makeAlertManagerConfigs(ctx context.Context, spec *monitoringv1alpha1.AlertingSpec) ([]yaml.MapSlice, map[string][]byte, error) {
	var (
		configs    []yaml.MapSlice
		secretData = make(map[string][]byte)
	)
	for _, am := range spec.Alertmanagers {
		var cfg yaml.MapSlice
		// Timeout, APIVersion, PathPrefix, and Scheme all resort to defaults if left unspecified.
		if am.Timeout != "" {
			cfg = append(cfg, yaml.MapItem{Key: "timeout", Value: am.Timeout})
		}
		// Default to V2 Alertmanager version.
		if am.APIVersion != "" {
			cfg = append(cfg, yaml.MapItem{Key: "api_version", Value: am.APIVersion})
		}
		// Default to / path prefix.
		if am.PathPrefix != "" {
			cfg = append(cfg, yaml.MapItem{Key: "path_prefix", Value: am.PathPrefix})
		}
		// Default to http scheme.
		if am.Scheme != "" {
			cfg = append(cfg, yaml.MapItem{Key: "scheme", Value: am.Scheme})
		}

		// Authorization.
		if am.Authorization != nil {
			// TODO(pintohutch): use native Prometheus structs here.
			authCfg := yaml.MapSlice{}
			if t := am.Authorization.Type; t != "" {
				authCfg = append(authCfg, yaml.MapItem{Key: "type", Value: strings.TrimSpace(t)})
			}
			if c := am.Authorization.Credentials; c != nil {
				b, err := getSecretKeyBytes(ctx, r.client, c)
				if err != nil {
					return configs, secretData, err
				}
				authCfg = append(authCfg, yaml.MapItem{Key: "credentials", Value: string(b)})
			}
			cfg = append(cfg, yaml.MapItem{Key: "authorization", Value: authCfg})
		}

		// TLS config.
		if tls := am.TLSConfig; tls != nil {
			cfg = append(cfg, yaml.MapItem{Key: "tls_config", Value: tlsConfigYAML(secretsDir, tls)})
			// Populate secretData cache to act on (i.e. upsert dedicated Secret) later.
			sd, err := getTLSSecretData(ctx, r.client, tls)
			if err != nil {
				return configs, sd, err
			}
			secretData = sd
		}

		// Kubernetes SD configs.
		cfg = append(cfg, yaml.MapItem{Key: "kubernetes_sd_configs", Value: k8sSDConfigYAML(am.Namespace)})

		// Relabel configs.
		cfg = append(cfg, yaml.MapItem{Key: "relabel_configs", Value: relabelConfigsYAML(&am)})

		// TODO(pintohutch): add support for basic_auth, oauth2, proxy_url, follow_redirects.

		// Append to alertmanagers config array.
		configs = append(configs, cfg)
	}

	return configs, secretData, nil
}

// tlsConfigYAML creates a yaml.MapSlice compatible with https://prometheus.io/docs/prometheus/latest/configuration/configuration/#tls_config
// from the provided TLSConfig and mount path `dir`.
func tlsConfigYAML(dir string, tls *monitoringv1alpha1.TLSConfig) yaml.MapSlice {
	var (
		filepath  string
		tlsConfig = yaml.MapSlice{
			{Key: "insecure_skip_verify", Value: tls.InsecureSkipVerify},
		}
	)
	if tls.CA.Secret != nil || tls.CA.ConfigMap != nil {
		filepath = path.Join(dir, pathForSelector(&tls.CA))
		tlsConfig = append(tlsConfig, yaml.MapItem{Key: "ca_file", Value: filepath})
	}
	if tls.Cert.Secret != nil || tls.Cert.ConfigMap != nil {
		filepath = path.Join(dir, pathForSelector(&tls.Cert))
		tlsConfig = append(tlsConfig, yaml.MapItem{Key: "cert_file", Value: filepath})
	}
	if tls.KeySecret != nil {
		scm := &monitoringv1alpha1.NamespacedSecretOrConfigMap{Secret: tls.KeySecret}
		filepath = path.Join(dir, pathForSelector(scm))
		tlsConfig = append(tlsConfig, yaml.MapItem{Key: "key_file", Value: filepath})
	}
	if tls.ServerName != "" {
		tlsConfig = append(tlsConfig, yaml.MapItem{Key: "server_name", Value: tls.ServerName})
	}
	return tlsConfig
}

// k8sSDConfigYAML returns the kubernetes_sd_config YAML spec
// from the provided namespace.
func k8sSDConfigYAML(namespace string) []yaml.MapSlice {
	k8sSDConfig := yaml.MapSlice{
		{
			Key:   "role",
			Value: "endpoints",
		},
	}

	k8sSDConfig = append(k8sSDConfig, yaml.MapItem{
		Key: "namespaces",
		Value: yaml.MapSlice{
			{
				Key:   "names",
				Value: []string{namespace},
			},
		},
	})

	return []yaml.MapSlice{
		k8sSDConfig,
	}
}

// relabelConfigsYAML returns the relabel_configs YAML spec
// from the provided AlertmanagerEndpoints.
func relabelConfigsYAML(am *monitoringv1alpha1.AlertmanagerEndpoints) []yaml.MapSlice {
	var relabelings []yaml.MapSlice

	relabelings = append(relabelings, yaml.MapSlice{
		{Key: "action", Value: "keep"},
		{Key: "source_labels", Value: []string{"__meta_kubernetes_service_name"}},
		{Key: "regex", Value: am.Name},
	})

	if am.Port.StrVal != "" {
		relabelings = append(relabelings, yaml.MapSlice{
			{Key: "action", Value: "keep"},
			{Key: "source_labels", Value: []string{"__meta_kubernetes_endpoint_port_name"}},
			{Key: "regex", Value: am.Port.String()},
		})
	} else if am.Port.IntVal != 0 {
		relabelings = append(relabelings, yaml.MapSlice{
			{Key: "action", Value: "keep"},
			{Key: "source_labels", Value: []string{"__meta_kubernetes_pod_container_port_number"}},
			{Key: "regex", Value: am.Port.String()},
		})
	}

	return relabelings
}

// getTLSSecretData parses the provided TLSConfig and fetches the secret key bytes
// and returns them in a map, keyed by unique filenames.
func getTLSSecretData(ctx context.Context, kClient client.Reader, tls *monitoringv1alpha1.TLSConfig) (map[string][]byte, error) {
	var m = make(map[string][]byte)
	// Fetch CA cert bytes.
	b, err := getSecretOrConfigMapBytes(ctx, kClient, &tls.CA)
	if err != nil {
		return m, err
	}
	m[pathForSelector(&tls.CA)] = b

	// Fetch client cert bytes.
	b, err = getSecretOrConfigMapBytes(ctx, kClient, &tls.Cert)
	if err != nil {
		return m, err
	}
	m[pathForSelector(&tls.Cert)] = b

	// Fetch secret client key bytes.
	if secret := tls.KeySecret; secret != nil {
		b, err := getSecretKeyBytes(ctx, kClient, secret)
		if err != nil {
			return m, err
		}
		m[pathForSelector(&monitoringv1alpha1.NamespacedSecretOrConfigMap{Secret: tls.KeySecret})] = b
	}
	return m, nil
}

// getSecretOrConfigMapBytes is a helper function to conditionally fetch
// the secret or configmap selector payloads.
func getSecretOrConfigMapBytes(ctx context.Context, kClient client.Reader, scm *monitoringv1alpha1.NamespacedSecretOrConfigMap) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	if secret := scm.Secret; secret != nil {
		b, err = getSecretKeyBytes(ctx, kClient, secret)
		if err != nil {
			return b, err
		}
	} else if cm := scm.ConfigMap; cm != nil {
		b, err = getConfigMapKeyBytes(ctx, kClient, cm)
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

// getSecretKeyBytes processes the given NamespacedSecretKeySelector and returns the referenced data.
func getSecretKeyBytes(ctx context.Context, kClient client.Reader, sel *monitoringv1alpha1.NamespacedSecretKeySelector) ([]byte, error) {
	var (
		secret = &corev1.Secret{}
		nn     = types.NamespacedName{
			Namespace: sel.Namespace,
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
func getConfigMapKeyBytes(ctx context.Context, kClient client.Reader, sel *monitoringv1alpha1.NamespacedConfigMapKeySelector) ([]byte, error) {
	var (
		cm = &corev1.ConfigMap{}
		nn = types.NamespacedName{
			Namespace: sel.Namespace,
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
func pathForSelector(scm *monitoringv1alpha1.NamespacedSecretOrConfigMap) string {
	if scm == nil {
		return ""
	}
	if scm.ConfigMap != nil {
		return fmt.Sprintf("%s_%s_%s_%s", "configmap", scm.ConfigMap.Namespace, scm.ConfigMap.Name, scm.ConfigMap.Key)
	}
	if scm.Secret != nil {
		return fmt.Sprintf("%s_%s_%s_%s", "secret", scm.Secret.Namespace, scm.Secret.Name, scm.Secret.Key)
	}
	return ""
}
