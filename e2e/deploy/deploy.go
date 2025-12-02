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

// Package deploy provides utilities for provisioning and configuring the Prometheus engine components and a synthetic
// application within a Kubernetes cluster. These deployed resources serve as the test environment for end-to-end
// tests, enabling validation of the Prometheus engine's functionality by interacting with its operator, collector,
// and rule-evaluator, and by scraping metrics from the synthetic application.
package deploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/manifests"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateResources(ctx context.Context, kubeClient client.Client, deployOpts ...DeployOption) error {
	opts := &deployOptions{}
	for _, opt := range deployOpts {
		opt(opts)
	}
	opts.setDefaults()

	if opts.disableGCM && opts.explicitCredentials != "" {
		return errors.New("both disableGCM and explicitCredentials option was set; forbidden")
	}

	return createResources(ctx, kubeClient, func(obj client.Object) (client.Object, error) {
		switch obj := obj.(type) {
		case *appsv1.DaemonSet:
			return normalizeDaemonSets(opts, obj)
		case *appsv1.Deployment:
			return normalizeDeployments(opts, obj)
		}
		return obj, nil
	})
}

type DeployOption func(*deployOptions)

type deployOptions struct {
	operatorNamespace string
	publicNamespace   string
	projectID         string
	cluster           string
	location          string
	disableGCM        bool
	// TODO(bwplotka): Remove once runtime config can change auth options.
	// See https://github.com/GoogleCloudPlatform/prometheus/issues/261.
	explicitCredentials     string
	explicitCollectorFilter string
}

func (opts *deployOptions) setDefaults() {
	if opts.operatorNamespace == "" {
		opts.operatorNamespace = operator.DefaultOperatorNamespace
	}
	if opts.publicNamespace == "" {
		opts.publicNamespace = operator.DefaultPublicNamespace
	}
}

func WithOperatorNamespace(namespace string) DeployOption {
	return func(opts *deployOptions) {
		opts.operatorNamespace = namespace
	}
}

func WithPublicNamespace(namespace string) DeployOption {
	return func(opts *deployOptions) {
		opts.publicNamespace = namespace
	}
}

func WithMeta(projectID, cluster, location string) DeployOption {
	return func(opts *deployOptions) {
		opts.projectID = projectID
		opts.cluster = cluster
		opts.location = location
	}
}

func WithDisableGCM(disableGCM bool) DeployOption {
	return func(opts *deployOptions) {
		opts.disableGCM = disableGCM
	}
}

// WithExplicitCredentials sets explicit credential file path in local container to use.
// TODO(bwplotka): Remove once runtime config can change auth options.
// See https://github.com/GoogleCloudPlatform/prometheus/issues/261.
func WithExplicitCredentials(filepath string) DeployOption {
	return func(opts *deployOptions) {
		opts.explicitCredentials = filepath
	}
}

// WithExplicitCollectorFilter injects --export.match to collector.
// This is useful to reproduce cases when collector has left over match flags (e.g.
// via EXTRA_ARGS).
func WithExplicitCollectorFilter(filter string) DeployOption {
	return func(opts *deployOptions) {
		opts.explicitCollectorFilter = filter
	}
}

func createResources(ctx context.Context, kubeClient client.Client, normalizeFn func(client.Object) (client.Object, error)) error {
	resources, err := resources(kubeClient.Scheme())
	if err != nil {
		return err
	}
	for _, obj := range resources {
		obj, err = normalizeFn(obj)
		if err != nil {
			return err
		}
		if obj == nil {
			continue
		}

		if err := kubeClient.Create(ctx, obj); err != nil {
			return err
		}
	}
	return err
}

func resources(scheme *runtime.Scheme) ([]client.Object, error) {
	var resources []client.Object
	objs, err := kube.ResourcesFromYAML(scheme, manifests.CRDManifest)
	if err != nil {
		return nil, err
	}
	resources = append(resources, objs...)

	objs, err = kube.ResourcesFromYAML(scheme, manifests.OperatorManifest)
	if err != nil {
		return nil, err
	}
	resources = append(resources, objs...)
	return resources, nil
}

func normalizeDaemonSets(opts *deployOptions, obj *appsv1.DaemonSet) (client.Object, error) {
	if !opts.disableGCM && opts.explicitCredentials == "" {
		return obj, nil
	}
	if obj.GetName() != operator.NameCollector {
		return obj, nil
	}
	for i := range obj.Spec.Template.Spec.Containers {
		container := &obj.Spec.Template.Spec.Containers[i]
		if container.Name == operator.CollectorPrometheusContainerName {
			if opts.disableGCM {
				container.Args = append(container.Args, "--export.debug.disable-auth")
			} else if opts.explicitCredentials != "" {
				container.Args = append(container.Args, "--export.credentials-file="+opts.explicitCredentials)
			}

			if opts.explicitCollectorFilter != "" {
				container.Args = append(container.Args, "--export.match="+opts.explicitCollectorFilter)
			}
			return obj, nil
		}
	}
	return nil, fmt.Errorf("unable to find collector %q container", operator.CollectorPrometheusContainerName)
}

func normalizeDeployments(opts *deployOptions, obj *appsv1.Deployment) (client.Object, error) {
	switch obj.GetName() {
	case operator.NameOperator:
		container, err := kube.DeploymentContainer(obj, "operator")
		if err != nil {
			return nil, fmt.Errorf("unable to find operator container: %w", err)
		}
		if opts.projectID != "" {
			container.Args = append(container.Args, fmt.Sprintf("--project-id=%s", opts.projectID))
		}
		if opts.location != "" {
			container.Args = append(container.Args, fmt.Sprintf("--location=%s", opts.location))
		}
		if opts.cluster != "" {
			container.Args = append(container.Args, fmt.Sprintf("--cluster=%s", opts.cluster))
		}
		if opts.operatorNamespace != "" {
			container.Args = append(container.Args, fmt.Sprintf("--operator-namespace=%s", opts.operatorNamespace))
		}
		if opts.publicNamespace != "" {
			container.Args = append(container.Args, fmt.Sprintf("--public-namespace=%s", opts.publicNamespace))
		}
	case operator.NameRuleEvaluator:
		if !opts.disableGCM && opts.explicitCredentials == "" {
			break
		}
		container, err := kube.DeploymentContainer(obj, operator.RuleEvaluatorContainerName)
		if err != nil {
			return nil, fmt.Errorf("unable to find rule-evaluator %q container: %w", operator.RuleEvaluatorContainerName, err)
		}
		if opts.disableGCM {
			container.Args = append(container.Args, "--export.debug.disable-auth")
			container.Args = append(container.Args, "--query.debug.disable-auth")
			break
		}
		if opts.explicitCredentials != "" {
			container.Args = append(container.Args, "--export.credentials-file="+opts.explicitCredentials)
			container.Args = append(container.Args, "--query.credentials-file="+opts.explicitCredentials)
			break
		}
	default:
		return nil, fmt.Errorf("unhandled deployment: %q", obj.GetName())
	}
	return obj, nil
}
