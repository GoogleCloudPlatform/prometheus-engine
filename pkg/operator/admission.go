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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes/scheme"

	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type admitFn func(*v1.AdmissionReview) (*v1.AdmissionResponse, error)

// AdmissionServer serves Kubernetes resource admission requests.
type AdmissionServer struct {
	logger  logr.Logger
	decoder runtime.Decoder
}

// NewAdmissionServer returns a new AdmissionServer with the provided logger.
func NewAdmissionServer(logger logr.Logger) *AdmissionServer {
	return &AdmissionServer{
		logger:  logger,
		decoder: scheme.Codecs.UniversalDeserializer(),
	}
}

// serveAdmission returns a http handler closure that evaluates Kubernetes admission
// requests. Encountered errors are logged and potentially returned in the admission
// response.
func (a *AdmissionServer) serveAdmission(admit admitFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.logger.V(1).Info("webhook called",
			"method", r.Method,
			"host", r.Host,
			"path", r.URL.Path)

		var req, resp v1.AdmissionReview
		// Read, decode, and evaluate admission request.
		if data, err := ioutil.ReadAll(r.Body); err != nil {
			a.logger.Error(err, "reading request body")
			resp.Response = toAdmissionResponse(err)
		} else if _, _, err := a.decoder.Decode(data, nil, &req); err != nil {
			a.logger.Error(err, "decoding request body")
			resp.Response = toAdmissionResponse(err)
		} else if ar, err := admit(&req); err != nil {
			a.logger.Error(err, "admitting admission request")
			resp.Response = toAdmissionResponse(err)
		} else {
			resp.Response = ar
		}
		// Return the same API, Kind, and UID as long as incoming
		// request data was decoded properly.
		if req.Request != nil {
			resp.APIVersion = req.APIVersion
			resp.Kind = req.Kind
			resp.Response.UID = req.Request.UID
		}

		// Write the admission response.
		if respBytes, err := json.Marshal(resp); err != nil {
			a.logger.Error(err, "encoding response body")
		} else if _, err := w.Write(respBytes); err != nil {
			a.logger.Error(err, "writing response body")
		}
	}
}

// admitPodMonitoring evaluates incoming PodMonitoring resources to ensure
// they are a valid resource.
func admitPodMonitoring(ar *v1.AdmissionReview) (*v1.AdmissionResponse, error) {
	var pm = &v1alpha1.PodMonitoring{}
	// Ensure the requested resource is a PodMonitoring.
	if ar.Request.Resource != v1alpha1.PodMonitoringResource() {
		return nil, fmt.Errorf("expected resource to be %v, but received %v", v1alpha1.PodMonitoringResource(), ar.Request.Resource)
		// Unmarshall payload to PodMonitoring stuct.
	} else if err := json.Unmarshal(ar.Request.Object.Raw, pm); err != nil {
		return nil, errors.Wrap(err, "unmarshalling admission request to podmonitoring")
		// If scrape endpoints are provided, try and create scrape configs.
	} else if eps := pm.Spec.Endpoints; len(eps) > 0 {
		for i := range eps {
			if _, err := makePodScrapeConfig(pm, i); err != nil {
				return nil, errors.Wrap(err, "making scrape config from podmonitoring resource")
			}
		}
		// If no scrape endpoints are provided, at least check label conflicts.
	} else if _, err := labelMappingRelabelConfigs(pm.Spec.TargetLabels.FromPod, podLabelPrefix); err != nil {
		return nil, errors.Wrap(err, "checking label mappings")
	}

	return &v1.AdmissionResponse{Allowed: true}, nil
}

// toAdmissionResponse is a helper function that returns an AdmissionResponse
// containing a message of the provided error.
func toAdmissionResponse(err error) *v1.AdmissionResponse {
	return &v1.AdmissionResponse{
		Allowed: false, // make explicit for clarity
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: err.Error(),
		},
	}
}
