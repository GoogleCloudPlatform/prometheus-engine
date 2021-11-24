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
	"bytes"
	"context"
	"testing"
	"time"

	monitoring "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	arv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestValidatingWebhookConfig(t *testing.T) {
	var (
		timeout     = 3 * time.Second
		client      = fake.NewSimpleClientset()
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		gvrs        = []metav1.GroupVersionResource{
			v1alpha1.PodMonitoringResource(),
		}
	)

	t.Cleanup(cancel)

	cases := []struct {
		doc      string
		caBundle []byte
	}{
		{
			doc:      "check create and validate",
			caBundle: []byte{1, 2, 3, 4},
		},
		{
			doc:      "check update and validate",
			caBundle: []byte{5, 6, 7, 8},
		},
	}

	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			vwCfg, err := upsertValidatingWebhookConfig(ctx,
				client.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
				validatingWebhookConfig("gmp-operator", "gmp-system", 12345, c.caBundle, gvrs))
			if err != nil {
				t.Fatalf("upserting validtingwebhookconfig: %s", err)
			}
			if whs := vwCfg.Webhooks; len(whs) != 1 {
				t.Errorf("unexpected number of webhooks: %d", len(whs))
			}
			if wh := vwCfg.Webhooks[0]; wh.Name != "podmonitorings.gmp-operator.gmp-system.svc" {
				t.Errorf("unexpected webhook name: %s", wh.Name)
			} else if name := wh.ClientConfig.Service.Name; name != "gmp-operator" {
				t.Errorf("unexpected webhook config name: %s", name)
			} else if ns := wh.ClientConfig.Service.Namespace; ns != "gmp-system" {
				t.Errorf("unexpected webhook config namespace: %s", ns)
			} else if port := wh.ClientConfig.Service.Port; *port != 12345 {
				t.Errorf("unexpected webhook config port: %v", port)
			} else if path := *wh.ClientConfig.Service.Path; path != "/validate/monitoring.googleapis.com/v1alpha1/podmonitorings" {
				t.Errorf("unexpected webhook path: %s", path)
			} else if crt := wh.ClientConfig.CABundle; !bytes.Equal(crt, c.caBundle) {
				t.Errorf("unexpected caBundle: %v", crt)
			} else if rule := wh.Rules[0]; !(rule.Operations[0] == arv1.Create && rule.Operations[1] == arv1.Update) {
				t.Errorf("unexpected rule operations: %+v", rule)
			} else if rr := rule.Rule; !(rr.APIGroups[0] == monitoring.GroupName && rr.APIVersions[0] == v1alpha1.Version && rr.Resources[0] == "podmonitorings") {
				t.Errorf("unexpected rule resources: %+v", rule)
			} else if policy := *wh.FailurePolicy; policy != arv1.Ignore {
				t.Errorf("unexpected policy: %s", policy)
			} else if se := *wh.SideEffects; se != arv1.SideEffectClassNone {
				t.Errorf("unexpected side effects: %s", se)
			} else if arvs := wh.AdmissionReviewVersions; arvs[0] != "v1" {
				t.Errorf("unexpected admission review versions: %v", arvs)
			}
		})
	}
}
