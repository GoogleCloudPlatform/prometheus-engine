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
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/e2e"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	logVerbosity := 0
	addr := ":8080"
	metricServiceAddr := ":8081"

	flag.IntVar(&logVerbosity, "v", logVerbosity, "Logging verbosity")
	flag.StringVar(&addr, "addr", addr, "Address to serve probe statuses (e.g. /readyz and /livez) and /metrics")
	flag.StringVar(&metricServiceAddr, "metric-service-addr", metricServiceAddr, "Address to serve a mock metric service server.")
	flag.Parse()

	logger := zap.New(zap.Level(zapcore.Level(-logVerbosity)))
	ctrl.SetLogger(logger)

	ctx := signals.SetupSignalHandler()
	if err := run(ctx, logger, addr, metricServiceAddr); err != nil {
		logger.Error(err, "exit with error")
		os.Exit(1)
	}
}

func run(ctx context.Context, logger logr.Logger, probeAddr, metricServiceAddr string) error {
	listener, err := net.Listen("tcp", metricServiceAddr)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	errs := make(chan error, 1)

	logger.Info("starting server...")
	metricDatabase := e2e.NewMetricDatabase()

	{
		server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			now := time.Now()
			logger.Info("grpc request", "method", info.FullMethod, "time", now, "data", prototext.Format(req.(proto.Message)))
			resp, err := handler(ctx, req)
			logger.Info("grpc response", "method", info.FullMethod, "time", now, "duration", time.Since(now))
			if err != nil {
				logger.Error(err, "grpc failure", "method", info.FullMethod, "time", now)
			}
			return resp, err
		}))
		monitoringpb.RegisterMetricServiceServer(server, e2e.NewFakeMetricServer(metricDatabase))

		wg.Add(1)
		go func() {
			for range ctx.Done() {
				server.GracefulStop()
				return
			}
		}()
		go func() {
			defer wg.Done()
			if err := server.Serve(listener); err != nil {
				errs <- err
			}
		}()
	}

	{
		registry := prometheus.NewRegistry()
		registry.MustRegister(e2e.NewMetricCollector(logger, export.MetricTypePrefix, metricDatabase))

		wg.Add(1)
		mux := http.NewServeMux()
		mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry}))

		server := http.Server{
			Addr:    probeAddr,
			Handler: mux,
		}
		go func() {
			for range ctx.Done() {
				// Start new context because ours is done.
				if err := server.Shutdown(context.Background()); err != nil {
					errs <- err
				}
			}
		}()
		go func() {
			defer wg.Done()
			if err := server.ListenAndServe(); err != nil {
				errs <- err
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	for err := range errs {
		return err
	}
	return nil
}
