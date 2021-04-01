package operator

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	monitoringv1alpha1 "github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"

	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/relabel"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLabelMappingRelabelConfigs(t *testing.T) {
	cases := []struct {
		doc      string
		mappings []monitoringv1alpha1.LabelMapping
		prefix   model.LabelName
		expected []*relabel.Config
		expErr   bool
	}{
		{
			doc:      "good podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from", To: "to"}},
			prefix:   podLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{podLabelPrefix + "from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from-instance", To: "instance"}},
			prefix:   podLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			prefix:   podLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc:      "empty to podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from"}},
			prefix:   podLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{podLabelPrefix + "from"},
				TargetLabel:  "from",
			}},
			expErr: false,
		},
		{
			doc:      "good svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from", To: "to"}},
			prefix:   serviceLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{serviceLabelPrefix + "from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from-instance", To: "instance"}},
			prefix:   serviceLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			prefix:   serviceLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc:      "empty to svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from"}},
			prefix:   serviceLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{serviceLabelPrefix + "from"},
				TargetLabel:  "from",
			}},
			expErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			// If we get an error when we don't expect, fail test.
			actual, err := labelMappingRelabelConfigs(c.mappings, c.prefix)
			if err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", err)
			}
			if err == nil && c.expErr {
				t.Errorf("should have returned an error")
			}
			if !reflect.DeepEqual(c.expected, actual) {
				t.Errorf("returned unexpected config")
			}
		})
	}
}

func TestInitAdmissionResoures(t *testing.T) {
	var (
		logger      = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		aresp       = &admissionv1.AdmissionReview{}
		kubeClient  = fake.NewSimpleClientset()
	)
	t.Cleanup(cancel)

	// Initialize operator and admission https server.
	op := &Operator{
		logger:     logger,
		kubeClient: kubeClient,
		opts:       Options{Namespace: DefaultNamespace, CASelfSign: true, ListenAddr: ":8443"},
	}
	// Use self-signed to avoid dealing with CSRs.
	srv, err := op.InitAdmissionResources(ctx)
	if err != nil {
		t.Fatalf("initializing https server: %s", err)
	}

	// Check for webhook config.
	if vwc, err := kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, DefaultName, metav1.GetOptions{}); err != nil {
		t.Errorf("getting validatingwebhook config: %s", err)
	} else if vwc.Name != DefaultName {
		t.Errorf("invalid validatingwebhook config: %+v", vwc)
	}

	// Listen for incoming requests.
	t.Cleanup(func() { srv.Close() })
	go srv.ListenAndServeTLS("", "")

	// Set up request - ensure there is a valid body structure to allow handler to
	// return proper error response.
	body := mustMarshalJSON(t, &admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://localhost:8443/validate/podmonitorings",
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %s", err)
	}

	// Initialize https client, and call server.
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("calling server: %s", err)
	}

	// Ensure response body can be read and unmarshalled with some message content.
	if rb, err := ioutil.ReadAll(resp.Body); err != nil {
		t.Errorf("reading response body: %s", err)
	} else if err := json.Unmarshal(rb, aresp); err != nil {
		t.Errorf("decoding response body: %s", err)
	} else if aresp.Response.Result.Message == "" {
		t.Errorf("request not handled by server: %s", err)
	}
}
