package operator

import (
	"bytes"
	"context"
	"testing"
	"time"

	monitoring "github.com/google/gpe-collector/pkg/operator/apis/monitoring"
	"github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"
	arv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestValidatingWebhookConfig(t *testing.T) {
	var (
		timeout     = 3 * time.Second
		client      = fake.NewSimpleClientset()
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		endpoints   = []string{"/validate/podmonitorings"}
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
			vwCfg, err := UpsertValidatingWebhookConfig(ctx,
				client.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
				ValidatingWebhookConfig("gpe-operator", "gpe-system", c.caBundle, endpoints))
			if err != nil {
				t.Fatalf("upserting validtingwebhookconfig: %s", err)
			}
			if whs := vwCfg.Webhooks; len(whs) != 2 {
				t.Errorf("unexpected number of webhooks: %d", len(whs))
			}
			for i, res := range []string{"podmonitorings"} {
				if wh := vwCfg.Webhooks[i]; wh.Name != res+".gpe-operator.gpe-system.svc" {
					t.Errorf("unexpected webhook name: %s", wh.Name)
				} else if name := wh.ClientConfig.Service.Name; name != "gpe-operator" {
					t.Errorf("unexpected webhook config name: %s", name)
				} else if ns := wh.ClientConfig.Service.Namespace; ns != "gpe-system" {
					t.Errorf("unexpected webhook config namespace: %s", ns)
				} else if path := *wh.ClientConfig.Service.Path; path != "/validate/"+res {
					t.Errorf("unexpected webhook path: %s", path)
				} else if crt := wh.ClientConfig.CABundle; bytes.Compare(crt, c.caBundle) != 0 {
					t.Errorf("unexpected caBundle: %v", crt)
				} else if rule := wh.Rules[0]; !(rule.Operations[0] == arv1.Create && rule.Operations[1] == arv1.Update) {
					t.Errorf("unexpected rule operations: %+v", rule)
				} else if rr := rule.Rule; !(rr.APIGroups[0] == monitoring.GroupName && rr.APIVersions[0] == v1alpha1.Version && rr.Resources[0] == res) {
					t.Errorf("unexpected rule resources: %+v", rule)
				} else if policy := *wh.FailurePolicy; policy != arv1.Ignore {
					t.Errorf("unexpected policy: %s", policy)
				} else if se := *wh.SideEffects; se != arv1.SideEffectClassNone {
					t.Errorf("unexpected side effects: %s", se)
				} else if arvs := wh.AdmissionReviewVersions; arvs[0] != "v1" {
					t.Errorf("unexpected admission review versions: %v", arvs)
				}
			}
		})
	}
}
