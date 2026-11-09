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
	"errors"
	"testing"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	promforkconfig "github.com/prometheus/prometheus/config"
	gcmconfig "github.com/prometheus/prometheus/google/config"
	"github.com/prometheus/prometheus/google/export"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPrometheusConfigForRuleEvaluator(t *testing.T) {
	configYAML := `
rule_files:
    - /etc/rules/*.yaml
google_cloud:
    export:
        compression: gzip
        credentials: credentials1.json
    query:
        project_id: abc123
        generator_url: http://example.com/
        credentials: credentials2.json
`
	out := promforkconfig.Config{}
	if err := yaml.Unmarshal([]byte(configYAML), &out); err != nil {
		t.Fatal(err)
	}

	expected := promforkconfig.DefaultConfig
	// Match defaults that are added on unmarshal.
	expected.StorageConfig.TSDBConfig = &promforkconfig.TSDBConfig{Retention: &promforkconfig.DefaultTSDBRetentionConfig}
	expected.RuleFiles = []string{"/etc/rules/*.yaml"}
	expected.GoogleCloud = gcmconfig.GoogleCloudConfig{
		Export: gcmconfig.GoogleCloudExportConfig{
			Compression:     "gzip",
			CredentialsFile: "credentials1.json",
		},
		Query: gcmconfig.GoogleCloudQueryConfig{
			ProjectID:       "abc123",
			GeneratorURL:    "http://example.com/",
			CredentialsFile: "credentials2.json",
		},
	}
	if diff := cmp.Diff(expected, out, cmp.Comparer(labels.Equal), cmpopts.IgnoreUnexported(promforkconfig.Config{})); diff != "" {
		t.Fatalf("unexpected config from marshaling (-want, +got): %s", diff)
	}

	// Check if we can marshal correctly.
	outBytes, err := yaml.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}

	// Prometheus adds some global marshaling, expect those.
	// NOTE: Ruler only uses rules_files and google_cloud.
	expectedYAML := `global:
    scrape_interval: 1m
    scrape_timeout: 10s
    evaluation_interval: 1m
    metric_name_validation_scheme: utf8
    metric_name_escaping_scheme: allow-utf-8
    scrape_native_histograms: false
    extra_scrape_metrics: false
runtime:
    gogc: 75
rule_files:
    - /etc/rules/*.yaml
storage:
    tsdb:
        outofordertimewindow: 0
        retention: {}
otlp:
    translation_strategy: UnderscoreEscapingWithSuffixes
    label_name_underscore_sanitization: true
    label_name_preserve_multiple_underscores: true
google_cloud:
    export:
        compression: gzip
        credentials: credentials1.json
    query:
        project_id: abc123
        generator_url: http://example.com/
        credentials: credentials2.json
`
	if diff := cmp.Diff(expectedYAML, string(outBytes)); diff != "" {
		t.Fatalf("unexpected output of the marshal (-want, +got): %s", diff)
	}

	// Unmarshal back.
	out = promforkconfig.Config{}
	if err := yaml.Unmarshal(outBytes, &out); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(expected, out, cmp.Comparer(labels.Equal), cmpopts.IgnoreUnexported(promforkconfig.Config{})); diff != "" {
		t.Fatalf("unexpected config after marshaling (-want, +got): %s", diff)
	}
}

func TestEnsureOperatorConfig(t *testing.T) {
	logger := logr.Discard()
	operatorOpts := Options{
		ProjectID: "test-project",
		Location:  "us-central1-c",
		Cluster:   "test-cluster",
	}
	defaultObjectMeta := v1.ObjectMeta{
		Namespace: DefaultPublicNamespace,
		Name:      NameOperatorConfig,
	}
	defaultOperatorConfig := monitoringv1.OperatorConfig{
		ObjectMeta: defaultObjectMeta,
		Collection: monitoringv1.CollectionSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
			},
		},
		Rules: monitoringv1.RuleEvaluatorSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
			},
		},
	}
	defaultOperatorConfigWithExtraLabels := monitoringv1.OperatorConfig{
		ObjectMeta: defaultObjectMeta,
		Collection: monitoringv1.CollectionSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
				"foo":               "bar",
			},
		},
		Rules: monitoringv1.RuleEvaluatorSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
				"abc":               "xyz",
			},
		},
	}

	testCases := []struct {
		desc     string
		existing *monitoringv1.OperatorConfig
		expected *monitoringv1.OperatorConfig
	}{
		{
			desc: "empty",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
			},
			expected: &defaultOperatorConfig,
		},
		{
			desc:     "does not exist",
			existing: nil,
			expected: &defaultOperatorConfig,
		},
		{
			desc:     "matches config",
			existing: &defaultOperatorConfigWithExtraLabels,
			expected: &defaultOperatorConfigWithExtraLabels,
		},
		{
			desc: "missing project",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyLocation: operatorOpts.Location,
						export.KeyCluster:  operatorOpts.Cluster,
						"foo":              "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyLocation: operatorOpts.Location,
						export.KeyCluster:  operatorOpts.Cluster,
						"abc":              "xyz",
					},
				},
			},
			expected: &defaultOperatorConfigWithExtraLabels,
		},
		{
			desc: "override project",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"foo":               "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"abc":               "xyz",
					},
				},
			},
			expected: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"foo":               "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"abc":               "xyz",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			clientBuilder := newFakeClientBuilder()
			if tc.existing != nil {
				clientBuilder = clientBuilder.WithObjects(tc.existing.DeepCopy())
			}
			kubeClient := clientBuilder.Build()

			reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts, false)
			operatorConfig, err := reconciler.ensureOperatorConfig(t.Context(), logger, reconcile.Request{
				Namespace: DefaultPublicNamespace,
				Name:      NameOperatorConfig,
			})
			if err != nil {
				t.Fatalf("ensure operator config: %s", err)
			}

			// Normalize before comparisons.
			operatorConfig.ResourceVersion = ""

			if diff := cmp.Diff(operatorConfig, tc.expected); diff != "" {
				t.Fatalf("unexpected operator config: %s", diff)
			}

			// Make sure the object is updated in case of defaulting.
			if tc.existing != nil {
				existingLatest := monitoringv1.OperatorConfig{}
				if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(tc.existing), &existingLatest); err != nil {
					t.Fatalf("get operator config: %s", err)
				}

				// Normalize before comparisons.
				existingLatest.ResourceVersion = ""

				if diff := cmp.Diff(&existingLatest, tc.expected); diff != "" {
					t.Fatalf("operator config expected update: %s", diff)
				}
			}
		})
	}
}

// Regression against https://github.com/GoogleCloudPlatform/prometheus-engine/issues/1550.
func TestEnsureAlertmanagerConfigSecret(t *testing.T) {
	operatorOpts := Options{
		ProjectID:         "test-project",
		Location:          "us-central1-c",
		Cluster:           "test-cluster",
		PublicNamespace:   DefaultPublicNamespace,
		OperatorNamespace: DefaultOperatorNamespace,
	}
	for _, tcase := range []struct {
		name                          string
		operatorConfigManagedAMExtURL string
		amConfig                      string

		expectedAmConfig string
	}{
		{
			name: "with secret; no external url",
			amConfig: `
route:
  receiver: "slack"
receivers:
- name: "slack"
  slack_configs:
  - channel: '#some_channel'
    api_url: https://slack.com/api/chat.postMessage
    http_config:
      authorization:
        type: 'Bearer'
        credentials: 'SUPER IMPORTANT SECRET'
`,
			expectedAmConfig: `
route:
  receiver: "slack"
receivers:
- name: "slack"
  slack_configs:
  - channel: '#some_channel'
    api_url: https://slack.com/api/chat.postMessage
    http_config:
      authorization:
        type: 'Bearer'
        credentials: 'SUPER IMPORTANT SECRET'
`,
		},
		{
			name:                          "with secret; set external url with the same values",
			operatorConfigManagedAMExtURL: "https://alertmanager.mycompany.com/",
			amConfig: `
google_cloud:
  # Must be exactly the same value as in OperatorConfig.managedAlertmanager.externalURL,
  # so buggy re-encoding is skipped until the 0.14.3 bugfix is rolled.
  external_url: "https://alertmanager.mycompany.com/"
route:
  receiver: "slack"
receivers:
- name: "slack"
  slack_configs:
  - channel: '#some_channel'
    api_url: https://slack.com/api/chat.postMessage
    http_config:
      authorization:
        type: 'Bearer'
        credentials: 'SUPER IMPORTANT SECRET'
`,
			expectedAmConfig: `
google_cloud:
  # Must be exactly the same value as in OperatorConfig.managedAlertmanager.externalURL,
  # so buggy re-encoding is skipped until the 0.14.3 bugfix is rolled.
  external_url: "https://alertmanager.mycompany.com/"
route:
  receiver: "slack"
receivers:
- name: "slack"
  slack_configs:
  - channel: '#some_channel'
    api_url: https://slack.com/api/chat.postMessage
    http_config:
      authorization:
        type: 'Bearer'
        credentials: 'SUPER IMPORTANT SECRET'
`,
		},
		{
			name:                          "with secret; external url set in operator config, but not in am yaml",
			operatorConfigManagedAMExtURL: "https://alertmanager.mycompany.com/",
			amConfig: `
route:
  receiver: "slack"
receivers:
- name: "slack"
  slack_configs:
  - channel: '#some_channel'
    api_url: https://slack.com/api/chat.postMessage
    http_config:
      authorization:
        type: 'Bearer'
        credentials: 'SUPER IMPORTANT SECRET'
`,
			expectedAmConfig: `google_cloud:
    external_url: https://alertmanager.mycompany.com/
receivers:
    - name: slack
      slack_configs:
        - api_url: https://slack.com/api/chat.postMessage
          channel: '#some_channel'
          http_config:
            authorization:
                credentials: SUPER IMPORTANT SECRET
                type: Bearer
route:
    receiver: slack
`,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			operatorConfig := &monitoringv1.OperatorConfig{
				Namespace: DefaultPublicNamespace,
				Name:      NameOperatorConfig,
				ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
					ConfigSecret: &corev1.SecretKeySelector{
						Name: AlertmanagerSecretName,
						Key:  AlertmanagerConfigKey,
					},
					ExternalURL: tcase.operatorConfigManagedAMExtURL,
				},
			}
			amSecret := &corev1.Secret{
				Name:        AlertmanagerSecretName,
				Namespace:   DefaultPublicNamespace,
				Annotations: componentAnnotations(),
				Labels:      alertmanagerLabels(),
				Data:        map[string][]byte{AlertmanagerConfigKey: []byte(tcase.amConfig)},
			}

			ctx := t.Context()
			fakeClient := newFakeClientBuilder().WithObjects(
				operatorConfig.DeepCopy(),
				amSecret.DeepCopy(),
			)
			kubeClient := fakeClient.Build()
			reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts, false)
			require.NoError(t, reconciler.ensureAlertmanagerConfigSecret(ctx, operatorConfig.ManagedAlertmanager))

			// Get output secret from gmp-system.
			b, err := getSecretKeyBytes(ctx, kubeClient, DefaultOperatorNamespace, operatorConfig.ManagedAlertmanager.ConfigSecret)
			require.NoError(t, err)

			require.Equal(t, tcase.expectedAmConfig, string(b))
		})
	}
}

func TestOperatorConfigReconcileVPAUnavailable(t *testing.T) {
	operatorOpts := Options{
		ProjectID:         "test-project",
		Location:          "us-central1-c",
		Cluster:           "test-cluster",
		PublicNamespace:   DefaultPublicNamespace,
		OperatorNamespace: DefaultOperatorNamespace,
	}
	req := reconcile.Request{
		Namespace: DefaultPublicNamespace,
		Name:      NameOperatorConfig,
	}
	newVPAConfig := func(uid types.UID, generation int64) *monitoringv1.OperatorConfig {
		return &monitoringv1.OperatorConfig{
			Namespace:  DefaultPublicNamespace,
			Name:       NameOperatorConfig,
			UID:        uid,
			Generation: generation,
			Scaling: monitoringv1.ScalingSpec{
				VPA: monitoringv1.VPASpec{
					Enabled: true,
				},
			},
		}
	}
	newReconciler := func(c client.Client, vpaAvailable bool) (*operatorConfigReconciler, *testingclock.FakeClock) {
		fakeClock := testingclock.NewFakeClock(time.Unix(1700000000, 0))
		r := newOperatorConfigReconciler(c, operatorOpts, vpaAvailable)
		r.clock = fakeClock
		return r, fakeClock
	}
	listEvents := func(t *testing.T, c client.Client, namespace string) []eventsv1.Event {
		t.Helper()
		var events eventsv1.EventList
		require.NoError(t, c.List(t.Context(), &events, client.InNamespace(namespace)))
		return events.Items
	}
	// reconcileAndCount reconciles after advancing the clock and returns the number of
	// events in gmp-system and gmp-public.
	reconcileAndCount := func(t *testing.T, r *operatorConfigReconciler, fakeClock *testingclock.FakeClock, c client.Client) (int, int) {
		t.Helper()
		fakeClock.Step(time.Second)
		_, err := r.Reconcile(t.Context(), req)
		require.NoError(t, err)
		return len(listEvents(t, c, DefaultOperatorNamespace)), len(listEvents(t, c, DefaultPublicNamespace))
	}

	t.Run("emits warning events and continues reconciling", func(t *testing.T) {
		ctx := t.Context()
		kubeClient := newFakeClientBuilder().WithObjects(newVPAConfig("uid-1", 1)).Build()
		reconciler, _ := newReconciler(kubeClient, false)

		_, err := reconciler.Reconcile(ctx, req)
		require.NoError(t, err)

		// Non-VPA subresources must still reconcile normally.
		var ruleEvalCM corev1.ConfigMap
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{
			Namespace: DefaultOperatorNamespace,
			Name:      NameRuleEvaluator,
		}, &ruleEvalCM))
		require.NotEmpty(t, ruleEvalCM.Data[configFilename])

		var amSecret corev1.Secret
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{
			Namespace: DefaultOperatorNamespace,
			Name:      AlertmanagerSecretName,
		}, &amSecret))

		// Verify workload Warning Event in gmp-system.
		sysEvents := listEvents(t, kubeClient, DefaultOperatorNamespace)
		require.Len(t, sysEvents, 1)
		sysEv := sysEvents[0]
		require.Equal(t, corev1.EventTypeWarning, sysEv.Type)
		require.Equal(t, "Ignored", sysEv.Action)
		require.Equal(t, reasonVPANotAvailable, sysEv.Reason)
		require.Equal(t, reportingController, sysEv.ReportingController)
		require.Equal(t, NameOperator, sysEv.ReportingInstance)
		require.False(t, sysEv.EventTime.IsZero())
		require.Equal(t, "Deployment", sysEv.Regarding.Kind)
		require.Equal(t, DefaultOperatorNamespace, sysEv.Regarding.Namespace)
		require.Equal(t, NameOperator, sysEv.Regarding.Name)
		require.NotNil(t, sysEv.Related)
		require.Equal(t, "OperatorConfig", sysEv.Related.Kind)
		require.Equal(t, DefaultPublicNamespace, sysEv.Related.Namespace)
		require.Equal(t, NameOperatorConfig, sysEv.Related.Name)
		require.Equal(t, types.UID("uid-1"), sysEv.Related.UID)
		require.Equal(t, "OperatorConfig gmp-public/config: vertical pod autoscaling is not available - install vpa support and restart the operator", sysEv.Note)

		// Verify resource Warning Event in gmp-public.
		pubEvents := listEvents(t, kubeClient, DefaultPublicNamespace)
		require.Len(t, pubEvents, 1)
		pubEv := pubEvents[0]
		require.Equal(t, corev1.EventTypeWarning, pubEv.Type)
		require.Equal(t, "Ignored", pubEv.Action)
		require.Equal(t, reasonVPANotAvailable, pubEv.Reason)
		require.Equal(t, reportingController, pubEv.ReportingController)
		require.Equal(t, NameOperator, pubEv.ReportingInstance)
		require.False(t, pubEv.EventTime.IsZero())
		require.Equal(t, "OperatorConfig", pubEv.Regarding.Kind)
		require.Equal(t, DefaultPublicNamespace, pubEv.Regarding.Namespace)
		require.Equal(t, NameOperatorConfig, pubEv.Regarding.Name)
		require.Equal(t, types.UID("uid-1"), pubEv.Regarding.UID)
		require.Equal(t, "vertical pod autoscaling is not available - install vpa support and restart the operator", pubEv.Note)
	})

	t.Run("emits warning events once per generation", func(t *testing.T) {
		ctx := t.Context()
		kubeClient := newFakeClientBuilder().WithObjects(newVPAConfig("uid-1", 1)).Build()
		reconciler, fakeClock := newReconciler(kubeClient, false)

		sysCount, pubCount := reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 1, sysCount)
		require.Equal(t, 1, pubCount)

		// Re-reconciling at the same (UID, Generation) must not emit duplicate events.
		sysCount, pubCount = reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 1, sysCount)
		require.Equal(t, 1, pubCount)

		// Bumping Generation emits a new event pair.
		var latest monitoringv1.OperatorConfig
		require.NoError(t, kubeClient.Get(ctx, req.NamespacedName, &latest))
		latest.Generation = 2
		require.NoError(t, kubeClient.Update(ctx, &latest))

		sysCount, pubCount = reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 2, sysCount)
		require.Equal(t, 2, pubCount)

		// Disabling scaling.vpa.enabled emits nothing.
		require.NoError(t, kubeClient.Get(ctx, req.NamespacedName, &latest))
		latest.Scaling.VPA.Enabled = false
		latest.Generation = 3
		require.NoError(t, kubeClient.Update(ctx, &latest))

		sysCount, pubCount = reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 2, sysCount)
		require.Equal(t, 2, pubCount)

		// Re-enabling scaling.vpa.enabled emits a new event pair.
		require.NoError(t, kubeClient.Get(ctx, req.NamespacedName, &latest))
		latest.Scaling.VPA.Enabled = true
		latest.Generation = 4
		require.NoError(t, kubeClient.Update(ctx, &latest))

		sysCount, pubCount = reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 3, sysCount)
		require.Equal(t, 3, pubCount)
	})

	t.Run("emits warning events again when OperatorConfig is recreated with the same generation", func(t *testing.T) {
		ctx := t.Context()
		kubeClient := newFakeClientBuilder().WithObjects(newVPAConfig("uid-1", 1)).Build()
		reconciler, fakeClock := newReconciler(kubeClient, false)

		sysCount, pubCount := reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 1, sysCount)
		require.Equal(t, 1, pubCount)

		// Deleting and recreating OperatorConfig yields a new UID at the same Generation.
		var latest monitoringv1.OperatorConfig
		require.NoError(t, kubeClient.Get(ctx, req.NamespacedName, &latest))
		require.NoError(t, kubeClient.Delete(ctx, &latest))
		require.NoError(t, kubeClient.Create(ctx, newVPAConfig("uid-2", 1)))

		sysCount, pubCount = reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 2, sysCount)
		require.Equal(t, 2, pubCount)
	})

	t.Run("event creation failures do not fail or retry reconciliation", func(t *testing.T) {
		ctx := t.Context()
		eventAttempts := map[string]int{}
		kubeClient := newFakeClientBuilder().
			WithObjects(newVPAConfig("uid-1", 1)).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if ev, ok := obj.(*eventsv1.Event); ok {
						eventAttempts[ev.Namespace]++
						if ev.Namespace == DefaultOperatorNamespace {
							return apierrors.NewForbidden(schema.GroupResource{Group: eventsv1.GroupName, Resource: "events"}, ev.Name, errors.New("events forbidden"))
						}
						return errors.New("transient apiserver error")
					}
					return c.Create(ctx, obj, opts...)
				},
			}).
			Build()
		reconciler, fakeClock := newReconciler(kubeClient, false)

		for range 2 {
			fakeClock.Step(time.Second)
			result, err := reconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, result)
		}
		// Both event creations are attempted once for the generation and never retried.
		require.Equal(t, map[string]int{DefaultOperatorNamespace: 1, DefaultPublicNamespace: 1}, eventAttempts)
		require.Empty(t, listEvents(t, kubeClient, DefaultOperatorNamespace))
		require.Empty(t, listEvents(t, kubeClient, DefaultPublicNamespace))

		// The rule-evaluator config is still reconciled despite the event failures.
		var ruleEvalCM corev1.ConfigMap
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{
			Namespace: DefaultOperatorNamespace,
			Name:      NameRuleEvaluator,
		}, &ruleEvalCM))
	})

	t.Run("emits no warning events when VPA is available", func(t *testing.T) {
		kubeClient := newFakeClientBuilder().WithObjects(newVPAConfig("uid-1", 1)).Build()
		reconciler, fakeClock := newReconciler(kubeClient, true)

		sysCount, pubCount := reconcileAndCount(t, reconciler, fakeClock, kubeClient)
		require.Equal(t, 0, sysCount)
		require.Equal(t, 0, pubCount)
	})
}
