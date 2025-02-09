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
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/suite"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

const (
	// Arbitrary amount of time to let tests exit cleanly before the main process
	// terminates.
	timeoutGracePeriod = 10 * time.Second
)

var (
	projectID, location, cluster string
	skipGCM                      bool
	pollDuration                 time.Duration

	gcpServiceAccount string
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

	flag.Parse()

	if projectID == "" || location == "" || cluster == "" {
		metadata, err := suite.ExtractGKEClusterMeta()
		if err != nil {
			fmt.Printf("unable to extract GKE cluster: %s", err)
			os.Exit(1)
		}
		if projectID == "" {
			projectID = metadata.ProjectID
		}
		if location == "" {
			location = metadata.Location
		}
		if cluster == "" {
			cluster = metadata.Cluster
		}
	}

	os.Exit(m.Run())
}

func setupCluster(ctx context.Context, t testing.TB) (client.Client, *rest.Config, error) {
	t.Log(">>> setting up cluster")
	kubeClient, restConfig, err := suite.NewLocalClient()
	if err != nil {
		return nil, nil, err
	}

	t.Log(">>> deploying resources")
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
		if err := deploy.CreateGCPSecretResources(context.Background(), kubeClient, metav1.NamespaceDefault, b); err != nil {
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
