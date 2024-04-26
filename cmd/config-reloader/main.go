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
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thanos-io/thanos/pkg/reloader"
)

func main() {
	var (
		watchedDirs      stringSlice
		configFile       = flag.String("config-file", "", "config file to watch for changes")
		configFileOutput = flag.String("config-file-output", "", "config file to write with interpolated environment variables")
		configDir        = flag.String("config-dir", "", "config directory to watch for changes")
		configDirOutput  = flag.String("config-dir-output", "", "config directory to write with interpolated environment variables")
		// Ready and reload endpoints should be compatible with Prometheus-style
		// management APIs, e.g.
		// https://prometheus.io/docs/prometheus/latest/management_api/
		// https://prometheus.io/docs/alerting/latest/management_api/
		reloadURLStr                      = flag.String("reload-url", "http://127.0.0.1:19090/-/reload", "reload endpoint of the configuration target that triggers a reload of the configuration file")
		readyURLStr                       = flag.String("ready-url", "http://127.0.0.1:19090/-/ready", "ready endpoint of the configuration target that returns a 200 when ready to serve traffic. If set, the config-reloader will probe it on startup")
		readyProbingInterval              = flag.Duration("ready-startup-probing-interval", 1*time.Second, "how often to poll ready endpoint during startup")
		readyProbingNoConnectionThreshold = flag.Int("ready-startup-probing-no-conn-threshold", 5, "how many times ready endpoint can fail due to no connection failure. This can happen if the config-reloader starts faster than the config target endpoint readiness server.")

		listenAddress = flag.String("listen-address", ":19091", "address on which to expose metrics")
	)
	flag.Var(&watchedDirs, "watched-dir", "directory to watch for file changes (for rule and secret files, may be repeated)")

	flag.Parse()

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	if *configDirOutput != "" && *configDir == "" {
		//nolint:errcheck
		level.Error(logger).Log("msg", "config-dir-output specified without config-dir")
		os.Exit(1)
	}

	metrics := prometheus.NewRegistry()
	metrics.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	reloadURL, err := url.Parse(*reloadURLStr)
	if err != nil {
		//nolint:errcheck
		level.Error(logger).Log("msg", "parsing reloader URL failed", "err", err)
		os.Exit(1)
	}

	// Set up interrupt signal handler.
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	// Poll ready endpoint indefinitely until it's up and running.
	req, err := http.NewRequest(http.MethodGet, *readyURLStr, nil)
	if err != nil {
		//nolint:errcheck
		level.Error(logger).Log("msg", "creating request", "err", err)
		os.Exit(1)
	}

	var (
		ticker                       = time.NewTicker(*readyProbingInterval)
		acceptableNoConnectionErrors = *readyProbingNoConnectionThreshold
		done                         = make(chan bool)
	)

	go func() {
		//nolint:errcheck
		level.Info(logger).Log("msg", "ensure ready-url is healthy")
		for {
			select {
			case <-term:
				//nolint:errcheck
				level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
				os.Exit(0)
			case <-ticker.C:
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					if acceptableNoConnectionErrors <= 0 {
						//nolint:errcheck
						level.Error(logger).Log("msg", "polling ready-url", "err", err, "no-connection-threshold", *readyProbingNoConnectionThreshold)
						os.Exit(1)
					}
					acceptableNoConnectionErrors--
					continue
				}
				if err := resp.Body.Close(); err != nil {
					//nolint:errcheck
					level.Warn(logger).Log("msg", "unable to close response body", "err", err)
				}
				if resp.StatusCode == http.StatusOK {
					//nolint:errcheck
					level.Info(logger).Log("msg", "ready-url is healthy")
					ticker.Stop()
					done <- true
					return
				}
			}
		}
	}()
	<-done

	var cfgDirs []reloader.CfgDirOption
	if *configDir != "" {
		cfgDirs = append(cfgDirs, reloader.CfgDirOption{
			Dir:       *configDir,
			OutputDir: *configDirOutput,
		})
	}

	rel := reloader.New(
		logger,
		metrics,
		&reloader.Options{
			ReloadURL:     reloadURL,
			CfgFile:       *configFile,
			CfgOutputFile: *configFileOutput,
			CfgDirs:       cfgDirs,
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
		cancel := make(chan struct{})
		g.Add(
			func() error {
				select {
				case <-term:
					//nolint:errcheck
					level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
				case <-cancel:
				}
				return nil
			},
			func(error) {
				close(cancel)
			},
		)
	}
	{
		server := &http.Server{Addr: *listenAddress}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))

		g.Add(func() error {
			//nolint:errcheck
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if err := server.Shutdown(ctx); err != nil {
				//nolint:errcheck
				level.Error(logger).Log("msg", "Server failed to shut down gracefully.")
			}
			cancel()
		})
	}

	if err := g.Run(); err != nil {
		//nolint:errcheck
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
