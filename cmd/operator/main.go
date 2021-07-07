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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

func unstableFlagHelp(help string) string {
	return help + " (Setting this flag voids any guarantees of proper behavior of the operator.)"
}

// The valid levels for the --log-level flag.
const (
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

var (
	validLogLevels = []string{
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
	}
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	var (
		defaultProjectID string
		defaultCluster   string
	)
	if metadata.OnGCE() {
		defaultProjectID, _ = metadata.ProjectID()
		defaultCluster, _ = metadata.InstanceAttributeValue("cluster-name")
	}
	var (
		apiserverURL = flag.String("apiserver", "",
			"URL to the Kubernetes API server.")
		logLevel = flag.String("log-level", logLevelInfo,
			fmt.Sprintf("Log level to use. Possible values: %s", strings.Join(validLogLevels, ", ")))
		projectID = flag.String("project-id", defaultProjectID,
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

	logger, err := setupLogger(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Creating logger failed: %s", err)
		os.Exit(2)
	}

	cfg, err := clientcmd.BuildConfigFromFlags(*apiserverURL, *kubeconfig)
	if err != nil {
		level.Error(logger).Log("msg", "building kubeconfig failed", "err", err)
		os.Exit(1)
	}

	metrics := prometheus.NewRegistry()
	metrics.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

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
		level.Error(logger).Log("msg", "instantiating operator failed", "err", err)
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
					level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
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
	// Init and run admission controller server.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		server, err := op.InitAdmissionResources(ctx)
		cancel()
		if err != nil {
			level.Error(logger).Log("msg", "initialize admission resources", "err", err)
			os.Exit(1)
		}
		g.Add(func() (err error) {
			return server.ListenAndServeTLS("", "")
		}, func(err error) {
			ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
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
		level.Error(logger).Log("msg", "exit with error", "err", err)
		os.Exit(1)
	}
}

func setupLogger(lvl string) (log.Logger, error) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	switch lvl {
	case logLevelDebug:
		logger = level.NewFilter(logger, level.AllowDebug())
	case logLevelInfo:
		logger = level.NewFilter(logger, level.AllowInfo())
	case logLevelWarn:
		logger = level.NewFilter(logger, level.AllowWarn())
	case logLevelError:
		logger = level.NewFilter(logger, level.AllowError())
	default:
		return nil, errors.Errorf("log level %q unknown, must be one of (%s)", lvl, strings.Join(validLogLevels, ", "))
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	return logger, nil
}
