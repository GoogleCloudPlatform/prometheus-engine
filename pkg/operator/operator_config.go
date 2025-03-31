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
	"fmt"
	"path"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	alertmanagerconfig "github.com/prometheus/alertmanager/config"
	promcommonconfig "github.com/prometheus/common/config"
	prommodel "github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	discoverykube "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	yaml "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Base resource names which may be used for multiple different resource kinds
// related to the given component.
const (
	NameOperatorConfig = "config"
	NameRuleEvaluator  = "rule-evaluator"
	NameCollector      = "collector"
	NameAlertmanager   = "alertmanager"
)

// Secret paths.
const (
	RulesSecretName              = "rules"
	CollectionSecretName         = "collection"
	AlertmanagerSecretName       = "alertmanager"
	AlertmanagerPublicSecretName = "alertmanager"
	AlertmanagerPublicSecretKey  = "alertmanager.yaml"
	rulesDir                     = "/etc/rules"
	secretsDir                   = "/etc/secrets"
	AlertmanagerConfigKey        = "config.yaml"
)

// Collector Kubernetes Deployment extraction/detection.
const (
	CollectorPrometheusContainerName         = "prometheus"
	CollectorPrometheusContainerPortName     = "prom-metrics"
	CollectorConfigReloaderContainerPortName = "cfg-rel-metrics"
	RuleEvaluatorContainerName               = "evaluator"
	AlertmanagerContainerName                = "alertmanager"
)

var AlertmanagerNoOpConfig = `
receivers:
  - name: "noop"
route:
  receiver: "noop"
`

func rulesLabels() map[string]string {
	return map[string]string{
		LabelAppName:      NameRuleEvaluator,
		KubernetesAppName: RuleEvaluatorAppName,
	}
}

func alertmanagerLabels() map[string]string {
	return map[string]string{
		LabelAppName:      NameAlertmanager,
		KubernetesAppName: AlertmanagerAppName,
	}
}

func componentAnnotations() map[string]string {
	return map[string]string{
		AnnotationMetricName: componentName,
		// Allow cluster autoscaler to evict evaluator Pods even though the Pods
		// have an emptyDir volume mounted. This is okay since the node where the
		// Pod runs will be scaled down.
		ClusterAutoscalerSafeEvictionLabel: "true",
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
	// Rule-evaluator secret filter.
	objFilterRuleEvaluatorSecret := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      RulesSecretName,
	}
	// Rule-evaluator secret filter.
	objFilterAlertManagerSecret := namespacedNamePredicate{
		namespace: op.opts.OperatorNamespace,
		name:      AlertmanagerSecretName,
	}

	// Reconcile operator-managed resources.
	err := ctrl.NewControllerManagedBy(op.manager).
		Named("operator-config").
		// Filter events without changes for all watches.
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		For(
			&monitoringv1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		Watches(
			&appsv1.Deployment{},
			enqueueConst(objRequest),
			builder.WithPredicates(
				objFilterRuleEvaluator,
				predicate.GenerationChangedPredicate{},
			)).
		Watches(
			&corev1.Secret{},
			enqueueConst(objRequest),
			builder.WithPredicates(predicate.NewPredicateFuncs(secretFilter(op.opts.PublicNamespace))),
		).
		// Detect and undo changes to the secret.
		Watches(
			&corev1.Secret{},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterRuleEvaluatorSecret)).
		Watches(
			&corev1.Secret{},
			enqueueConst(objRequest),
			builder.WithPredicates(objFilterAlertManagerSecret)).
		Complete(newOperatorConfigReconciler(op.manager.GetClient(), op.opts))

	if err != nil {
		return fmt.Errorf("operator-config controller: %w", err)
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
	logger, _ := logr.FromContext(ctx)
	logger.WithValues("operatorconfig", req.NamespacedName).Info("reconciling operatorconfig")

	config, err := r.ensureOperatorConfig(ctx, logger, req)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ensure the rule-evaluator config and grab any to-be-mirrored
	// secret data on the way.
	secretData, err := r.ensureRuleEvaluatorConfig(ctx, &config.Rules)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure rule-evaluator config: %w", err)
	}

	// Ensure the alertmanager configuration is pulled from the spec.
	if err := r.ensureAlertmanagerConfigSecret(ctx, config.ManagedAlertmanager); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure alertmanager config secret: %w", err)
	}

	if err := r.ensureAlertmanagerStatefulSet(ctx, config.ManagedAlertmanager); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure alertmanager statefulset: %w", err)
	}

	// Mirror the fetched secret data to where the rule-evaluator can
	// mount and access.
	if err := r.ensureRuleEvaluatorSecrets(ctx, secretData); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure rule-evaluator secrets: %w", err)
	}

	// Ensure the rule-evaluator deployment and volume mounts.
	if err := r.ensureRuleEvaluatorDeployment(ctx); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure rule-evaluator deploy: %w", err)
	}

	return reconcile.Result{}, nil
}

// ensureOperatorConfig returns either the defaulted user-defined OperatorConfig, or if it
// does not exist in the cluster, a default OperatorConfig. If the user-defined
// OperatorConfig is missing default values, it is updated in the cluster.
func (r *operatorConfigReconciler) ensureOperatorConfig(ctx context.Context, logger logr.Logger, req reconcile.Request) (*monitoringv1.OperatorConfig, error) {
	exists := true
	config := &monitoringv1.OperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace,
			Name:      req.Name,
		},
	}
	if err := r.client.Get(ctx, req.NamespacedName, config); apierrors.IsNotFound(err) {
		logger.Info("no operatorconfig created yet")
		exists = false
	} else if err != nil {
		return nil, fmt.Errorf("get operatorconfig for incoming: %q: %w", req.String(), err)
	}
	defaulter := &operatorConfigDefaulter{
		projectID: r.opts.ProjectID,
		location:  r.opts.Location,
		cluster:   r.opts.Cluster,
	}
	wasUpdated := defaulter.update(config)
	if exists && wasUpdated {
		if err := r.client.Update(ctx, config); err != nil {
			return nil, fmt.Errorf("default operatorconfig: %w", err)
		}
	}
	return config, nil
}

// ensureRuleEvaluatorConfig reconciles the config for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorConfig(ctx context.Context, spec *monitoringv1.RuleEvaluatorSpec) (map[string][]byte, error) {
	cfg, secretData, err := r.makeRuleEvaluatorConfig(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("make rule-evaluator configmap: %w", err)
	}

	// Upsert rule-evaluator config.
	if err := r.client.Update(ctx, cfg); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, cfg); err != nil {
			return nil, fmt.Errorf("create rule-evaluator config: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("update rule-evaluator config: %w", err)
	}
	return secretData, nil
}

type RuleEvaluatorConfig struct {
	promconfig.Config `yaml:",inline"`

	// Google Cloud configuration. Matches our fork's configuration.
	GoogleCloud GoogleCloudConfig `yaml:"google_cloud,omitempty"`
}

func (config *RuleEvaluatorConfig) UnmarshalYAML(value *yaml.Node) error {
	// See: https://github.com/go-yaml/yaml/issues/125
	// Since the Prometheus configuration uses a custom unmarshaler, it is unable to be
	// unmarshal-ed unless we write our own.
	if err := value.Decode(&config.Config); err != nil {
		return err
	}
	// We must replicate the nested fields.
	googleCloudConfig := struct {
		GoogleCloud GoogleCloudConfig `yaml:"google_cloud,omitempty"`
	}{}
	if err := value.Decode(&googleCloudConfig); err != nil {
		return err
	}
	config.GoogleCloud = googleCloudConfig.GoogleCloud
	return nil
}

type GoogleCloudQueryConfig struct {
	ProjectID       string `yaml:"project_id,omitempty"`
	GeneratorURL    string `yaml:"generator_url,omitempty"`
	CredentialsFile string `yaml:"credentials,omitempty"`
}

// makeRuleEvaluatorConfig creates the config for rule-evaluator.
// This is stored as a Secret rather than a ConfigMap as it could contain
// sensitive configuration information.
func (r *operatorConfigReconciler) makeRuleEvaluatorConfig(ctx context.Context, spec *monitoringv1.RuleEvaluatorSpec) (*corev1.ConfigMap, map[string][]byte, error) {
	amConfigs, secretData, err := r.makeAlertmanagerConfigs(ctx, &spec.Alerting)
	if err != nil {
		return nil, nil, fmt.Errorf("make alertmanager config: %w", err)
	}
	if spec.Credentials != nil {
		p := pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials})
		b, err := getSecretKeyBytes(ctx, r.client, r.opts.PublicNamespace, spec.Credentials)
		if err != nil {
			return nil, nil, fmt.Errorf("get service account credentials: %w", err)
		}
		secretData[p] = b
	}

	// If no explicit project ID is set, use the one provided to the operator.
	// On GKE the rule-evaluator can also auto-detect the cluster's project
	// but this won't work in other Kubernetes environments.
	queryProjectID, _, _ := resolveLabels(r.opts.ProjectID, r.opts.Location, r.opts.Cluster, spec.ExternalLabels)
	if spec.QueryProjectID != "" {
		queryProjectID = spec.QueryProjectID
	}

	cfg := RuleEvaluatorConfig{
		Config: promconfig.Config{
			GlobalConfig: promconfig.GlobalConfig{
				ExternalLabels: labels.FromMap(spec.ExternalLabels),
			},
			AlertingConfig: promconfig.AlertingConfig{
				AlertmanagerConfigs: amConfigs,
			},
			RuleFiles: []string{path.Join(rulesDir, "*.yaml")},
		},
		GoogleCloud: GoogleCloudConfig{
			Query: &GoogleCloudQueryConfig{
				ProjectID:    queryProjectID,
				GeneratorURL: spec.GeneratorURL,
			},
		},
	}
	if spec.Credentials != nil {
		credentialsFile := path.Join(secretsDir, pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: spec.Credentials}))
		cfg.GoogleCloud.Query.CredentialsFile = credentialsFile
		cfg.GoogleCloud.Export = &GoogleCloudExportConfig{
			CredentialsFile: ptr.To(credentialsFile),
		}
	}

	cfgEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal Prometheus config: %w", err)
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
			Annotations: componentAnnotations(),
			Labels:      rulesLabels(),
		},
		Data: make(map[string][]byte),
	}
	for f, b := range data {
		secret.Data[f] = b
	}

	if err := r.client.Update(ctx, secret); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, secret); err != nil {
			return fmt.Errorf("create rule-evaluator secrets: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update rule-evaluator secrets: %w", err)
	}
	return nil
}

// ensureAlertmanagerConfigSecret copies the managed Alertmanager config secret from gmp-public.
func (r *operatorConfigReconciler) ensureAlertmanagerConfigSecret(ctx context.Context, spec *monitoringv1.ManagedAlertmanagerSpec) error {
	logger, _ := logr.FromContext(ctx)
	pubNamespace := r.opts.PublicNamespace

	// This is the default, no-op secret config. If we find a user-defined config,
	// we will overwrite the default data with the user's data.
	// If we don't find a user config, we will still proceed with ensuring this
	// default secret exists (so that the alertmanager pod doesn't crash due to no
	// config found). This flow also handles user deletion/disabling of managed AM.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        AlertmanagerSecretName,
			Namespace:   r.opts.OperatorNamespace,
			Annotations: componentAnnotations(),
			Labels:      alertmanagerLabels(),
		},
		Data: map[string][]byte{AlertmanagerConfigKey: []byte(AlertmanagerNoOpConfig)},
	}

	// Set defaults on public namespace secret.
	var sel = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: AlertmanagerPublicSecretName,
		},
		Key: AlertmanagerPublicSecretKey,
	}
	// Overwrite defaults if specified.
	if spec != nil && spec.ConfigSecret != nil {
		sel.Name = spec.ConfigSecret.Name
		sel.Key = spec.ConfigSecret.Key
	}

	// Try and read the secret for use.
	b, err := getSecretKeyBytes(ctx, r.client, pubNamespace, sel)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// If the config secret is not found, it may have been manually deleted
		// (ie, to disable managed AM), so we will continue with restoring the no-op config
		// so that the managed AM pod doesn't crash loop.
		logger.Info(fmt.Sprintf("alertmanager config secret not found in namespace %s: %s", pubNamespace, err.Error()))
	} else {
		config := alertmanagerConfig{}
		if err := yaml.Unmarshal(b, &config); err != nil {
			return fmt.Errorf("load alertmanager config: %w", err)
		}

		// Only set the value if we need to. This provides a fail-safe in case users change our
		// Alertmanager image with their own. Otherwise, if we always set and they change the image,
		// their Alertmanager will fail unless they have our patch.
		if config.GoogleCloud.ExternalURL != spec.ExternalURL {
			config.GoogleCloud.ExternalURL = spec.ExternalURL

			// NOTE: We can't use direct marshal on alertmanagerConfig because of
			// secret redaction logic, not customizable in old versions.
			b, err = alertmanagerConfigMarshal(b, spec)
			if err != nil {
				return fmt.Errorf("marshal alertmanager config: %w", err)
			}
		}
		secret.Data[AlertmanagerConfigKey] = b
	}

	if err := r.client.Update(ctx, secret); apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, secret); err != nil {
			return fmt.Errorf("create alertmanager config secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("update alertmanager config secret: %w", err)
	}
	return nil
}

func alertmanagerConfigMarshal(userConfig []byte, overrideSpec *monitoringv1.ManagedAlertmanagerSpec) ([]byte, error) {
	inter := map[string]any{}

	if err := yaml.Unmarshal(userConfig, inter); err != nil {
		return nil, err
	}

	delete(inter, "google_cloud")

	inter["google_cloud"] = googleCloudAlertmanagerConfig{ExternalURL: overrideSpec.ExternalURL}

	return yaml.Marshal(inter)
}

type alertmanagerConfig struct {
	alertmanagerconfig.Config `yaml:",inline"`

	// Google Cloud configuration. Matches our fork's configuration.
	GoogleCloud googleCloudAlertmanagerConfig `yaml:"google_cloud,omitempty"`
}

type googleCloudAlertmanagerConfig struct {
	ExternalURL string `yaml:"external_url,omitempty"`
}

func (config *alertmanagerConfig) UnmarshalYAML(value *yaml.Node) error {
	// See: https://github.com/go-yaml/yaml/issues/125
	// Since the Prometheus configuration uses a custom unmarshaler, it is unable to be
	// unmarshal-ed unless we write our own.
	if err := value.Decode(&config.Config); err != nil {
		return err
	}
	// We must replicate the nested fields.
	googleCloudConfig := struct {
		GoogleCloud googleCloudAlertmanagerConfig `yaml:"google_cloud,omitempty"`
	}{}
	if err := value.Decode(&googleCloudConfig); err != nil {
		return err
	}
	config.GoogleCloud = googleCloudConfig.GoogleCloud
	return nil
}

// ensureAlertmanagerStatefulSet configures the managed Alertmanager instance
// to reflect the provided spec.
func (r *operatorConfigReconciler) ensureAlertmanagerStatefulSet(ctx context.Context, spec *monitoringv1.ManagedAlertmanagerSpec) error {
	if spec == nil {
		return nil
	}

	logger, _ := logr.FromContext(ctx)

	var sset appsv1.StatefulSet
	err := r.client.Get(ctx, client.ObjectKey{Namespace: r.opts.OperatorNamespace, Name: NameAlertmanager}, &sset)
	// Some users deliberately not want to run the alertmanager.
	// Only emit a warning but don't cause retries
	// as this logic gets re-triggered anyway if the StatefulSet is created later.
	if apierrors.IsNotFound(err) {
		logger.Error(err, "Alertmanager StatefulSet does not exist")
		return nil
	}
	return err
}

// ensureRuleEvaluatorDeployment reconciles the Deployment for rule-evaluator.
func (r *operatorConfigReconciler) ensureRuleEvaluatorDeployment(ctx context.Context) error {
	logger, _ := logr.FromContext(ctx)

	var deploy appsv1.Deployment
	err := r.client.Get(ctx, client.ObjectKey{Namespace: r.opts.OperatorNamespace, Name: NameRuleEvaluator}, &deploy)
	// Some users deliberately not want to run the rule-evaluator. Only emit a warning but don't cause
	// retries as this logic gets re-triggered anyway if the Deployment is created later.
	if apierrors.IsNotFound(err) {
		logger.Error(err, "rule-evaluator Deployment does not exist")
		return nil
	}
	return err
}

// makeAlertmanagerConfigs creates the alertmanager_config entries as described in
// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alertmanager_config.
func (r *operatorConfigReconciler) makeAlertmanagerConfigs(ctx context.Context, spec *monitoringv1.AlertingSpec) (promconfig.AlertmanagerConfigs, map[string][]byte, error) {
	var (
		err        error
		configs    promconfig.AlertmanagerConfigs
		secretData = make(map[string][]byte)
	)

	amNamespacedName := types.NamespacedName{
		Namespace: r.opts.OperatorNamespace,
		Name:      NameAlertmanager,
	}
	// If the default Alertmanager exists, append it to the list of spec.Alertmanagers.
	var amSvc corev1.Service
	if resourceErr := r.client.Get(ctx, amNamespacedName, &amSvc); resourceErr == nil {
		// Alertmanager service should have one port defined, otherwise ignore.
		if ports := amSvc.Spec.Ports; len(ports) > 0 {
			// Assume first port on service is the correct endpoint.
			port := ports[0].Port
			svcDNSName := fmt.Sprintf("%s.%s:%d", amSvc.Name, amSvc.Namespace, port)
			cfg := promconfig.DefaultAlertmanagerConfig
			cfg.ServiceDiscoveryConfigs = discovery.Configs{
				discovery.StaticConfig{
					&targetgroup.Group{
						Targets: []prommodel.LabelSet{{prommodel.AddressLabel: prommodel.LabelValue(svcDNSName)}},
					},
				},
			}
			configs = append(configs, &cfg)
		}
	}

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
				return nil, nil, fmt.Errorf("invalid timeout: %w", err)
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
				p := pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: c})

				secretData[p] = b
				cfg.HTTPClientConfig.Authorization.CredentialsFile = path.Join(secretsDir, p)
			}
		}
		// TLS config.
		if am.TLS != nil {
			minVersion, err := monitoringv1.TLSVersionFromString(am.TLS.MinVersion)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to convert TLS min version: %w", err)
			}
			maxVersion, err := monitoringv1.TLSVersionFromString(am.TLS.MaxVersion)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to convert TLS min version: %w", err)
			}

			tlsCfg := promcommonconfig.TLSConfig{
				InsecureSkipVerify: am.TLS.InsecureSkipVerify,
				ServerName:         am.TLS.ServerName,
				MinVersion:         minVersion,
				MaxVersion:         maxVersion,
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
				p := pathForSelector(r.opts.PublicNamespace, &monitoringv1.SecretOrConfigMap{Secret: am.TLS.KeySecret})
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
			return nil, nil, fmt.Errorf("cannot build regex from service name %q: %w", am.Name, err)
		}
		cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
			Action:       relabel.Keep,
			SourceLabels: prommodel.LabelNames{"__meta_kubernetes_endpoints_name"},
			Regex:        svcNameRE,
		})
		if am.Port.StrVal != "" {
			re, err := relabel.NewRegexp(am.Port.String())
			if err != nil {
				return nil, nil, fmt.Errorf("cannot build regex from port %q: %w", am.Port, err)
			}
			cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
				Action:       relabel.Keep,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_endpoint_port_name"},
				Regex:        re,
			})
		} else if am.Port.IntVal != 0 {
			// The endpoints object does not provide a meta label for the port number. If the endpoint
			// is backed by a pod we can inspect the pod port number label, but to make it work in general
			// we simply override the port in the address label.
			// If the endpoints has multiple ports, this will create duplicate targets but they will be
			// deduplicated by the discovery engine.
			re, err := relabel.NewRegexp(`(.+):\d+`)
			if err != nil {
				return nil, nil, fmt.Errorf("building address regex failed: %w", err)
			}
			cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__address__"},
				Regex:        re,
				TargetLabel:  "__address__",
				Replacement:  fmt.Sprintf("$1:%d", am.Port.IntVal),
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
func getSecretOrConfigMapBytes(ctx context.Context, c client.Reader, namespace string, scm *monitoringv1.SecretOrConfigMap) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	if secret := scm.Secret; secret != nil {
		b, err = getSecretKeyBytes(ctx, c, namespace, secret)
		if err != nil {
			return b, err
		}
	} else if cm := scm.ConfigMap; cm != nil {
		b, err = getConfigMapKeyBytes(ctx, c, namespace, cm)
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

// getSecretKeyBytes processes the given NamespacedSecretKeySelector and returns the referenced data.
func getSecretKeyBytes(ctx context.Context, c client.Reader, namespace string, sel *corev1.SecretKeySelector) ([]byte, error) {
	var (
		secret = &corev1.Secret{}
		nn     = types.NamespacedName{
			Namespace: namespace,
			Name:      sel.Name,
		}
	)
	err := c.Get(ctx, nn, secret)
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}
	if b, ok := secret.Data[sel.Key]; ok {
		return b, nil
	} else if s, ok := secret.StringData[sel.Key]; ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("key %q in secret %q not found", sel.Key, sel.Name)
}

// getConfigMapKeyBytes processes the given NamespacedConfigMapKeySelector and returns the referenced data.
func getConfigMapKeyBytes(ctx context.Context, c client.Reader, namespace string, sel *corev1.ConfigMapKeySelector) ([]byte, error) {
	var (
		cm = &corev1.ConfigMap{}
		nn = types.NamespacedName{
			Namespace: namespace,
			Name:      sel.Name,
		}
	)
	err := c.Get(ctx, nn, cm)
	if err != nil {
		return nil, fmt.Errorf("get configmap: %w", err)
	}
	if s, ok := cm.Data[sel.Key]; ok {
		return []byte(s), nil
	} else if b, ok := cm.BinaryData[sel.Key]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("key %q in configmap %q not found", sel.Key, sel.Name)
}

// pathForSelector cretes the filepath for the provided NamespacedSecretOrConfigMap.
// This can be used to avoid naming collisions of like-keys across K8s resources.
func pathForSelector(namespace string, scm *monitoringv1.SecretOrConfigMap) string {
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
