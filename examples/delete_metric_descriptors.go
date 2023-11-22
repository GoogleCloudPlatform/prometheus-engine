package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"regexp"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"

	"google.golang.org/api/option"
)

var (
	cloudMonitoringEndpoint = flag.String("address", "monitoring.googleapis.com:443", "address of monitoring API")
	resourceContainer       = flag.String("resource_container", "", "target resource container, ex. projects/test-project")
	dryRun                  = flag.Bool("dry_run", false, "whether to dry run or not")
)

/*
* To acquire Application Default Credentials, run:

gcloud auth application-default login

* One way to run this file is to initialize a go module.
* For example, move this file into a new directory and run the following:

go mod init example.com/m
go mod tidy
go run delete_metric_descriptors_timestamps.go -resource_container=projects/test-project

*/

func main() {
	flag.Parse()
	ctx := context.Background()

	client, err := monitoring.NewMetricClient(ctx, option.WithEndpoint(*cloudMonitoringEndpoint))

	if err != nil {
		log.Fatalf("failed to build NewMetricClient for %s", *cloudMonitoringEndpoint)
	}

	it := client.ListMetricDescriptors(
		ctx,
		&monitoringpb.ListMetricDescriptorsRequest{
			Name: *resourceContainer,
		})

	var deleted = 0
	var numTotal = 0

	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Failed ListMetricDescriptors request: %v", err)
		}
		var metricType = resp.Type
		match, err := regexp.MatchString("insert_regex_pattern_to_match", metricType)
		if err == nil && match {
			if *dryRun {
				numTotal++
				fmt.Printf("%s matches the provided regular expression\n", metricType)
			} else {
				err := client.DeleteMetricDescriptor(ctx, &monitoringpb.DeleteMetricDescriptorRequest{
					Name: fmt.Sprintf("%s/metricDescriptors/%s", *resourceContainer, metricType),
				})
				if err != nil {
					log.Fatalf("Failed DeleteMetricDescriptors: %v", err)
				}
				numTotal++
				deleted++
				fmt.Printf("%s deleted\n", metricType)
				// Delete metrics in batches of 1000 metrics and sleep inbetween batches to avoid overwhelming
				// configuration servers.
				if deleted == 1000 {
					time.Sleep(5 * time.Minute)
					deleted = 0
				}
			}
		}
	}
	fmt.Printf("%d deleted in total.\n", numTotal)

	if err := client.Close(); err != nil {
		log.Fatalf("Failed to close client: %v", err)
	}
}
