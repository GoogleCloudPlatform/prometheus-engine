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

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

func unstableFlagHelp(help string) string {
	return help + " (Setting this flag voids any guarantees of proper behavior of the operator.)"
}

func main() {
	var (
		defaultProjectID string
		defaultCluster   string
	)
	if metadata.OnGCE() {
		defaultProjectID, _ = metadata.ProjectID()
		defaultCluster, _ = metadata.InstanceAttributeValue("cluster-name")
	}
	var (
		logVerbosity = flag.Int("v", 0, "Logging verbosity")
		projectID    = flag.String("project-id", defaultProjectID,
			"Project ID of the cluster.")
		cluster = flag.String("cluster", defaultCluster,
			"Name of the cluster the operator acts on.")
		operatorNamespace = flag.String("operator-namespace", operator.DefaultOperatorNamespace,
			"Namespace in which the operator manages its resources.")

		imageCollector = flag.String("image-collector", operator.ImageCollector,
			unstableFlagHelp("Override for the container image of the collector."))
		imageConfigReloader = flag.String("image-config-reloader", operator.ImageConfigReloader,
			unstableFlagHelp("Override for the container image of the config reloader."))
		priorityClass = flag.String("priority-class", "",
			"Priority class at which the collector pods are run.")
		gcmEndpoint = flag.String("cloud-monitoring-endpoint", "",
			"Override for the Cloud Monitoring endpoint to use for all collectors.")
		caSelfSign = flag.Bool("ca-selfsign", true,
			"Whether to self-sign or have kube-apiserver sign certificate key pair for TLS.")
		webhookAddr = flag.String("webhook-addr", ":8443",
			"Address to listen to for incoming kube admission webhook connections.")
		metricsAddr = flag.String("metrics-addr", ":8080", "Address to emit metrics on.")
	)
	flag.Parse()

	logger := zap.New(zap.Level(zapcore.Level(-*logVerbosity)))
	ctrl.SetLogger(logger)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "loading kubeconfig failed")
		os.Exit(1)
	}

	// controller-runtime creates a registry against which its metrics are registered globally.
	// Using it as our non-global registry is the easiest way to combine metrics into a single
	// /metrics endpoint.
	// It already has the GoCollector and ProcessCollector metrics installed.
	metrics := ctrlmetrics.Registry

	op, err := operator.New(logger, cfg, metrics, operator.Options{
		ProjectID:               *projectID,
		Cluster:                 *cluster,
		OperatorNamespace:       *operatorNamespace,
		ImageCollector:          *imageCollector,
		ImageConfigReloader:     *imageConfigReloader,
		PriorityClass:           *priorityClass,
		CloudMonitoringEndpoint: *gcmEndpoint,
		CASelfSign:              *caSelfSign,
		ListenAddr:              *webhookAddr,
	})
	if err != nil {
		logger.Error(err, "instantiating operator failed")
		os.Exit(1)
	}

	var g run.Group
	// Termination handler.
	{
		term := make(chan os.Signal, 1)
		cancel := make(chan struct{})
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)

		g.Add(
			func() error {
				select {
				case <-term:
					logger.Info("received SIGTERM, exiting gracefully...")
				case <-cancel:
				}
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	// Operator monitoring.
	{
		server := &http.Server{Addr: *metricsAddr}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))
		g.Add(func() error {
			return server.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			server.Shutdown(ctx)
			cancel()
		})
	}
	// Main operator loop.
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			return op.Run(ctx)
		}, func(err error) {
			cancel()
		})
	}
	if err := g.Run(); err != nil {
		logger.Error(err, "exit with error")
		os.Exit(1)
	}
}
