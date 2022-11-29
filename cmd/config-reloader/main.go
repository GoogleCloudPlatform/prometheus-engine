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
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thanos-io/thanos/pkg/reloader"
	"k8s.io/apimachinery/pkg/util/wait"
)

func main() {
	var (
		watchedDirs      stringSlice
		configFile       = flag.String("config-file", "", "config file to watch for changes")
		configFileOutput = flag.String("config-file-output", "", "config file to write with interpolated environment variables")
		reloadURLStr     = flag.String("reload-url", "http://127.0.0.1:19090/-/reload", "Prometheus reload endpoint")
		listenAddress    = flag.String("listen-address", ":19091", "address on which to expose metrics")
	)
	flag.Var(&watchedDirs, "watched-dir", "directory to watch for file changes (for rule and secret files, may be repeated)")

	flag.Parse()

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	metrics := prometheus.NewRegistry()
	metrics.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	reloadURL, err := url.Parse(*reloadURLStr)
	if err != nil {
		level.Error(logger).Log("msg", "parsing reloader URL failed", "err", err)
		os.Exit(1)
	}

	// Poll Prometheus's ready endpoint until it's up and running.
	readyURLStr := strings.Replace(*reloadURLStr, "reload", "ready", 1)
	req, _ := http.NewRequest(http.MethodGet, readyURLStr, nil)
	if err := wait.Poll(2*time.Second, 2*time.Minute, func() (bool, error) {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false, err
		}
		if resp.StatusCode == http.StatusOK {
			level.Info(logger).Log("msg", "Prometheus is ready")
			return true, nil
		}
		return false, nil
	}); err != nil {
		level.Error(logger).Log("msg", "waiting for Prometheus ready", "err", err)
		os.Exit(1)
	}

	rel := reloader.New(
		logger,
		metrics,
		&reloader.Options{
			ReloadURL:     reloadURL,
			CfgFile:       *configFile,
			CfgOutputFile: *configFileOutput,
			WatchedDirs:   watchedDirs,
			// There are some reliability issues with fsnotify picking up file changes.
			// Configure a very aggress refresh for now. The reloader will only send reload signals
			// to Prometheus if the contents actually changed. So this should not have any practical
			// drawbacks.
			WatchInterval: 10 * time.Second,
			RetryInterval: 5 * time.Second,
			DelayInterval: 3 * time.Second,
		},
	)

	var g run.Group
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			return rel.Watch(ctx)
		}, func(error) {
			cancel()
		})
	}
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
		server := &http.Server{Addr: *listenAddress}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))

		g.Add(func() error {
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			server.Shutdown(ctx)
			cancel()
		})
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "running reloader failed", "err", err)
		os.Exit(1)
	}
}

type stringSlice []string

func (ss *stringSlice) String() string {
	return strings.Join(*ss, ", ")
}

func (ss *stringSlice) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}
