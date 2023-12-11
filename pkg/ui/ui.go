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

package ui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/prometheus/common/server"
)

func Handler(externalURL *url.URL) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// The "/" pattern matches everything, so we need to check
		// that we're at the root here.
		if r.URL.Path == "/" {
			http.Redirect(w, r, path.Join(externalURL.Path, "graph"), http.StatusFound)
		}
	})

	// Serve UI index.
	var reactRouterPaths = []string{
		"/graph",
	}
	for _, p := range reactRouterPaths {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			f, err := Assets.Open("/static/react/index.html")
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Error opening React index.html: %v", err)
				return
			}
			idx, err := io.ReadAll(f)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Error reading React index.html: %v", err)
				return
			}
			idx = bytes.ReplaceAll(idx, []byte("CONSOLES_LINK_PLACEHOLDER"), []byte(""))
			idx = bytes.ReplaceAll(idx, []byte("TITLE_PLACEHOLDER"), []byte("Google Cloud Managed Service for Prometheus"))

			if _, err := w.Write(idx); err != nil {
				fmt.Fprintf(w, "Error writing bytes: %s", err)
			}
		})
	}

	// Serve static assets.
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = path.Join("/static/react/", r.URL.Path)

		server.StaticFileServer(Assets).ServeHTTP(w, r)
	})

	return mux
}
