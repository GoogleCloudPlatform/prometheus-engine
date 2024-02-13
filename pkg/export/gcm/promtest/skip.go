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

package promtest

import (
	"testing"
	"time"

	"github.com/efficientgo/e2e"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dto "github.com/prometheus/client_model/go"
)

type noopBackend struct{}

func (n noopBackend) Ref() string {
	return "noop"
}

func (n noopBackend) start(t testing.TB, env e2e.Environment) (api v1.API, extraLset map[string]string) {
	return
}

func (n noopBackend) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration) {
}

// NoopBackend creates noop backend, useful when you want to skip one backend for
// local debugging purpose without changing test significantly.
func NoopBackend() noopBackend { return noopBackend{} }
