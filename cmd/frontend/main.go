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

// A proxy that forwards incoming requests to an HTTP endpoint while authenticating
// it with static service account credentials or the default service account on GCE
// instances.
// It's primarily intended to authenticate Prometheus queries coming from Grafana against
// GPE as Grafana has no option to configure OAuth2 credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/ui"
)

const projectIDVar = "PROJECT_ID"

var (
	projectID = flag.String("query.project-id", "",
		"Project ID of the Google Cloud Monitoring workspace project to query.")

	credentialsFile = flag.String("query.credentials-file", "",
		"JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.")

	listenAddress = flag.String("web.listen-address", ":19090",
		"Address on which to expose metrics and the query UI.")

	targetURLStr = flag.String("query.target-url", fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", projectIDVar),
		fmt.Sprintf("The URL to forward authenticated requests to. (%s is replaced with the --project-id flag.)", projectIDVar))
)

func main() {
	flag.Parse()

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	metrics := prometheus.NewRegistry()
	metrics.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	if *projectID == "" {
		level.Error(logger).Log("msg", "--project-id must be set")
		os.Exit(1)
	}

	targetURL, err := url.Parse(strings.ReplaceAll(*targetURLStr, projectIDVar, *projectID))
	if err != nil {
		level.Error(logger).Log("msg", "parsing target URL failed", "err", err)
		os.Exit(1)
	}

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
		opts := []option.ClientOption{
			option.WithScopes("https://www.googleapis.com/auth/monitoring.read"),
			option.WithCredentialsFile(*credentialsFile),
		}
		ctx, cancel := context.WithCancel(context.Background())

		transport, err := apihttp.NewTransport(ctx, http.DefaultTransport, opts...)
		if err != nil {
			level.Error(logger).Log("msg", "create proxy HTTP transport", "err", err)
			os.Exit(1)
		}

		server := &http.Server{Addr: *listenAddress}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))
		http.Handle("/api/", forward(logger, targetURL, transport))

		http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Prometheus frontend is Healthy.\n")
		})
		http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Prometheus frontend is Ready.\n")
		})

		http.Handle("/", ui.Handler())

		g.Add(func() error {
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, _ = context.WithTimeout(ctx, time.Minute)
			server.Shutdown(ctx)
			cancel()
		})
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "running reloader failed", "err", err)
		os.Exit(1)
	}
}

func forward(logger log.Logger, target *url.URL, transport http.RoundTripper) http.Handler {
	client := http.Client{Transport: transport}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		u := *target
		u.Path = path.Join(u.Path, req.URL.Path)
		u.RawQuery = req.URL.RawQuery

		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), req.Body)
		if err != nil {
			level.Warn(logger).Log("msg", "creating request failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		copyHeader(newReq.Header, req.Header)

		resp, err := client.Do(newReq)
		if err != nil {
			level.Warn(logger).Log("msg", "requesting GCM failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		defer resp.Body.Close()
		if _, err := io.Copy(w, resp.Body); err != nil {
			level.Warn(logger).Log("msg", "copying response body failed", "err", err)
			return
		}
	})
}

func copyHeader(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}
