// Copyright 2026 Google LLC
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

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReferencedSecretsFromEndpoints(t *testing.T) {
	pmon := &PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "apps",
			Name:      "example",
		},
		Spec: PodMonitoringSpec{
			Endpoints: []ScrapeEndpoint{
				{
					HTTPClientConfig: HTTPClientConfig{
						BasicAuth: &BasicAuth{
							Password: &SecretSelector{
								Secret: &SecretKeySelector{
									Name: "metrics-auth",
									Key:  "password",
								},
							},
						},
					},
				},
				{
					HTTPClientConfig: HTTPClientConfig{
						TLS: &TLS{
							CA: &SecretSelector{
								Secret: &SecretKeySelector{
									Name: "metrics-tls",
									Key:  "ca",
								},
							},
						},
					},
				},
			},
		},
	}

	got := pmon.ReferencedSecrets()
	if len(got) != 2 {
		t.Fatalf("expected 2 secret references, got %d", len(got))
	}
	if got[0].Namespace != "apps" || got[0].Name != "metrics-auth" {
		t.Fatalf("unexpected first secret reference: %#v", got[0])
	}
	if got[1].Namespace != "apps" || got[1].Name != "metrics-tls" {
		t.Fatalf("unexpected second secret reference: %#v", got[1])
	}
}

func TestClusterPodMonitoringReferencedSecrets(t *testing.T) {
	cmon := &ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example",
		},
		Spec: ClusterPodMonitoringSpec{
			Endpoints: []ScrapeEndpoint{
				{
					HTTPClientConfig: HTTPClientConfig{
						OAuth2: &OAuth2{
							ClientSecret: &SecretSelector{
								Secret: &SecretKeySelector{
									Namespace: "oauth",
									Name:      "client",
									Key:       "secret",
								},
							},
						},
					},
				},
			},
		},
	}

	got := cmon.ReferencedSecrets()
	if len(got) != 1 {
		t.Fatalf("expected 1 secret reference, got %d", len(got))
	}
	if got[0].Namespace != "oauth" || got[0].Name != "client" {
		t.Fatalf("unexpected secret reference: %#v", got[0])
	}
}
