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

package deployutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/prometheus/client_golang/prometheus"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deployLocalOptions struct {
	deployOptions
	portForward bool
}

type DeployLocalOption interface {
	applyToDeployLocalOptions(*deployLocalOptions)
}

func (f deployOptionFunc) applyToDeployLocalOptions(do *deployLocalOptions) {
	f(&do.deployOptions)
}

type deployLocalOptionFunc func(*deployLocalOptions)

func (f deployLocalOptionFunc) applyToDeployLocalOptions(do *deployLocalOptions) {
	f(do)
}

func WithPortForward(portForward bool) DeployLocalOption {
	return deployLocalOptionFunc(func(do *deployLocalOptions) {
		do.portForward = portForward
	})
}

func DeployLocalOperator(t testing.TB, ctx context.Context, restConfig *rest.Config, kubeClient client.Client, deployOpts ...DeployLocalOption) error {
	opts := &deployLocalOptions{}
	for _, opt := range deployOpts {
		opt.applyToDeployLocalOptions(opts)
	}

	var httpClient *http.Client
	if opts.portForward {
		var err error
		httpClient, err = kubeutil.PortForwardClient(t, restConfig, kubeClient)
		if err != nil {
			return fmt.Errorf("creating HTTP client: %s", err)
		}
	}

	op, err := operator.New(opts.logger, restConfig, operator.Options{
		ProjectID:         opts.projectID,
		Cluster:           opts.cluster,
		Location:          opts.location,
		OperatorNamespace: opts.operatorNamespace,
		PublicNamespace:   opts.publicNamespace,
		// Pick a random available port.
		ListenAddr:          ":0",
		CollectorHTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("instantiating operator: %s", err)
	}

	go func() {
		if err := op.Run(ctx, prometheus.NewRegistry()); err != nil {
			// Since we aren't in the main test goroutine we cannot fail with Fatal here.
			t.Errorf("running operator: %s", err)
		}
	}()

	return createResources(t, ctx, restConfig, kubeClient, &opts.deployOptions, func(obj client.Object) client.Object {
		switch obj := obj.(type) {
		case *admissionregistrationv1.MutatingWebhookConfiguration, *admissionregistrationv1.ValidatingWebhookConfiguration:
			return nil
		case *appsv1.DaemonSet:
			if obj.GetName() == operator.NameCollector {
				for i := range obj.Spec.Template.Spec.Containers {
					container := &obj.Spec.Template.Spec.Containers[i]
					if container.Name == operator.CollectorPrometheusContainerName {
						container.Args = append(container.Args, "--export.debug.disable-auth")
						break
					}
				}
			}
		case *appsv1.Deployment:
			if obj.GetName() == operator.NameOperator {
				// Don't deploy the operator because we already started it!
				return nil
			}
		}
		return obj
	})
}
