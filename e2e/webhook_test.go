// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	arv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// webhook configurations.
func TestWebhookCABundleInjection(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)

	var (
		whConfigName = fmt.Sprintf("gmp-operator.%s.monitoring.googleapis.com", tctx.namespace)
		policy       = arv1.Ignore // Prevent collisions with other test or real usage
		sideEffects  = arv1.SideEffectClassNone
		url          = "https://0.1.2.3/"
	)

	t.Run("validating webhook", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		// Create webhook configs. The operator must populate their caBundles.
		vwc := &arv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: whConfigName,
			},
			Webhooks: []arv1.ValidatingWebhook{
				{
					Name:                    "wh1.monitoring.googleapis.com",
					ClientConfig:            arv1.WebhookClientConfig{URL: &url},
					FailurePolicy:           &policy,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}, {
					Name:                    "wh2.monitoring.googleapis.com",
					ClientConfig:            arv1.WebhookClientConfig{URL: &url},
					FailurePolicy:           &policy,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				},
			},
		}
		if err := tctx.Client().Create(ctx, vwc); err != nil {
			t.Fatal(err)
		}

		// Wait for caBundle injection.
		err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			if err := tctx.Client().Get(ctx, client.ObjectKeyFromObject(vwc), vwc); err != nil {
				return false, fmt.Errorf("get validatingwebhook configuration: %w", err)
			}
			if len(vwc.Webhooks) != 2 {
				return false, fmt.Errorf("expected 2 webhooks but got %d", len(vwc.Webhooks))
			}
			for _, wh := range vwc.Webhooks {
				if len(wh.ClientConfig.CABundle) == 0 {
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("waiting for ValidatingWebhook CA bundle failed: %s", err)
		}
	}))

	t.Run("mutating webhook", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		// Create webhook configs. The operator must populate their caBundles.
		mwc := &arv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: whConfigName,
			},
			Webhooks: []arv1.MutatingWebhook{
				{
					Name:                    "wh1.monitoring.googleapis.com",
					ClientConfig:            arv1.WebhookClientConfig{URL: &url},
					FailurePolicy:           &policy,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}, {
					Name:                    "wh2.monitoring.googleapis.com",
					ClientConfig:            arv1.WebhookClientConfig{URL: &url},
					FailurePolicy:           &policy,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				},
			},
		}
		if err := tctx.Client().Create(ctx, mwc); err != nil {
			t.Fatal(err)
		}

		// Wait for caBundle injection.
		err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			if err := tctx.Client().Get(ctx, client.ObjectKeyFromObject(mwc), mwc); err != nil {
				return false, fmt.Errorf("get mutatingwebhook configuration: %w", err)
			}
			if len(mwc.Webhooks) != 2 {
				return false, fmt.Errorf("expected 2 webhooks but got %d", len(mwc.Webhooks))
			}
			for _, wh := range mwc.Webhooks {
				if len(wh.ClientConfig.CABundle) == 0 {
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("waiting for MutatingWebhook CA bundle failed: %s", err)
		}
	}))
}
