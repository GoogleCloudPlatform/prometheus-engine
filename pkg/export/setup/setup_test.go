// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package setup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
)

func TestFromFlags_NotOnGCE(t *testing.T) {
	// Asserting there is actually no GCE underneath.
	if metadata.OnGCE() {
		t.Skip("This test can't run on GCP or Cloudtop; we expect no metadata server.")
	}

	fake := kingpin.New("test", "test")
	newExport := FromFlags(fake, "product")
	// Our readines is default (3 * 10s), so ensure FromFlags is never longer than 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fake app invocation.
	if _, err := fake.Parse(nil); err != nil {
		t.Fatal(err)
	}

	// Make sure constructor does is not stuck forever.
	_, _ = newExport(ctx, log.NewLogfmtLogger(os.Stderr), nil)
}

// Regression test for b/344740239. We ensure that even stuck metadata servers
// calls will timeout correctly (we propagate context properly).
func TestTryPopulateUnspecifiedFromMetadata(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	// Setup fake HTTP server taking forever.
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		wg.Wait()
	}))
	t.Cleanup(func() {
		wg.Done()
		s.Close()
	})

	// Main "readiness" like timeout that we have to be faster than.
	testCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Inject metadata URL for our server (does not matter if 404).
	t.Setenv("GCE_METADATA_HOST", s.Listener.Addr().String())

	// We expect this to finish sooner.
	ctx, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()
	opts := export.ExporterOpts{}
	tryPopulateUnspecifiedFromMetadata(ctx, log.NewLogfmtLogger(os.Stderr), &opts)
	if diff := cmp.Diff(export.ExporterOpts{}, opts); diff != "" {
		t.Fatal("expected no options populated", diff)
	}
	// We expect to finish the test, before testCtx is cancelled.
	// TODO(bwplotka): Assert we are not exiting faster because metadata cannot access our HTTP test server.
	// I checked manually for inverse to fail (e.g. setting in ctx longer than textCtx).
	if testCtx.Err() != nil {
		t.Fatal("tryPopulateUnspecifiedFromMetadata took 30s to complete, it should timeout after 1s")
	}
}
