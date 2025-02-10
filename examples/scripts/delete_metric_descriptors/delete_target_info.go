// Copyright 2025 Google LLC
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

// This script is meant for customers migrating to using the new OTLP backend. Since the backend
// will export prometheus.googleapis.com/target_info/gauge as a DOUBLE, it will conflict with
// existing metrics that are INT64. This script deletes all such incompatible descriptors within
// the scoping project provided.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/api/metric"

	"google.golang.org/api/option"
)

var (
	cloudMonitoringEndpoint = flag.String("address", "monitoring.googleapis.com:443", "address of the monitoring API")
	resourceContainer       = flag.String("resource_container", "", "scoping project, e.g. project/test-project")
	dryRun                  = flag.Bool("dry_run", true, "whether to dry run or not")
)

func main() {
	flag.Parse()
	if *resourceContainer == "" {
		log.Fatalf("--resource_container flag must be set")
	}

	ctx := context.Background()

	client, err := monitoring.NewMetricClient(ctx, option.WithEndpoint(*cloudMonitoringEndpoint))

	if err != nil {
		log.Fatalf("failed to connect to monitoring API: %v", err)
	}

	if *dryRun {
		fmt.Print("*** DRY RUN ***\n")
	}

	it := client.ListMetricDescriptors(
		ctx,
		&monitoringpb.ListMetricDescriptorsRequest{
			Name:   *resourceContainer,
			Filter: "metric.type=\"prometheus.googleapis.com/target_info/gauge\"",
		})

	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("ListMetricDescriptors failed: %v", err)
		}
		if resp.ValueType == metric.MetricDescriptor_DOUBLE {
			fmt.Printf("%s is already DOUBLE, skipping\n", resp.Name)
			continue
		}
		if *dryRun {
			fmt.Printf("%s is not DOUBLE, would delete\n", resp.Name)
			continue
		}

		err = client.DeleteMetricDescriptor(ctx, &monitoringpb.DeleteMetricDescriptorRequest{Name: resp.Name})
		if err != nil {
			log.Fatalf("DeleteMetricDescriptors failed: %v", err)
		}
		fmt.Printf("%s deleted\n", resp.Name)
	}
}
