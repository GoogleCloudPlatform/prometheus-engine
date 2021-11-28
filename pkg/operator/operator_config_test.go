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
	"strings"
	"testing"

	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOperatorConfigValidator(t *testing.T) {
	v := &operatorConfigValidator{namespace: "foo"}

	cases := []struct {
		desc string
		oc   *monitoringv1alpha1.OperatorConfig
		err  string
	}{
		{
			desc: "valid",
			oc: &monitoringv1alpha1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config",
				},
			},
		},
		{
			desc: "bad namespace",
			oc: &monitoringv1alpha1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo_x",
					Name:      "config",
				},
			},
			err: `OperatorConfig must be in namespace "foo" with name "config"`,
		},
		{
			desc: "bad name",
			oc: &monitoringv1alpha1.OperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "config_x",
				},
			},
			err: `OperatorConfig must be in namespace "foo" with name "config"`,
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := v.ValidateCreate(context.Background(), c.oc)
			if err == nil && c.err == "" {
				return
			}
			if c.err == "" && err != nil {
				t.Fatalf("unexpected error %q", err)
			}
			if !strings.Contains(err.Error(), c.err) {
				t.Fatalf("expected error containing %q but got %q", c.err, err)
			}
		})
	}
}
