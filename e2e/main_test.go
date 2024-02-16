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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

const (
	// Arbitrary amount of time to let tests exit cleanly before the main process
	// terminates.
	timeoutGracePeriod = 10 * time.Second
)

var (
	images = [...]string{"operator", "config-reloader", "rule-evaluator", "go-synthetic"}

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

func setupCluster(ctx context.Context, t testing.TB) (client.Client, *rest.Config, error) {
	if imageTag != "" && imageRegistryPort >= 0 {
		if err := copyImagesToLocalRegistry(ctx, t, imageTag, imageRegistryPort); err != nil {
			return nil, nil, err
		}
	} else {
		t.Log(">>> skipping image copy")
	}

	restConfig, err := newRestConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := newKubeClient(restConfig)
	if err != nil {
		return nil, nil, err
	}

	t.Log(">>> deploying static resources")
	if err := createResources(ctx, kubeClient); err != nil {
		return nil, nil, err
	}

	t.Log(">>> waiting for operator to be deployed")
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		return nil, nil, err
	}
	t.Log(">>> waiting for operator to be ready")
	if err := deploy.WaitForOperatorReady(ctx, kubeClient); err != nil {
		return nil, nil, err
	}
	t.Log(">>> operator started successfully")
	return kubeClient, restConfig, nil
}

func setRESTConfigDefaults(restConfig *rest.Config) error {
	// https://github.com/kubernetes/client-go/issues/657
	// https://github.com/kubernetes/client-go/issues/1159
	// https://github.com/kubernetes/kubectl/blob/6fb6697c77304b7aaf43a520d30cb17563c69886/pkg/cmd/util/kubectl_match_version.go#L115
	defaultGroupVersion := &schema.GroupVersion{Group: "", Version: "v1"}
	if restConfig.GroupVersion == nil {
		restConfig.GroupVersion = defaultGroupVersion
	}
	if restConfig.APIPath == "" {
		restConfig.APIPath = "/api"
	}
	if restConfig.NegotiatedSerializer == nil {
		restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	return rest.SetKubernetesDefaults(restConfig)
}

func newRestConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return nil, err
	}
	if err := setRESTConfigDefaults(restConfig); err != nil {
		return nil, err
	}
	return restConfig, nil
}

func newKubeClient(restConfig *rest.Config) (client.Client, error) {
	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}

	return client.New(restConfig, client.Options{
		Scheme: scheme,
	})
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

func createResources(ctx context.Context, kubeClient client.Client) error {
	if err := deploy.CreateResources(ctx, kubeClient, deploy.WithMeta(projectID, cluster, location), deploy.WithDisableGCM(skipGCM)); err != nil {
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
		if err := deploy.CreateGCPSecretResources(ctx, kubeClient, metav1.NamespaceDefault, b); err != nil {
			return err
		}
	}
	return nil
}

// contextWithDeadline returns a context that will timeout before t.Deadline().
// See: https://github.com/golang/go/issues/36532
func contextWithDeadline(t *testing.T) context.Context {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return context.Background()
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline.Truncate(timeoutGracePeriod))
	t.Cleanup(cancel)
	return ctx
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

func copyImagesToLocalRegistry(ctx context.Context, t testing.TB, tag string, port int) error {
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
			t.Logf(">>> using latest image: %s", image)
			exists, err = imageExists(ctx, dockerClient, srcUntagged)
			if err != nil {
				return err
			}
			src = srcUntagged
		} else {
			src = srcTagged
		}
		if !exists {
			t.Logf(">>> skipping non-existent image: %s", image)
			return nil
		}

		dst := fmt.Sprintf("localhost:%d/%s:%s", port, image, tag)
		exists, err = imageExists(ctx, dockerClient, dst)
		if err != nil {
			return err
		}
		if exists {
			t.Logf(">>> skipping existent destination: %s", dst)
			return nil
		}

		t.Logf(">>> tagging and pushing image: %s", image)
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
