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
		defaultLocation  string
	)
	if metadata.OnGCE() {
		defaultProjectID, _ = metadata.ProjectID()
		defaultCluster, _ = metadata.InstanceAttributeValue("cluster-name")
		defaultLocation, _ = metadata.InstanceAttributeValue("cluster-location")
	}
	var (
		logVerbosity      = flag.Int("v", 0, "Logging verbosity")
		projectID         = flag.String("project-id", defaultProjectID, "Project ID of the cluster. May be left empty on GKE.")
		location          = flag.String("location", defaultLocation, "GCP location of the cluster. Maybe be left empty on GKE.")
		cluster           = flag.String("cluster", defaultCluster, "Name of the cluster the operator acts on. May be left empty on GKE.")
		operatorNamespace = flag.String("operator-namespace", operator.DefaultOperatorNamespace,
			"Namespace in which the operator manages its resources.")
		publicNamespace = flag.String("public-namespace", operator.DefaultPublicNamespace,
			"Namespace in which the operator reads user-provided resources.")

		imageCollector = flag.String("image-collector", operator.ImageCollector,
			unstableFlagHelp("Override for the container image of the collector."))
		imageConfigReloader = flag.String("image-config-reloader", operator.ImageConfigReloader,
			unstableFlagHelp("Override for the container image of the config reloader."))
		imageRuleEvaluator = flag.String("image-rule-evaluator", operator.ImageRuleEvaluator,
			unstableFlagHelp("Override for the container image of the rule evaluator."))

		hostNetwork = flag.Bool("host-network", false,
			"Whether pods are deployed with hostNetwork enabled. If true, GKE clusters with Workload Identity will not require additional permission for the components deployed by the operator. Must be false on GKE Autopilot clusters.")
		priorityClass = flag.String("priority-class", "",
			"Priority class at which the collector pods are run.")
		gcmEndpoint = flag.String("cloud-monitoring-endpoint", "",
			"Override for the Cloud Monitoring endpoint to use for all collectors.")
		tlsCert     = flag.String("tls-cert-base64", "", "The base64-encoded TLS certificate.")
		tlsKey      = flag.String("tls-key-base64", "", "The base64-encoded TLS key.")
		caCert      = flag.String("ca-cert-base64", "", "The base64-encoded certificate authority.")
		webhookAddr = flag.String("webhook-addr", ":10250",
			"Address to listen to for incoming kube admission webhook connections.")
		metricsAddr = flag.String("metrics-addr", ":18080", "Address to emit metrics on.")

		collectorMemoryResource = flag.Int64("collector-memory-resource", 200, "The Memory Resource of collector pod, in mega bytes")
		collectorMemoryLimit    = flag.Int64("collector-memory-limit", 3000, "The Memory Limit of collector pod, in mega bytes.")
		collectorCPUResource    = flag.Int64("collector-cpu-resource", 100, "The CPU Resource of collector pod, in milli cpu.")
		evaluatorMemoryResource = flag.Int64("evaluator-memory-resource", 200, "The Memory Resource of evaluator pod, in mega bytes.")
		evaluatorMemoryLimit    = flag.Int64("evaluator-memory-limit", 1000, "The Memory Limit of evaluator pod, in mega bytesv.")
		evaluatorCPUResource    = flag.Int64("evaluator-cpu-resource", 100, "The CPU Resource of evaluator pod, in milli cpu.")
		mode                    = flag.String("mode", "kubectl", "how managed collection was provisioned.")
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
		Location:                *location,
		Cluster:                 *cluster,
		OperatorNamespace:       *operatorNamespace,
		PublicNamespace:         *publicNamespace,
		ImageCollector:          *imageCollector,
		ImageConfigReloader:     *imageConfigReloader,
		ImageRuleEvaluator:      *imageRuleEvaluator,
		HostNetwork:             *hostNetwork,
		PriorityClass:           *priorityClass,
		CloudMonitoringEndpoint: *gcmEndpoint,
		TLSCert:                 *tlsCert,
		TLSKey:                  *tlsKey,
		CACert:                  *caCert,
		ListenAddr:              *webhookAddr,
		CollectorMemoryResource: *collectorMemoryResource,
		CollectorMemoryLimit:    *collectorMemoryLimit,
		CollectorCPUResource:    *collectorCPUResource,
		EvaluatorCPUResource:    *evaluatorCPUResource,
		EvaluatorMemoryResource: *evaluatorMemoryResource,
		EvaluatorMemoryLimit:    *evaluatorMemoryLimit,
		Mode:                    *mode,
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
