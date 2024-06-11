// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deploy

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateFakeMetricService(ctx context.Context, kubeClient client.Client, namespace, name, image string) error {
	labels := map[string]string{
		"app.kubernetes.io/name": "fake-metric-service",
	}
	if err := kubeClient.Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "server",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "web",
									ContainerPort: 8080,
								},
								{
									Name:          "metric-service",
									ContainerPort: 8081,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "GRPC_GO_LOG_VERBOSITY_LEVEL",
									Value: "99",
								},
								{
									Name:  "GRPC_GO_LOG_SEVERITY_LEVEL",
									Value: "info",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "livez",
										Port:   intstr.FromInt(8080),
										Scheme: "HTTP",
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "readyz",
										Port:   intstr.FromInt(8080),
										Scheme: "HTTP",
									},
								},
							},
						},
					},
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("create metric-service deployment: %w", err)
	}
	if err := kubeClient.Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-grpc",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8081,
					TargetPort: intstr.FromString("metric-service"),
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("create metric-service service: %w", err)
	}
	if err := kubeClient.Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-web",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromString("web"),
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("create metric-service service: %w", err)
	}
	return kube.WaitForDeploymentReady(ctx, kubeClient, namespace, name)
}

func FakeMetricServiceEndpoint(namespace, name string) string {
	return fmt.Sprintf("%s-grpc.%s.svc.cluster.local:8081", name, namespace)
}

func FakeMetricServiceWebEndpoint(namespace, name string) string {
	return fmt.Sprintf("%s-web.%s.svc.cluster.local:8080", name, namespace)
}

func CreateFakeMetricCollector(ctx context.Context, kubeClient client.Client, namespace, name, metricServiceEndpoint string) error {
	labels := map[string]string{
		"app.kubernetes.io/name": "fake-metric-collector",
	}
	if err := kubeClient.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			// Use a static config because we don't want Kubernetes meta-labels.
			"config.yaml": fmt.Sprintf(`
global:
  scrape_interval: 5s
scrape_configs:
  - job_name: metric-collector
    static_configs:
      - targets: ["%s"]
`, metricServiceEndpoint),
		},
	}); err != nil {
		return err
	}
	if err := kubeClient.Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: operator.NameCollector,
					Containers: []corev1.Container{
						{
							Name:  "prometheus",
							Image: "prom/prometheus:v2.52.0",
							Args: []string{
								"--config.file=/config/config.yaml",
								"--web.listen-address=:8080",
								"--web.enable-lifecycle",
								"--web.route-prefix=/",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "api",
									ContainerPort: 8080,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/-/healthy",
										Port:   intstr.FromInt(8080),
										Scheme: "HTTP",
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/-/ready",
										Port:   intstr.FromInt(8080),
										Scheme: "HTTP",
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/config",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: name,
									},
								},
							},
						},
					},
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("create prometheus deployment: %w", err)
	}
	if err := kubeClient.Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromString("api"),
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("create metric-service service: %w", err)
	}
	return kube.WaitForDeploymentReady(ctx, kubeClient, namespace, name)
}

func FakeMetricCollectorEndpoint(namespace, name string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:8080", name, namespace)
}
