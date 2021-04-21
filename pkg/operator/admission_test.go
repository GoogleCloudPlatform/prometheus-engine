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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	monitoring "github.com/google/gpe-collector/pkg/operator/apis/monitoring"
	"github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAdmitPodMonitoring(t *testing.T) {
	cases := []struct {
		doc      string
		resGroup string
		resVer   string
		res      string
		mappings []v1alpha1.LabelMapping
		expErr   bool
	}{
		{
			doc:      "admit",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "to"}},
			expErr:   false,
		},
		{
			doc:      "admit no relabelling",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: nil,
			expErr:   false,
		},
		{
			doc:      "admit preserved relabelling",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "from"}},
			expErr:   false,
		},
		{
			doc:      "admit unset To relabelling",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from"}},
			expErr:   false,
		},
		{
			doc:      "bad api group",
			resGroup: "unsupported.domain.group",
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "to"}},
			expErr:   true,
		},
		{
			doc:      "bad api version",
			resGroup: monitoring.GroupName,
			resVer:   "v0",
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "to"}},
			expErr:   true,
		},
		{
			doc:      "bad api resource",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "servicemonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "to"}},
			expErr:   true,
		},
		{
			doc:      "bad non-plural api resource",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "PodMonitoring",
			mappings: []v1alpha1.LabelMapping{{From: "from", To: "to"}},
			expErr:   true,
		},
		{
			doc:      "bad relabel mappings - conflicts with target schema",
			resGroup: monitoring.GroupName,
			resVer:   v1alpha1.Version,
			res:      "podmonitorings",
			mappings: []v1alpha1.LabelMapping{{From: "from-instance", To: "instance"}},
			expErr:   true,
		},
	}

	// Test cases for proper admission requests.
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s - no scrape endpoints", c.doc), func(t *testing.T) {
			// Prepare PodMonitoring resource.
			res := &v1alpha1.PodMonitoring{}
			res.Spec.TargetLabels.FromPod = c.mappings

			// Create admission request.
			bytes := mustMarshalJSON(t, res)
			req := &v1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    c.resGroup,
					Version:  c.resVer,
					Resource: c.res,
				},
				Object: runtime.RawExtension{
					Raw: bytes,
				},
			}

			// Send admission request to pod monitoring validation.
			if resp, err := admitPodMonitoring(&v1.AdmissionReview{
				Request: req,
			}); err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", resp.Result.Message)
			} else if err == nil && c.expErr {
				t.Errorf("should have returned error")
			}
		})
		t.Run(fmt.Sprintf("%s - with scrape endpoints", c.doc), func(t *testing.T) {
			// Prepare PodMonitoring resource.
			res := &v1alpha1.PodMonitoring{
				Spec: v1alpha1.PodMonitoringSpec{
					Endpoints: []v1alpha1.ScrapeEndpoint{
						{
							Port:     intstr.FromString("8080"),
							Interval: "5s",
						},
					},
				},
			}
			res.Spec.TargetLabels.FromPod = c.mappings

			// Create admission request.
			bytes := mustMarshalJSON(t, res)
			req := &v1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    c.resGroup,
					Version:  c.resVer,
					Resource: c.res,
				},
				Object: runtime.RawExtension{
					Raw: bytes,
				},
			}

			// Send admission request to pod monitoring validation.
			if resp, err := admitPodMonitoring(&v1.AdmissionReview{
				Request: req,
			}); err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", resp.Result.Message)
			} else if err == nil && c.expErr {
				t.Errorf("should have returned error")
			}
		})
	}

	// Test bad request object sent to admission controller.
	t.Run("bad request bytes", func(t *testing.T) {
		// Create admission request.
		req := &v1.AdmissionRequest{
			Resource: v1alpha1.PodMonitoringResource(),
			Object: runtime.RawExtension{
				Raw: []byte("bad-data"),
			},
		}
		if _, err := admitPodMonitoring(&v1.AdmissionReview{
			Request: req,
		}); err == nil {
			t.Errorf("should have returned an error")
		}
	})
}

func TestServeAdmission(t *testing.T) {
	var (
		expUID  = types.UID("1234")
		expVer  = "v1"
		expKind = "AdmissionReview"
	)
	pm := mustMarshalJSON(t, &v1alpha1.PodMonitoring{})
	body := mustMarshalJSON(t, &v1.AdmissionReview{
		Request: &v1.AdmissionRequest{
			UID:      expUID,
			Resource: v1alpha1.PodMonitoringResource(),
			Object: runtime.RawExtension{
				Raw: pm,
			},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: expVer,
			Kind:       expKind,
		},
	})

	cases := []struct {
		doc    string
		body   []byte
		expErr bool
	}{
		{
			doc:    "good admission request",
			body:   body,
			expErr: false,
		},
		{
			doc:    "bad admission request",
			body:   []byte("bad-data"),
			expErr: true,
		},
	}

	as := NewAdmissionServer(log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)))
	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			var (
				r    = httptest.NewRequest("POST", "/", bytes.NewReader(c.body))
				w    = httptest.NewRecorder()
				resp = &v1.AdmissionReview{}
			)

			// Serve admission request and check case results.
			as.serveAdmission(noopAdmit).ServeHTTP(w, r)
			if rb, err := ioutil.ReadAll(w.Result().Body); err != nil {
				t.Errorf("reading response body: %s", err)
			} else if err := json.Unmarshal(rb, resp); err != nil {
				t.Errorf("decoding response body: %s", err)
			} else if c.expErr && resp.Response.Allowed {
				t.Errorf("response should not be allowed for body %v", c.body)
			} else if uid := resp.Response.UID; !c.expErr && uid != expUID {
				t.Errorf("expected uid to be %v but recieved %v", expUID, uid)
			} else if ver := resp.APIVersion; !c.expErr && ver != expVer {
				t.Errorf("expected version to be %q but recieved %q", expVer, ver)
			} else if kind := resp.Kind; !c.expErr && kind != expKind {
				t.Errorf("expected kind to be %q but recieved %q", expKind, kind)
			}
		})
	}
}

// noopAdmit performs a no-op on the incoming admission review
// and returns an empty response.
func noopAdmit(_ *v1.AdmissionReview) (*v1.AdmissionResponse, error) {
	return &v1.AdmissionResponse{}, nil
}

// mustMarshalJSON is a testing helper function that attempts to marshal
// the provided value. If any errors are encountered, the test fails
// immediately and returns nil.
func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	if bytes, err := json.Marshal(v); err != nil {
		t.Fatalf("marshalling resource: %s", err)
	} else {
		return bytes
	}
	return nil
}
