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

	"github.com/pkg/errors"
	arv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
)

func validatePath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/validate/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

// validatingWebhookConfig returns a config for a webhook that listens for
// CREATE and UPDATE on provided resources.
// The default policy for any failed resource admission is to Ignore.
func validatingWebhookConfig(name, namespace string, port int32, caBundle []byte, gvrs []metav1.GroupVersionResource, ors ...metav1.OwnerReference) *arv1.ValidatingWebhookConfiguration {
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

	// Add an entry for each validator.
	for _, gvr := range gvrs {
		path := validatePath(gvr)

		vwc.Webhooks = append(vwc.Webhooks,
			arv1.ValidatingWebhook{
				Name: fmt.Sprintf("%s.%s.%s.svc", gvr.Resource, name, namespace),
				ClientConfig: arv1.WebhookClientConfig{
					Service: &arv1.ServiceReference{
						Name:      name,
						Namespace: namespace,
						Path:      &path,
						Port:      &port,
					},
					CABundle: caBundle,
				},
				Rules: []arv1.RuleWithOperations{
					{
						Operations: []arv1.OperationType{arv1.Create, arv1.Update},
						Rule: arv1.Rule{
							APIGroups:   []string{gvr.Group},
							APIVersions: []string{gvr.Version},
							Resources:   []string{gvr.Resource},
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

// upsertValidatingWebhookConfig attempts to create or update a validatingwebhookconfiguration
// resource if one exists.
func upsertValidatingWebhookConfig(ctx context.Context, api v1.ValidatingWebhookConfigurationInterface, in *arv1.ValidatingWebhookConfiguration) (*arv1.ValidatingWebhookConfiguration, error) {
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
