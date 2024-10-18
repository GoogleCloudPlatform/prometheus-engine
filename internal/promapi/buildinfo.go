// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promapi

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
)

const (
	timestampFormat              = "20060102-15:04:05"
	buildinfoPath                = "/api/v1/status/buildinfo"
	premoetheusDepencencyVersion = "1.8.2" // current version of the prometheus/prometheus dependency.
)

// BuildinfoHandlerFunc simulates the /api/v1/status/buildinfo prometheus endpoint.
// It is used by Grafana to determine the Prometheus flavor, e.g. to check whether the ruler-api is enabled.
// binary: e.g. "frontend" or "rule-evaluator".
func BuildinfoHandlerFunc(logger log.Logger, binaryName, binaryVersion string) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		// TODO(yama6a): Populate buildinfo at build time, analogous to: https://github.com/prometheus/common/blob/v0.60.0/version/info.go
		response := promapiv1.PrometheusVersion{
			Version:   premoetheusDepencencyVersion,
			Revision:  fmt.Sprintf("gmp/%s-%s", binaryName, binaryVersion),
			Branch:    "HEAD",
			BuildUser: "gmp@localhost",
			BuildDate: getBinaryCreatedTimestamp(logger),
			GoVersion: runtime.Version(),
		}
		WriteSuccessResponse(logger, w, http.StatusOK, buildinfoPath, response)
	}
}

func getBinaryCreatedTimestamp(logger log.Logger) string {
	fileInfo, err := os.Stat(os.Args[0])
	if err != nil {
		level.Error(logger).Log("msg", "Failed to get binary creation timestamp, usinng now()", "err", err)
		return time.Now().Format(timestampFormat)
	}

	return fileInfo.ModTime().Format(timestampFormat)
}
