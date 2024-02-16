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

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package e2e

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

var (
	images = [...]string{"operator", "config-reloader", "rule-evaluator", "example-app"}

	projectID, location, cluster string
	skipGCM                      bool
	pollDuration                 time.Duration

	gcpServiceAccount string

	imageRegistryPort int
	imageTag          string
)

// TestMain injects custom flags to tests.
func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.Level(zapcore.DebugLevel)))

	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.DurationVar(&pollDuration, "duration", 3*time.Second, "How often to poll and retry for resources.")

	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")

	flag.IntVar(&imageRegistryPort, "registry-port", -1, "The port of the local registry.")
	flag.StringVar(&imageTag, "image-tag", "", "The tag to copy images from.")

	flag.Parse()

	os.Exit(m.Run())
}

func setupCluster(ctx context.Context, logger log) (kubernetes.Interface, versioned.Interface, error) {
	if imageTag != "" && imageRegistryPort >= 0 {
		if err := copyImagesToLocalRegistry(ctx, logger, imageTag, imageRegistryPort); err != nil {
			return nil, nil, err
		}
	} else {
		logger.Log(">>> skipping image copy")
	}

	restConfig, err := newRestConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, opClient, err := newKubeClients(restConfig)
	if err != nil {
		return nil, nil, err
	}

	scheme, err := newScheme()
	if err != nil {
		return nil, nil, err
	}

	c, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, nil, err
	}

	logger.Log(">>> deploying static resources")
	if err := createResources(ctx, c, logger); err != nil {
		return nil, nil, err
	}
	return kubeClient, opClient, nil
}

type log interface {
	Log(args ...any)
	Logf(format string, args ...any)
}

func newRestConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
}

func newKubeClients(restConfig *rest.Config) (kubernetes.Interface, versioned.Interface, error) {
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, err
	}
	opClient, err := versioned.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, err
	}

	return kubeClient, opClient, nil
}

func newScheme() (*runtime.Scheme, error) {
	scheme, err := operator.NewScheme()
	if err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func createResources(ctx context.Context, kubeClient client.Client, logger log) error {
	if err := deploy.CreateResources(context.Background(), kubeClient, deploy.WithMeta(projectID, cluster, location), deploy.WithDisableGCM(skipGCM)); err != nil {
		return err
	}

	if gcpServiceAccount == "" {
		gcpServiceAccount, _ = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	}
	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return fmt.Errorf("read service account file %q: %w", gcpServiceAccount, err)
		}
		if err := deploy.CreateGCPSecretResources(context.Background(), kubeClient, metav1.NamespaceDefault, b); err != nil {
			return err
		}
	}

	logger.Log(">>> waiting for operator to be deployed")
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		return err
	}
	logger.Log(">>> waiting for operator to be ready")
	if err := deploy.WaitForOperatorReady(ctx, kubeClient, operator.DefaultOperatorNamespace); err != nil {
		return err
	}

	return nil
}

func imageExists(ctx context.Context, c *dockerclient.Client, image string) (bool, error) {
	images, err := c.ImageList(ctx, types.ImageListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", image)),
	})
	if err != nil {
		return false, err
	}
	if len(images) > 1 {
		return false, errors.New("more than one image found")
	}
	return len(images) == 1, nil
}

func copyImagesToLocalRegistry(ctx context.Context, logger log, tag string, port int) error {
	if tag == "" {
		return errors.New("tag is required")
	}
	if port < 0 {
		return fmt.Errorf("invalid port: %d", port)
	}

	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return err
	}
	dockerClient.NegotiateAPIVersion(ctx)

	for _, image := range images {
		srcRoot := fmt.Sprintf("gmp/%s", image)
		srcUntagged := fmt.Sprintf("%s:latest", srcRoot)
		srcTagged := fmt.Sprintf("%s:%s", srcRoot, tag)

		var src string
		exists, err := imageExists(ctx, dockerClient, srcTagged)
		if err != nil {
			return err
		}
		if !exists {
			logger.Logf(">>> using latest image: %s", image)
			exists, err = imageExists(ctx, dockerClient, srcUntagged)
			if err != nil {
				return err
			}
			src = srcUntagged
		} else {
			src = srcTagged
		}
		if !exists {
			logger.Logf(">>> skipping non-existent image: %s", image)
			return nil
		}

		dst := fmt.Sprintf("localhost:%d/%s:%s", port, image, tag)
		exists, err = imageExists(ctx, dockerClient, dst)
		if err != nil {
			return err
		}
		if exists {
			logger.Logf(">>> skipping existent destination: %s", dst)
			return nil
		}

		logger.Logf(">>> tagging and pushing image: %s", image)
		if err := dockerClient.ImageTag(ctx, src, dst); err != nil {
			return err
		}
		_, err = dockerClient.ImagePush(ctx, dst, types.ImagePushOptions{
			RegistryAuth: "*",
		})
		if err != nil {
			return err
		}
	}
	return err
}
