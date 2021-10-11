package operator

import (
	"context"
	"fmt"
	"path"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	NameRuleEvaluator = "rule-evaluator"
	rulesVolumeName   = "rules"
	rulesDir          = "/etc/rules"
	RuleEvaluatorPort = 19092
)

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
			&corev1.ConfigMap{},
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

	var config = &monitoringv1alpha1.OperatorConfig{}
	if err := r.client.Get(ctx, req.NamespacedName, config); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get operatorconfig")
	}
	if err := r.ensureRuleEvaluatorConfig(ctx, config); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator config")
	}
	if err := r.ensureRuleEvaluatorDeployment(ctx, &config.Rules); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure rule-evaluator deploy")
	}

	return reconcile.Result{}, nil
}

// ensureRuleEvaluatorConfig reconciles the ConfigMap for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorConfig(ctx context.Context, config *monitoringv1alpha1.OperatorConfig) error {
	amConfigs, err := makeAlertManagerConfigs(&config.Rules.Alerting)
	if err != nil {
		return errors.Wrap(err, "make alertmanager config")
	}
	cm, err := makeRuleEvaluatorConfigMap(amConfigs, NameRuleEvaluator, r.opts.OperatorNamespace, "config.yaml")
	if err != nil {
		return errors.Wrap(err, "make rule-evaluator configmap")
	}

	// Upsert rule-evaluator ConfigMap.
	if err := r.client.Update(ctx, cm); err != nil {
		if err := r.client.Create(ctx, cm); err != nil {
			return errors.Wrap(err, "create rule-evaluator config")
		}
	} else if err != nil {
		return errors.Wrap(err, "update rule-evaluator config")
	}
	return nil
}

// makeRuleEvaluatorConfigMap creates the ConfigMap for rule-evaluator.
// TODO(pintohutch): change function signature to use native Promethues go structs
// over k8s configmap.
func makeRuleEvaluatorConfigMap(amConfigs []yaml.MapSlice, name, namespace, filename string) (*corev1.ConfigMap, error) {
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

	// Create rule-evaluator ConfigMap.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			filename: string(cfgEncoded),
		},
	}
	return cm, nil
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
	podLabels := map[string]string{
		LabelAppName: NameRuleEvaluator,
	}
	podAnnotations := map[string]string{
		AnnotationMetricName: componentName,
	}
	evaluatorArgs := []string{fmt.Sprintf("--config.file=%s", path.Join(configOutDir, configFilename)),
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
			MatchLabels: podLabels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      podLabels,
				Annotations: podAnnotations,
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
func makeAlertManagerConfigs(spec *monitoringv1alpha1.AlertingSpec) ([]yaml.MapSlice, error) {
	var configs []yaml.MapSlice
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
		// TODO(pintohutch): fill the rest out.

		configs = append(configs, cfg)
	}
	return configs, nil
}
