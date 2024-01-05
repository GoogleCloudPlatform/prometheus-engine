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

package e2e

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/operatorutil"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProbe(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)
	ctx := context.Background()

	if err := operatorutil.DeployBlackboxExporter(ctx, tctx.Client(), tctx.namespace, testLabel, tctx.GetOperatorTestLabelValue()); err != nil {
		t.Fatal(err)
	}

	tctx.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
		Features: monitoringv1.OperatorFeatures{
			TargetStatus: monitoringv1.TargetStatusSpec{
				Enabled: true,
			},
		},
	})

	c := tctx.Client()
	probe := monitoringv1.Probe{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example",
		},
		Spec: monitoringv1.ProbeSpec{
			Targets: []monitoringv1.ProbeTarget{
				{
					Module: "http_2xx",
					StaticTargets: []string{
						"http://example.com",
					},
				},
			},
		},
	}
	if err := c.Create(ctx, &probe); err != nil {
		t.Fatal(err)
	}

	if err := kubeutil.WaitForDeploymentReady(ctx, c, tctx.namespace, "blackbox-exporter"); err != nil {
		t.Fatal(err)
	}
	if err := operatorutil.WaitForProbeReady(ctx, c, tctx.namespace, &probe); err != nil {
		t.Fatal(err)
	}
}
