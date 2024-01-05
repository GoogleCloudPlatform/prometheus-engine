// Copyright 2023 Google LLC
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

package operatorutil

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const blackboxExporterManifest = "../cmd/operator/deploy/operator/13-blackbox.yaml"

func DeployBlackboxExporter(ctx context.Context, kubeClient client.Client, namespace, labelName, labelValue string) error {
	obj, err := kubeutil.ResourceFromFile(kubeClient.Scheme(), blackboxExporterManifest)
	if err != nil {
		return fmt.Errorf("decode blackbox-exporter: %w", err)
	}
	deployment := obj.(*appsv1.Deployment)
	deployment.Namespace = namespace
	if deployment.Spec.Template.Labels == nil {
		deployment.Spec.Template.Labels = map[string]string{}
	}
	deployment.Spec.Template.Labels[labelName] = labelValue

	if err = kubeClient.Create(ctx, deployment); err != nil {
		return fmt.Errorf("create blackbox-exporter Deployment: %w", err)
	}
	return nil
}
