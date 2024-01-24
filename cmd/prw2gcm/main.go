// Copyright 2024 Google LLC
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
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export/setup"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

var (
	credentialsFile = flag.String("gcm.credentials-file", "",
		"File with JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.")
	gcmEndpoint = flag.String("gcm.endpoint", "monitoring.googleapis.com:443",
		"GCM API endpoint to send metric data to.")
	listenAddress = flag.String("listen-address", ":19091",
		"Address on which to expose metrics and the Remote Write handler.")
	unsafeAllowClassicHistograms = flag.Bool("unsafe.allow-classic-histograms", false, "Don't reject classic histogram series. Enable only if you understand the lack of self-contained histogram risks. Additionally, if enabled, more memory resources will be required by proxy.")
)

func newGCMClient(ctx context.Context, endpoint, ua string, credsFile string) (*monitoring.MetricClient, error) {
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor)),
		option.WithUserAgent(ua),
		option.WithCredentialsFile(credsFile),
	}
	if endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(endpoint))
	}
	return monitoring.NewMetricClient(ctx, clientOpts...)
}

func main() {
	flag.Parse()

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	metrics := prometheus.NewRegistry()
	metrics.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	var g run.Group
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
	{
		// TODO(bwplotk): Yolo, double check.
		ver, err := export.Version()
		if err != nil {
			level.Error(logger).Log("msg", "detect version", "err", err)
			os.Exit(1)
		}

		env := setup.UAEnvUnspecified
		// Default target fields if we can detect them in GCP.
		if metadata.OnGCE() {
			env = setup.UAEnvGCE
			cluster, _ := metadata.InstanceAttributeValue("cluster-name")
			if cluster != "" {
				env = setup.UAEnvGKE
			}
		}

		// Identity User Agent for all gRPC requests.
		ua := strings.TrimSpace(fmt.Sprintf("%s/%s %s (env:%s;mode:%s)",
			"prometheus-engine-prw2gcm", ver, "prw2-gcm", env, "unspecified"))

		ctx, cancel := context.WithCancel(context.Background())
		client, err := newGCMClient(ctx, *gcmEndpoint, ua, *credentialsFile)
		if err != nil {
			defer cancel()
			level.Error(logger).Log("msg", "create GCM client", "err", err)
			os.Exit(1)
		}

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))
		mux.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "prw2gcm is Healthy.\n")
		})
		mux.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "prw2gcm is Ready.\n")
		})
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		registerRWHandler(mux, client, logger, *unsafeAllowClassicHistograms)
		server := &http.Server{
			Handler: mux,
			Addr:    *listenAddress,
		}

		g.Add(func() error {
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, _ = context.WithTimeout(ctx, time.Minute)
			_ = server.Shutdown(ctx)
			cancel()
		})
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "running reloader failed", "err", err)
		os.Exit(1)
	}
}
