// Copyright 2023 Google LLC
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

// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

// Copied from https://github.com/thanos-io/thanos/tree/19dcc7902d2431265154cefff82426fbc91448a3/pkg/logging

package logginghttp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/efficientgo/core/testutil"
	"github.com/go-kit/log"
)

func TestHTTPServerMiddleware(t *testing.T) {
	b := bytes.Buffer{}

	m := NewHTTPServerMiddleware(log.NewLogfmtLogger(io.Writer(&b)))
	handler := func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, "Test Works")
		if err != nil {
			testutil.Ok(t, err)
		}
	}
	hm := m.WrapHandler("test", http.HandlerFunc(handler))

	u, err := url.Parse("http://example.com:5555/foo")
	testutil.Ok(t, err)
	req := &http.Request{
		Method: "GET",
		URL:    u,
		Body:   nil,
	}

	w := httptest.NewRecorder()

	hm(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	testutil.Ok(t, err)

	testutil.Equals(t, 200, resp.StatusCode)
	testutil.Equals(t, "Test Works", string(body))
	testutil.Assert(t, !strings.Contains(b.String(), "err="))

	// Typical way:
	req = httptest.NewRequest("GET", "http://example.com:5555/foo", nil)
	b.Reset()

	w = httptest.NewRecorder()
	hm(w, req)

	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	testutil.Ok(t, err)

	testutil.Equals(t, 200, resp.StatusCode)
	testutil.Equals(t, "Test Works", string(body))
	testutil.Assert(t, !strings.Contains(b.String(), "err="))

	// URL with no explicit port number in the format-hostname:port
	req = httptest.NewRequest("GET", "http://example.com/foo", nil)
	b.Reset()

	w = httptest.NewRecorder()
	hm(w, req)

	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	testutil.Ok(t, err)

	testutil.Equals(t, 200, resp.StatusCode)
	testutil.Equals(t, "Test Works", string(body))
	testutil.Assert(t, !strings.Contains(b.String(), "err="))
}
