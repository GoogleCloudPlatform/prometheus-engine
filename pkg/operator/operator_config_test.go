// Copyright 2022 Google LLC
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
	"strings"
	"testing"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOperatorConfigValidator(t *testing.T) {
	v := &operatorConfigValidator{namespace: "foo"}

	cases := []struct {
		desc string
		oc   *monitoringv1.OperatorConfig
		err  string
	}{
		{
			desc: "valid",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
			},
		},
		{
			desc: "bad namespace",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo_x",
					Name:      "config",
				},
			},
			err: `OperatorConfig must be in namespace "foo" with name "config"`,
		},
		{
			desc: "bad name",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config_x",
				},
			},
			err: `OperatorConfig must be in namespace "foo" with name "config"`,
		},
		{
			desc: "bad scrape interval",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Collection: monitoringv1.CollectionSpec{
					KubeletScraping: &monitoringv1.KubeletScraping{
						Interval: "xyz",
					},
				},
			},
			err: `invalid scrape interval: not a valid duration string: "xyz"`,
		},
		{
			desc: "missing scrape interval",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Collection: monitoringv1.CollectionSpec{
					KubeletScraping: &monitoringv1.KubeletScraping{
						Interval: "",
					},
				},
			},
			err: `invalid scrape interval: empty duration string`,
		},
		{
			desc: "bad generator URL",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					GeneratorURL: "~:://example.com",
				},
			},
			err: `failed to parse generator URL: parse "~:://example.com": first path segment in URL cannot contain colon`,
		},
		{
			desc: "missing collection credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Collection: monitoringv1.CollectionSpec{
					Credentials: &v1.SecretKeySelector{},
				},
			},
			err: "invalid collection credentials: missing secret key selector name",
		},
		{
			desc: "collection credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Collection: monitoringv1.CollectionSpec{
					Credentials: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "baz",
						},
					},
				},
			},
		},
		{
			desc: "missing managed alert manager config secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
					ConfigSecret: &v1.SecretKeySelector{},
				},
			},
			err: "invalid managed alert manager config secret: missing secret key selector name",
		},
		{
			desc: "managed alert manager config secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				ManagedAlertmanager: &monitoringv1.ManagedAlertmanagerSpec{
					ConfigSecret: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "baz",
						},
					},
				},
			},
		},
		{
			desc: "missing rule manager credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Credentials: &v1.SecretKeySelector{},
				},
			},
			err: "invalid rules config: invalid credentials: missing secret key selector name",
		},
		{
			desc: "rule manager credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Credentials: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "baz",
						},
					},
				},
			},
		},
		{
			desc: "missing rule manager authorization credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS:  &monitoringv1.TLSConfig{},
							Authorization: &monitoringv1.Authorization{
								Credentials: &v1.SecretKeySelector{},
							},
						}},
					},
				},
			},
			err: "invalid rules config: invalid alert manager endpoint `bar` (index 0): invalid authorization credentials: missing secret key selector name",
		},
		{
			desc: "rule manager authorization credentials secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS:  &monitoringv1.TLSConfig{},
							Authorization: &monitoringv1.Authorization{
								Credentials: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "baz",
									},
								},
							},
						}},
					},
				},
			},
		},
		{
			desc: "missing rule manager TLS secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								KeySecret: &v1.SecretKeySelector{},
							},
						}},
					},
				},
			},
			err: "invalid rules config: invalid alert manager endpoint `bar` (index 0): invalid TLS key: missing secret key selector name",
		},
		{
			desc: "rule manager TLS secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								KeySecret: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "baz",
									},
								},
							},
						}},
					},
				},
			},
		},
		{
			desc: "missing rule manager TLS CA secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								CA: &monitoringv1.SecretOrConfigMap{
									Secret: &v1.SecretKeySelector{},
								},
							},
						}},
					},
				},
			},
			err: "invalid rules config: invalid alert manager endpoint `bar` (index 0): invalid TLS CA: missing secret key selector name",
		},
		{
			desc: "rule manager TLS CA secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								CA: &monitoringv1.SecretOrConfigMap{
									Secret: &v1.SecretKeySelector{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "baz",
										},
									},
								},
							},
						}},
					},
				},
			},
		},
		{
			desc: "missing rule manager TLS Cert secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								Cert: &monitoringv1.SecretOrConfigMap{
									Secret: &v1.SecretKeySelector{},
								},
							},
						}},
					},
				},
			},
			err: "invalid rules config: invalid alert manager endpoint `bar` (index 0): invalid TLS Cert: missing secret key selector name",
		},
		{
			desc: "rule manager TLS Cert secret key",
			oc: &monitoringv1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					Alerting: monitoringv1.AlertingSpec{
						Alertmanagers: []monitoringv1.AlertmanagerEndpoints{{
							Name: "bar",
							TLS: &monitoringv1.TLSConfig{
								Cert: &monitoringv1.SecretOrConfigMap{
									Secret: &v1.SecretKeySelector{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "baz",
										},
									},
								},
							},
						}},
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), c.oc)
			if err == nil && c.err == "" {
				return
			}
			if c.err == "" && err != nil {
				t.Fatalf("unexpected error %q", err)
			}
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("expected error containing %q but got %q", c.err, err)
			}
		})
	}
}
