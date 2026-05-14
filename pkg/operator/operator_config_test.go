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
	"fmt"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	promforkconfig "github.com/prometheus/prometheus/config"
	gcmconfig "github.com/prometheus/prometheus/google/config"
	"github.com/prometheus/prometheus/google/export"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	if diff := cmp.Diff(expected, out); diff != "" {
		t.Fatalf("unexpected config from marshaling (-want, +got): %s", diff)
	}

	// Check if we can marshal correctly.
	outBytes, err := yaml.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}

	// Prometheus adds some global marshaling, expect those.
	expectedYAML := `global:
    scrape_interval: 1m
    scrape_timeout: 10s
    scrape_protocols:
        - OpenMetricsText1.0.0
        - OpenMetricsText0.0.1
        - PrometheusText0.0.4
    evaluation_interval: 1m
runtime:
    gogc: 75` + configYAML
	if diff := cmp.Diff(expectedYAML, string(outBytes)); diff != "" {
		t.Fatalf("unexpected output of the marshal (-want, +got): %s", diff)
	}

	// Unmarshal back.
	out = promforkconfig.Config{}
	if err := yaml.Unmarshal(outBytes, &out); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(expected, out); diff != "" {
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

			reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)
			operatorConfig, err := reconciler.ensureOperatorConfig(t.Context(), logger, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: DefaultPublicNamespace,
					Name:      NameOperatorConfig,
				},
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
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultPublicNamespace,
					Name:      NameOperatorConfig,
				},
				ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
					ConfigSecret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: AlertmanagerSecretName,
						},
						Key: AlertmanagerConfigKey,
					},
					ExternalURL: tcase.operatorConfigManagedAMExtURL,
				},
			}
			amSecret := &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:        AlertmanagerSecretName,
					Namespace:   DefaultPublicNamespace,
					Annotations: componentAnnotations(),
					Labels:      alertmanagerLabels(),
				},
				Data: map[string][]byte{AlertmanagerConfigKey: []byte(tcase.amConfig)},
			}

			ctx := t.Context()
			fakeClient := newFakeClientBuilder().WithObjects(
				operatorConfig.DeepCopy(),
				amSecret.DeepCopy(),
			)
			kubeClient := fakeClient.Build()
			reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)
			require.NoError(t, reconciler.ensureAlertmanagerConfigSecret(ctx, operatorConfig.ManagedAlertmanager))

			// Get output secret from gmp-system.
			b, err := getSecretKeyBytes(ctx, kubeClient, DefaultOperatorNamespace, operatorConfig.ManagedAlertmanager.ConfigSecret)
			require.NoError(t, err)

			require.Equal(t, tcase.expectedAmConfig, string(b))
		})
	}
}

func TestEnsureAlertmanagerStatefulSet_Storage(t *testing.T) {
	operatorOpts := Options{
		ProjectID:         "test-project",
		Location:          "us-central1-c",
		Cluster:           "test-cluster",
		PublicNamespace:   DefaultPublicNamespace,
		OperatorNamespace: DefaultOperatorNamespace,
	}

	newSset := func() *appsv1.StatefulSet {
		return &appsv1.StatefulSet{
			ObjectMeta: v1.ObjectMeta{
				Namespace: DefaultOperatorNamespace,
				Name:      NameAlertmanager,
			},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name:         alertmanagerDataVolumeName,
								VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
							},
						},
					},
				},
			},
		}
	}

	storageGB := func(gb int) corev1.PersistentVolumeClaimSpec {
		return corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", gb)),
				},
			},
		}
	}

	t.Run("nil storage leaves emptyDir intact", func(t *testing.T) {
		ctx := t.Context()
		sset := newSset()
		kubeClient := newFakeClientBuilder().WithObjects(sset).Build()
		reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)

		require.NoError(t, reconciler.ensureAlertmanagerStatefulSet(ctx, &monitoringv1.ManagedAlertmanagerSpec{}))

		var got appsv1.StatefulSet
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKeyFromObject(sset), &got))
		require.NotNil(t, got.Spec.Template.Spec.Volumes[0].EmptyDir, "emptyDir volume must be preserved when no storage spec is set")
		require.Nil(t, got.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim)

		// No PVC should have been created.
		var pvc corev1.PersistentVolumeClaim
		err := kubeClient.Get(ctx, client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: alertmanagerDataVolumeName}, &pvc)
		require.True(t, apierrors.IsNotFound(err), "PVC must not exist when storage is unset; got err=%v", err)
	})

	t.Run("storage set provisions PVC and swaps volume to PVC reference", func(t *testing.T) {
		ctx := t.Context()
		sset := newSset()
		kubeClient := newFakeClientBuilder().WithObjects(sset).Build()
		reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)

		spec := &monitoringv1.ManagedAlertmanagerSpec{
			Storage: &monitoringv1.AlertmanagerStorageSpec{
				VolumeClaim: monitoringv1.EmbeddedPersistentVolumeClaim{
					EmbeddedObjectMetadata: monitoringv1.EmbeddedObjectMetadata{
						Labels:      map[string]string{"team": "platform"},
						Annotations: map[string]string{"backup.example.com/enabled": "true"},
					},
					Spec: storageGB(5),
				},
			},
		}
		require.NoError(t, reconciler.ensureAlertmanagerStatefulSet(ctx, spec))

		var pvc corev1.PersistentVolumeClaim
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: alertmanagerDataVolumeName}, &pvc))
		require.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes, "access mode must default to ReadWriteOnce when caller omits it")
		require.Equal(t, "5Gi", pvc.Spec.Resources.Requests.Storage().String())
		require.Equal(t, "platform", pvc.Labels["team"])
		require.Equal(t, NameAlertmanager, pvc.Labels[LabelAppName], "operator-owned label must be set")
		require.Equal(t, "true", pvc.Annotations["backup.example.com/enabled"])

		var got appsv1.StatefulSet
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKeyFromObject(sset), &got))
		require.NotNil(t, got.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim, "data volume must now reference the PVC")
		require.Equal(t, alertmanagerDataVolumeName, got.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
		require.Nil(t, got.Spec.Template.Spec.Volumes[0].EmptyDir)
	})

	t.Run("expanding storage request patches the PVC", func(t *testing.T) {
		ctx := t.Context()
		sset := newSset()
		// Pre-bind PVC at 5Gi to simulate a steady-state cluster.
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Namespace: DefaultOperatorNamespace,
				Name:      alertmanagerDataVolumeName,
			},
			Spec: storageGB(5),
		}
		existingPVC.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		kubeClient := newFakeClientBuilder().WithObjects(sset, existingPVC).Build()
		reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)

		spec := &monitoringv1.ManagedAlertmanagerSpec{
			Storage: &monitoringv1.AlertmanagerStorageSpec{
				VolumeClaim: monitoringv1.EmbeddedPersistentVolumeClaim{Spec: storageGB(10)},
			},
		}
		require.NoError(t, reconciler.ensureAlertmanagerStatefulSet(ctx, spec))

		var pvc corev1.PersistentVolumeClaim
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: alertmanagerDataVolumeName}, &pvc))
		require.Equal(t, "10Gi", pvc.Spec.Resources.Requests.Storage().String(), "PVC must be expanded to match requested size")
	})

	t.Run("shrink request is ignored", func(t *testing.T) {
		ctx := t.Context()
		sset := newSset()
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Namespace: DefaultOperatorNamespace,
				Name:      alertmanagerDataVolumeName,
			},
			Spec: storageGB(10),
		}
		existingPVC.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		kubeClient := newFakeClientBuilder().WithObjects(sset, existingPVC).Build()
		reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)

		spec := &monitoringv1.ManagedAlertmanagerSpec{
			Storage: &monitoringv1.AlertmanagerStorageSpec{
				VolumeClaim: monitoringv1.EmbeddedPersistentVolumeClaim{Spec: storageGB(2)},
			},
		}
		require.NoError(t, reconciler.ensureAlertmanagerStatefulSet(ctx, spec))

		var pvc corev1.PersistentVolumeClaim
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: alertmanagerDataVolumeName}, &pvc))
		require.Equal(t, "10Gi", pvc.Spec.Resources.Requests.Storage().String(), "PVC must not shrink; Kubernetes does not allow this")
	})

	t.Run("removing storage spec falls back to emptyDir and leaves PVC in place", func(t *testing.T) {
		ctx := t.Context()
		sset := newSset()
		sset.Spec.Template.Spec.Volumes[0] = corev1.Volume{
			Name: alertmanagerDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: alertmanagerDataVolumeName},
			},
		}
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Namespace: DefaultOperatorNamespace,
				Name:      alertmanagerDataVolumeName,
			},
			Spec: storageGB(5),
		}
		kubeClient := newFakeClientBuilder().WithObjects(sset, existingPVC).Build()
		reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)

		require.NoError(t, reconciler.ensureAlertmanagerStatefulSet(ctx, &monitoringv1.ManagedAlertmanagerSpec{}))

		var got appsv1.StatefulSet
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKeyFromObject(sset), &got))
		require.NotNil(t, got.Spec.Template.Spec.Volumes[0].EmptyDir, "removing storage spec must restore emptyDir")

		var pvc corev1.PersistentVolumeClaim
		require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: DefaultOperatorNamespace, Name: alertmanagerDataVolumeName}, &pvc), "PVC must remain so silences survive accidental config removal")
	})
}
