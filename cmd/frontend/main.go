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

// A proxy that forwards incoming requests to an HTTP endpoint while authenticating
// it with static service account credentials or the default service account on GCE
// instances.
// It's primarily intended to authenticate Prometheus queries coming from Grafana against
// GMP as Grafana has no option to configure OAuth2 credentials.
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

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/ui"
)

const projectIDVar = "PROJECT_ID"

var (
	authUsernameEnv = "AUTH_USERNAME"
	authPasswordEnv = "AUTH_PASSWORD"

	projectID = flag.String("query.project-id", "",
		"Project ID of the Google Cloud Monitoring workspace project to query.")

	credentialsFile = flag.String("query.credentials-file", "",
		"JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.")

	listenAddress = flag.String("web.listen-address", ":19090",
		"Address on which to expose metrics and the query UI.")

	externalURLStr = flag.String("web.external-url", "", "The URL under which the frontend is externally reachable (for example, if it is served via a reverse proxy). Used for generating relative and absolute links back to the frontend itself. If the URL has a path portion, it will be used to prefix served HTTP endpoints. If omitted, relevant URL components will be derived automatically.")

	targetURLStr = flag.String("query.target-url", fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", projectIDVar),
		fmt.Sprintf("The URL to forward authenticated requests to. (%s is replaced with the --query.project-id flag.)", projectIDVar))
)

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

	if *projectID == "" {
		//nolint:errcheck
		level.Error(logger).Log("msg", "--query.project-id must be set")
		os.Exit(1)
	}

	targetURL, err := url.Parse(strings.ReplaceAll(*targetURLStr, projectIDVar, *projectID))
	if err != nil {
		//nolint:errcheck
		level.Error(logger).Log("msg", "parsing target URL failed", "err", err)
		os.Exit(1)
	}

	externalURL, err := url.Parse(*externalURLStr)
	if err != nil {
		//nolint:errcheck
		level.Error(logger).Log("msg", "parsing external URL failed", "err", err)
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
					//nolint:errcheck
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
			//nolint:errcheck
			level.Error(logger).Log("msg", "create proxy HTTP transport", "err", err)
			os.Exit(1)
		}

		server := &http.Server{Addr: *listenAddress}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics}))
		http.Handle("/api/", authenticate(forward(logger, targetURL, transport)))

		http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Prometheus frontend is Healthy.\n")
		})
		http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Prometheus frontend is Ready.\n")
		})

		http.Handle("/", authenticate(ui.Handler(externalURL)))

		g.Add(func() error {
			//nolint:errcheck
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, _ = context.WithTimeout(ctx, time.Minute)
			if err := server.Shutdown(ctx); err != nil {
				//nolint:errcheck
				level.Error(logger).Log("msg", "Server failed to shut down gracefully")
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

func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username := os.Getenv(authUsernameEnv)
		password := os.Getenv(authPasswordEnv)
		if len(username) > 0 && len(password) > 0 {
			reqUser, reqPass, ok := req.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", "Basic")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if reqUser != username || reqPass != password {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, req)
	})
}

func forward(logger log.Logger, target *url.URL, transport http.RoundTripper) http.Handler {
	client := http.Client{Transport: transport}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		u := *target
		u.Path = path.Join(u.Path, req.URL.Path)

		method := req.Method
		// Write all params into the URL and make a GET request to work around
		// /api/v1/series currently not accepting match[] params on POST.
		if req.URL.Path == "/api/v1/series" {
			method = "GET"
			if err := req.ParseForm(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
			u.RawQuery = req.Form.Encode()
		} else {
			u.RawQuery = req.URL.RawQuery
		}

		newReq, err := http.NewRequestWithContext(req.Context(), method, u.String(), req.Body)
		if err != nil {
			//nolint:errcheck
			level.Warn(logger).Log("msg", "creating request failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		copyHeader(newReq.Header, req.Header)

		resp, err := client.Do(newReq)
		if err != nil {
			//nolint:errcheck
			level.Warn(logger).Log("msg", "requesting GCM failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		defer resp.Body.Close()
		if _, err := io.Copy(w, resp.Body); err != nil {
			//nolint:errcheck
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
