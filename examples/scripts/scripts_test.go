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

package scripts

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	"github.com/oklog/ulid"
	"google.golang.org/api/iterator"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/efficientgo/core/runutil"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// GCMServiceAccountOrFail gets the Google SA JSON content from GCM_SECRET
// environment variable or fails.
func gcmServiceAccountOrFail(t testing.TB) []byte {
	// TODO(bwplotka): Move it to https://cloud.google.com/build CI.
	saJSON := []byte(os.Getenv("GCM_SECRET"))
	if len(saJSON) == 0 {
		t.Fatal("gcmServiceAccountOrFail: no GCM_SECRET env var provided, can't run the test")
	}
	return saJSON
}

func assertExpectedTypesExists(t *testing.T, client *gcm.MetricClient, reqName string, expectedTypes []string) {
	t.Helper()

	tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runutil.RetryWithLog(log.NewLogfmtLogger(os.Stderr), 200*time.Second, tctx.Done(), func() error {
		it := client.ListMetricDescriptors(
			tctx,
			&monitoringpb.ListMetricDescriptorsRequest{
				Name:   reqName,
				Filter: fmt.Sprintf(`metric.type = starts_with("%s")`, expectedTypes[0]),
			},
		)
		var foundTypes []string
		for {
			resp, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			foundTypes = append(foundTypes, resp.Type)
		}
		if diff := cmp.Diff(expectedTypes, foundTypes); diff != "" {
			return fmt.Errorf("expected different types: %v", diff)
		}
		return nil
	}); err != nil {
		t.Fatalf("check if available: %v", err)
	}
}

func TestDeleteMetricDescriptorsDryRun(t *testing.T) {
	gcmSA := gcmServiceAccountOrFail(t)

	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, gcmSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		t.Fatalf("create credentials from JSON: %s", err)
	}

	// Mimic export user-agent.
	ua := "prometheus-engine-script-test"
	client, err := gcm.NewMetricClient(
		ctx,
		option.WithCredentials(creds),
		option.WithUserAgent(ua))
	if err != nil {
		t.Fatalf("create client: %s", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	testID := fmt.Sprintf("%v: %v", t.Name(), ulid.MustNew(ulid.Now(), rand.New(rand.NewSource(time.Now().UnixNano()))).String())

	var expectedTypes = []string{
		"prometheus.googleapis.com/pe_test_script", "prometheus.googleapis.com/pe_test_script2",
	}

	reqName := "projects/" + creds.ProjectID
	resource := &monitoredres_pb.MonitoredResource{
		Type: "prometheus_target",
		Labels: map[string]string{
			"project_id": creds.ProjectID,
			"cluster":    "pe-github-action",
			"location":   "europe-west3-a",
			"job":        "",
			"instance":   "",
			"namespace":  "",
		},
	}
	now := time.Now()
	// Writing test data.
	if err := client.CreateTimeSeries(ctx, &monitoringpb.CreateTimeSeriesRequest{
		Name: reqName,
		TimeSeries: []*monitoringpb.TimeSeries{
			{
				Resource:   resource,
				Metric:     &metricpb.Metric{Type: expectedTypes[0], Labels: map[string]string{"test": testID}},
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,

				Points: []*monitoringpb.Point{{
					Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(now), EndTime: timestamppb.New(now)},
					Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.1}}}},
			},
			{
				Resource:   resource,
				Metric:     &metricpb.Metric{Type: expectedTypes[1], Labels: map[string]string{"test": testID}},
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,

				Points: []*monitoringpb.Point{{
					Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(now), EndTime: timestamppb.New(now)},
					Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.1}}}},
			},
		},
	}); err != nil {
		t.Fatalf("test data write: %v", err)
	}

	assertExpectedTypesExists(t, client, reqName, expectedTypes)

	b := bytes.Buffer{}
	cmd := exec.Command(
		"go", "run",
		"delete_metric_descriptors/delete_metric_descriptors.go",
		"-projects", "projects/"+creds.ProjectID,
		"-metric_type_regex", ".*pe_test_script(|2)",
		"-dry_run",
		"-sa-envvar", "GCM_SECRET",
	)
	cmd.Stderr, cmd.Stdout = &b, &b
	if err := cmd.Run(); err != nil {
		fmt.Println(b.String())
		t.Fatal(err)
	}
	if expPrefix, got := "\nFor project projects/gpe-test-1:\n[prometheus.googleapis.com/pe_test_script prometheus.googleapis.com/pe_test_script2]\nAfter checking", b.String(); !strings.HasPrefix(got, expPrefix) {
		fmt.Println(cmp.Diff(expPrefix, got))
		t.Fatalf("got %v, does not have expected prefix %v", got, expPrefix)
	}
	if expSuffix, got := "descriptors, found 2 to delete across 1 project(s)\n\n-dry_run selected, job done!\n", b.String(); !strings.HasSuffix(got, expSuffix) {
		t.Fatalf("got %v, does not have expected suffix %v", got, expSuffix)
	}

	// Nothing should be deleted.
	assertExpectedTypesExists(t, client, reqName, expectedTypes)

}
