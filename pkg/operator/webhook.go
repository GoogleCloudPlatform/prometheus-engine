// Copyright 2021 Google LLC
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

	monitoring "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/pkg/errors"
	arv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
)

// ValidatingWebhookConfig returns a config for a webhook that listens for CREATE and UPDATE on GPE resources.
// The resource kind is pulled from the basename of any given endpoint, and must be the plural,
// e.g. `/validate/podmonitorings`.
// The default policy for any failed resource admission is to Ignore.
func ValidatingWebhookConfig(name, namespace string, caBundle []byte, endpoints []string, ors ...metav1.OwnerReference) *arv1.ValidatingWebhookConfiguration {
	var (
		vwc = &arv1.ValidatingWebhookConfiguration{
			// Note: this is a "namespace-less" resource.
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				OwnerReferences: ors,
			},
		}
		policy      = arv1.Ignore
		sideEffects = arv1.SideEffectClassNone
	)

	// Create webhook for each endpoint.
	for _, ep := range endpoints {
		// Init new memory address.
		p := ep
		// Assuming endpoint is of form "/path/to/plural-resourcename",
		// e.g. "/validate/podmonitorings".
		res := path.Base(p)
		vwc.Webhooks = append(vwc.Webhooks,
			arv1.ValidatingWebhook{
				Name: fmt.Sprintf("%s.%s.%s.svc", res, name, namespace),
				ClientConfig: arv1.WebhookClientConfig{
					Service: &arv1.ServiceReference{
						Name:      name,
						Namespace: namespace,
						Path:      &p,
					},
					CABundle: caBundle,
				},
				Rules: []arv1.RuleWithOperations{
					{
						Operations: []arv1.OperationType{arv1.Create, arv1.Update},
						Rule: arv1.Rule{
							APIGroups:   []string{monitoring.GroupName},
							APIVersions: []string{v1alpha1.Version},
							Resources:   []string{res},
						},
					},
				},
				FailurePolicy:           &policy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			})
	}
	return vwc
}

// UpsertValidatingWebhookConfig attempts to create or update a validatingwebhookconfiguration
// resource if one exists.
func UpsertValidatingWebhookConfig(ctx context.Context, api v1.ValidatingWebhookConfigurationInterface, in *arv1.ValidatingWebhookConfiguration) (*arv1.ValidatingWebhookConfiguration, error) {
	out, err := api.Create(ctx, in, metav1.CreateOptions{})
	switch {
	case err == nil:
		return out, err
	case k8serrors.IsAlreadyExists(err) && len(in.Name) > 0:
		vwc, err := api.Get(ctx, in.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "getting existing config")
		}
		in.ResourceVersion = vwc.ResourceVersion
		out, err = api.Update(ctx, in, metav1.UpdateOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "updating existing config")
		}
		return out, nil
	default:
		return nil, errors.Wrapf(err, "creating config")
	}
}
