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
	"crypto/fips140"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/oklog/run"
	versioninfo "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

const (
	defaultTLSDir = "/etc/tls/private"
)

func main() {
	ctx := context.Background()

	var (
		defaultProjectID string
		defaultCluster   string
		defaultLocation  string
	)
	errList := []error{}
	if metadata.OnGCE() {
		var err error
		defaultProjectID, err = metadata.ProjectIDWithContext(ctx)
		errList = append(errList, err)
		defaultCluster, err = metadata.InstanceAttributeValueWithContext(ctx, "cluster-name")
		errList = append(errList, err)
		defaultLocation, err = metadata.InstanceAttributeValueWithContext(ctx, "cluster-location")
		errList = append(errList, err)
	}
	var (
		logVerbosity      = flag.Int("v", 0, "Logging verbosity")
		projectID         = flag.String("project-id", defaultProjectID, "Project ID of the cluster. May be left empty on GKE.")
		location          = flag.String("location", defaultLocation, "Google Cloud region or zone where your data will be stored. May be left empty on GKE.")
		cluster           = flag.String("cluster", defaultCluster, "Name of the cluster the operator acts on. May be left empty on GKE.")
		operatorNamespace = flag.String("operator-namespace", operator.DefaultOperatorNamespace,
			"Namespace in which the operator manages its resources.")
		publicNamespace = flag.String("public-namespace", operator.DefaultPublicNamespace,
			"Namespace in which the operator reads user-provided resources.")

		tlsCert     = flag.String("tls-cert-base64", "", "The base64-encoded TLS certificate.")
		tlsKey      = flag.String("tls-key-base64", "", "The base64-encoded TLS key.")
		caCert      = flag.String("ca-cert-base64", "", "The base64-encoded certificate authority.")
		certDir     = flag.String("cert-dir", defaultTLSDir, "The directory which contains TLS certificates for webhook server.")
		webhookAddr = flag.String("webhook-addr", ":10250",
			"Address to listen to for incoming kube admission webhook connections.")
		probeAddr   = flag.String("probe-addr", ":18081", "Address to outputs probe statuses (e.g. /readyz and /healthz)")
		metricsAddr = flag.String("metrics-addr", ":18080", "Address to emit metrics on.")

		// Permit the operator to cleanup previously-managed resources that
		// are missing the provided annotation. An empty string disables this
		// feature.
		cleanupAnnotKey = flag.String("cleanup-unless-annotation-key", "",
			"Clean up operator-managed workloads without the provided annotation key.")
	)
	flag.Parse()

	logger := zap.New(zap.Level(zapcore.Level(-*logVerbosity)))
	ctrl.SetLogger(logger)
	if err := errors.Join(errList...); err != nil {
		logger.Error(err, "unable to fetch Google Cloud metadata")
	}

	if !fips140.Enabled() {
		logger.Error(errors.New("FIPS mode required"), "FIPS mode not enabled")
		os.Exit(1)
	}

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
	metrics.MustRegister(versioninfo.NewCollector("operator")) // Add build_info metric.

	op, err := operator.New(logger, cfg, operator.Options{
		ProjectID:         *projectID,
		Location:          *location,
		Cluster:           *cluster,
		OperatorNamespace: *operatorNamespace,
		PublicNamespace:   *publicNamespace,
		ProbeAddr:         *probeAddr,
		TLSCert:           *tlsCert,
		TLSKey:            *tlsKey,
		CACert:            *caCert,
		CertDir:           *certDir,
		ListenAddr:        *webhookAddr,
		CleanupAnnotKey:   *cleanupAnnotKey,
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
			func(error) {
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
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if err := server.Shutdown(ctx); err != nil {
				logger.Error(err, "Server failed to shut down gracefully.")
			}
			cancel()
		})
	}
	// Main operator loop.
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			return op.Run(ctx, metrics)
		}, func(error) {
			cancel()
		})
	}
	if err := g.Run(); err != nil {
		logger.Error(err, "exit with error")
		os.Exit(1)
	}
}
