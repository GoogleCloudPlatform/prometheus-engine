// Copyright 2024 Google LLC
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

// Package operator contains the Prometheus
package operator

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	arv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func readKeyAndCertFiles(dir string, t *testing.T) ([]byte, []byte) {
	outCert, err := os.ReadFile(path.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatalf("error reading from cert file: %v", err)
	}
	outKey, err := os.ReadFile(path.Join(dir, "tls.key"))
	if err != nil {
		t.Fatalf("error reading from key file: %v", err)
	}
	return outCert, outKey
}

func TestEnsureCertsExplicit(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_ensure_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for _, tc := range []struct {
		desc         string
		tlsKey       string
		tlsCert      string
		caCert       string
		namespace    string
		expectCert   string
		expectKey    string
		expectCaCert string
		expectErr    bool
	}{
		{
			desc:         "input key/cert/ca",
			tlsKey:       "a2V5",
			tlsCert:      "Y2VydA==",
			caCert:       "Y2FjZXJ0",
			namespace:    "test-ns",
			expectCert:   "cert",
			expectKey:    "key",
			expectCaCert: "cacert",
			expectErr:    false,
		},
		{
			desc:       "cert/key and no CA",
			tlsKey:     "a2V5",
			tlsCert:    "Y2VydA==",
			namespace:  "test-ns",
			expectCert: "cert",
			expectKey:  "key",
			expectErr:  false,
		},
		{
			desc:      "bad cert",
			tlsCert:   "not a cert",
			tlsKey:    "not a key",
			caCert:    "not a CA",
			namespace: "test-ns",
			expectErr: true,
		},
		{
			desc:      "cert and no key/ca",
			tlsCert:   "cert",
			namespace: "test-ns",
			expectErr: true,
		},
		{
			desc:      "key and no cert/ca",
			tlsKey:    "key",
			namespace: "test-ns",
			expectErr: true,
		},
		{
			desc:      "ca and no cert/key",
			caCert:    "CAcert",
			namespace: "test-ns",
			expectErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			caBundle, err := ensureCerts(tc.namespace, dir, tc.tlsCert, tc.tlsKey, tc.caCert)
			if (err == nil && tc.expectErr) || (err != nil && !tc.expectErr) {
				t.Fatalf("want err: %v; got %v", tc.expectErr, err)
			}
			if err != nil && tc.expectErr {
				return
			}
			// Test outputted files.
			outCert, outKey := readKeyAndCertFiles(dir, t)
			if string(outCert) != tc.expectCert {
				t.Errorf("want cert: %v; got %v", tc.tlsCert, string(outCert))
			}
			if string(outKey) != tc.expectKey {
				t.Errorf("want key: %v; got %v", tc.tlsKey, string(outKey))
			}
			if string(caBundle) != tc.expectCaCert {
				t.Errorf("want ca: %v; got %v", string(caBundle), string(outCert))
			}
		})
	}
}

func TestEnsureCertsSelfSigned(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_ensure_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	t.Run("self generate keys/cert", func(t *testing.T) {
		caBundle, err := ensureCerts("test-ns", dir, "", "", "")
		if err != nil {
			t.Fatalf("ensure certs: %v", err)
		}
		// Cert and key will be randomly generated, check if they exisits.
		outCert, outKey := readKeyAndCertFiles(dir, t)
		if len(outKey) == 0 {
			t.Errorf("expected generated key but was empty")
		}
		if len(outCert) == 0 {
			t.Errorf("expected generated cert but was empty")
		}
		// self-generate case, ca is equal to crt.
		if string(outCert) != string(caBundle) {
			t.Errorf("want ca: %v; got %v", string(outCert), string(caBundle))
		}
	})
}

func TestWebhookCABundleUpdate(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		desc              string
		dir               string
		opts              Options
		runFunc           func(kubeClient client.Client, objs []runtime.Object)
		managedCABundle   []byte
		unmanagedCABundle []byte
	}{
		{
			desc: "nil CABundle",
			dir:  "test_webhook_ca_bundle_nil",
			opts: Options{
				OperatorNamespace: DefaultOperatorNamespace,
				TLSKey:            "a2V5",
				TLSCert:           "Y2VydA==",
				CACert:            "Y2FjZXJ0",
			},
			runFunc: func(kubeClient client.Client, objs []runtime.Object) {
				for i := range objs {
					switch obj := objs[i].(type) {
					case *arv1.MutatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromMutatingWebhook(obj) {
							clientConfig.CABundle = nil
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					case *arv1.ValidatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromValidatingWebhook(obj) {
							clientConfig.CABundle = nil
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					}
				}
			},
		},
		{
			desc: "update CABundle",
			dir:  "test_webhook_ca_bundle_update",
			opts: Options{
				OperatorNamespace: DefaultOperatorNamespace,
				TLSKey:            "a2V5",
				TLSCert:           "Y2VydA==",
				CACert:            "Y2FjZXJ0",
			},
			runFunc: func(kubeClient client.Client, objs []runtime.Object) {
				for i := range objs {
					switch obj := objs[i].(type) {
					case *arv1.MutatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromMutatingWebhook(obj) {
							clientConfig.CABundle = []byte("abc123")
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					case *arv1.ValidatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromValidatingWebhook(obj) {
							clientConfig.CABundle = []byte("abc123")
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					}
				}
			},
			unmanagedCABundle: []byte("abc123"),
		},
		{
			desc: "recreate webhooks",
			dir:  "test_webhook_ca_bundle_recreate",
			opts: Options{
				OperatorNamespace: DefaultOperatorNamespace,
				TLSKey:            "a2V5",
				TLSCert:           "Y2VydA==",
				CACert:            "Y2FjZXJ0",
			},
			runFunc: func(kubeClient client.Client, objs []runtime.Object) {
				for i := range objs {
					switch obj := objs[i].(type) {
					case *arv1.MutatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromMutatingWebhook(obj) {
							clientConfig.CABundle = nil
						}
						if err := kubeClient.Delete(ctx, obj); err != nil {
							t.Fatal(err)
						}
						obj.ResourceVersion = ""
						if err := kubeClient.Create(ctx, obj); err != nil {
							t.Fatal(err)
						}
					case *arv1.ValidatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromValidatingWebhook(obj) {
							clientConfig.CABundle = nil
						}
						if err := kubeClient.Delete(ctx, obj); err != nil {
							t.Fatal(err)
						}
						obj.ResourceVersion = ""
						if err := kubeClient.Create(ctx, obj); err != nil {
							t.Fatal(err)
						}
					}
				}
			},
		},
		{
			desc: "ignore CABundle",
			dir:  "test_webhook_ca_bundle_ignore",
			opts: Options{
				OperatorNamespace: DefaultOperatorNamespace,
				TLSKey:            "a2V5",
				TLSCert:           "Y2VydA==",
			},
			runFunc: func(kubeClient client.Client, objs []runtime.Object) {
				caBundle := []byte("abc123")
				for i := range objs {
					switch obj := objs[i].(type) {
					case *arv1.MutatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromMutatingWebhook(obj) {
							clientConfig.CABundle = caBundle
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					case *arv1.ValidatingWebhookConfiguration:
						if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
							t.Fatal(err)
						}
						for _, clientConfig := range clientConfigsFromValidatingWebhook(obj) {
							clientConfig.CABundle = caBundle
						}
						if err := kubeClient.Update(ctx, obj); err != nil {
							t.Fatal(err)
						}
					}
				}
			},
			managedCABundle:   []byte("abc123"),
			unmanagedCABundle: []byte("abc123"),
		},
	}

	for i := range testCases {
		tc := &testCases[i]
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			opts := tc.opts
			name := webhookName(opts.OperatorNamespace)
			nameInvalid := webhookName("invalid")

			objs := []runtime.Object{
				&arv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
					Webhooks: []arv1.MutatingWebhook{
						{
							Name:         "webhook-1",
							ClientConfig: arv1.WebhookClientConfig{},
						},
						{
							Name:         "webhook-2",
							ClientConfig: arv1.WebhookClientConfig{},
						},
					},
				},
				&arv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: nameInvalid,
					},
					Webhooks: []arv1.MutatingWebhook{
						{
							Name:         "webhook-1",
							ClientConfig: arv1.WebhookClientConfig{},
						},
						{
							Name:         "webhook-2",
							ClientConfig: arv1.WebhookClientConfig{},
						},
					},
				},
				&arv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
					Webhooks: []arv1.ValidatingWebhook{
						{
							Name:         "webhook-1",
							ClientConfig: arv1.WebhookClientConfig{},
						},
						{
							Name:         "webhook-2",
							ClientConfig: arv1.WebhookClientConfig{},
						},
					},
				},
				&arv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: nameInvalid,
					},
					Webhooks: []arv1.ValidatingWebhook{
						{
							Name:         "webhook-1",
							ClientConfig: arv1.WebhookClientConfig{},
						},
						{
							Name:         "webhook-2",
							ClientConfig: arv1.WebhookClientConfig{},
						},
					},
				},
			}
			kubeClient := fake.NewFakeClient(objs...)

			dir, err := os.MkdirTemp("", tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			webhookServer := &webhook.DefaultServer{
				Options: webhook.Options{
					CertDir: dir,
				},
			}

			if err := setupAdmissionWebhooks(ctx, logr.Discard(), kubeClient, webhookServer, &opts, false); err != nil {
				t.Fatal(err)
			}

			validateCABundles := func(managedCABundle, unmanagedCABundle []byte) {
				if err := wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
					mutatingWebhook := arv1.MutatingWebhookConfiguration{
						ObjectMeta: metav1.ObjectMeta{
							Name: name,
						},
					}
					if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&mutatingWebhook), &mutatingWebhook); err != nil {
						t.Log(err)
						return false, nil
					}
					if !slices.Equal(managedCABundle, getCABundle(clientConfigsFromMutatingWebhook(&mutatingWebhook))) {
						return false, nil
					}

					validatingWebhook := arv1.ValidatingWebhookConfiguration{
						ObjectMeta: metav1.ObjectMeta{
							Name: name,
						},
					}
					if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&validatingWebhook), &validatingWebhook); err != nil {
						t.Log(err)
						return false, nil
					}
					if !slices.Equal(managedCABundle, getCABundle(clientConfigsFromValidatingWebhook(&validatingWebhook))) {
						return false, nil
					}

					mutatingWebhook = arv1.MutatingWebhookConfiguration{
						ObjectMeta: metav1.ObjectMeta{
							Name: nameInvalid,
						},
					}
					if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&mutatingWebhook), &mutatingWebhook); err != nil {
						t.Log(err)
						return false, nil
					}
					if !slices.Equal(unmanagedCABundle, getCABundle(clientConfigsFromMutatingWebhook(&mutatingWebhook))) {
						return false, fmt.Errorf("webhook %q should not be updated", client.ObjectKeyFromObject(&mutatingWebhook))
					}

					validatingWebhook = arv1.ValidatingWebhookConfiguration{
						ObjectMeta: metav1.ObjectMeta{
							Name: nameInvalid,
						},
					}
					if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&validatingWebhook), &validatingWebhook); err != nil {
						t.Log(err)
						return false, nil
					}
					if !slices.Equal(unmanagedCABundle, getCABundle(clientConfigsFromValidatingWebhook(&validatingWebhook))) {
						return false, fmt.Errorf("webhook %q should not be updated", client.ObjectKeyFromObject(&validatingWebhook))
					}

					return true, nil
				}); err != nil {
					t.Fatalf("CABundle not written: %s", err)
				}
			}

			expectedCABundle, err := base64.StdEncoding.DecodeString(opts.CACert)
			if err != nil {
				t.Fatal(err)
			}
			validateCABundles(expectedCABundle, []byte{})

			if tc.managedCABundle == nil {
				tc.managedCABundle = expectedCABundle
			}
			if tc.unmanagedCABundle == nil {
				tc.unmanagedCABundle = []byte{}
			}

			tc.runFunc(kubeClient, objs)
			validateCABundles(tc.managedCABundle, tc.unmanagedCABundle)
		})
	}
}

func clientConfigsFromMutatingWebhook(webhookConfig *arv1.MutatingWebhookConfiguration) []*arv1.WebhookClientConfig {
	var clientConfigs []*arv1.WebhookClientConfig
	for i := range webhookConfig.Webhooks {
		clientConfigs = append(clientConfigs, &webhookConfig.Webhooks[i].ClientConfig)
	}
	return clientConfigs
}

func clientConfigsFromValidatingWebhook(webhookConfig *arv1.ValidatingWebhookConfiguration) []*arv1.WebhookClientConfig {
	var clientConfigs []*arv1.WebhookClientConfig
	for i := range webhookConfig.Webhooks {
		clientConfigs = append(clientConfigs, &webhookConfig.Webhooks[i].ClientConfig)
	}
	return clientConfigs
}

func getCABundle(clientConfigs []*arv1.WebhookClientConfig) []byte {
	if len(clientConfigs) == 0 {
		return nil
	}
	var caBundle []byte
	for _, clientConfig := range clientConfigs {
		if caBundle == nil {
			caBundle = clientConfig.CABundle
		}
		if !slices.Equal(caBundle, clientConfig.CABundle) {
			return nil
		}
	}
	return caBundle
}
