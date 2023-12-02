package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

/*
This script deletes metric descriptors from the given projects (-projects flag),
matching the given metric type (descriptor name) regex expression (-metric_type_regex flag).

Metrics to delete will be first printed and then awaiting interactive confirmation,
before the actual removal. Dry run option also exists.

WARNING: All underlying time series behind each descriptor (potentially years
of data) will be irreversibly removed once confirmed.

Example run:

1. Setup Application Default Credentials (ADC) (https://cloud.google.com/docs/authentication/provide-credentials-adc)
if you haven't yet:
	1a. Make sure the account behind the ADC for chosen projects has Monitoring Editor or Monitoring Admin permissions: https://cloud.google.com/monitoring/access-control#monitoring-perms
  1b. Acquire Application Default Credentials in your environment using gcloud:

gcloud auth application-default login

2. Run Go script (from the same directory as the script):

go run delete_metric_descriptors.go -projects projects/<your-project> -metric_type_regex "<your matching expression>"

See go run delete_metric_descriptors.go -help for all options.
*/

var (
	cloudMonitoringEndpoint = flag.String("address", "monitoring.googleapis.com:443", "address of monitoring API")

	projectNames    = flag.String("projects", "", "required: comma-separated project IDs of the projects on which to execute the requests. Name format is as defined in https://cloud.google.com/monitoring/api/ref_v3/rpc/google.monitoring.v3#listmetricdescriptorsrequesttarget, e.g. projects/test-project,projects/test-project2")
	metricTypeRegex = flag.String("metric_type_regex", "", "required: RE2 regex expression matching metric.type (anchored), so metric descriptor names to delete. Guarded with the interactive 'y' confirmation. See --dry_run to only print those")
	dryRun          = flag.Bool("dry_run", false, "whether to dry run or not")

	serviceAccountEnvVar = flag.String("sa-envvar", "", "optional environment variable containing Google Service Account JSON, without it application-default flow will be used.")
)

func deleteDescriptors(endpoint string, projects []string, re2 *regexp.Regexp, saEnvVar string, dryRun bool) error {
	ctx := context.Background()

	// Recommended way is to use auth from your environment. Use `gcloud auth application-default login` to set it up.
	client, err := monitoring.NewMetricClient(ctx, func() []option.ClientOption {
		// Optional, service account JSON in environment variable.
		if saEnvVar != "" {
			return []option.ClientOption{
				option.WithEndpoint(endpoint),
				option.WithCredentialsJSON([]byte(os.Getenv(saEnvVar))),
			}
		}
		return []option.ClientOption{option.WithEndpoint(endpoint)}
	}()...)
	if err != nil {
		log.Fatalf("failed to build client for %s", endpoint)
	}
	defer client.Close()

	// Find descriptors to delete.
	descsToDelete := map[string][]string{}
	toDelete := 0
	checked := 0
	for _, p := range projects {
		it := client.ListMetricDescriptors(ctx, &monitoringpb.ListMetricDescriptorsRequest{Name: p})
		for {
			resp, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("ListMetricDescriptors iteration: %w", err)
			}
			checked++
			if !re2.MatchString(resp.Type) {
				continue
			}
			descsToDelete[p] = append(descsToDelete[p], resp.Type)
			toDelete++
		}
	}

	// Print and perform interactive safety check.
	{
		for p, descs := range descsToDelete {
			fmt.Println()
			fmt.Printf("For project %v:\n", p)
			fmt.Println(descs)
		}
		fmt.Printf("After checking %v descriptors, found %v to delete across %v project(s)\n", checked, toDelete, len(projects))
		fmt.Println()
	}
	if toDelete == 0 {
		fmt.Println("nothing to do, job done!")
		return nil
	}
	if dryRun {
		fmt.Println("-dry_run selected, job done!")
		return nil
	}
	if !confirmDelete() {
		fmt.Println("Deletion not confirmed, exiting")
		return nil
	}

	// Delete.
	deleted := 0
	for p, descs := range descsToDelete {
		for _, d := range descs {
			if err := client.DeleteMetricDescriptor(ctx,
				&monitoringpb.DeleteMetricDescriptorRequest{
					Name: fmt.Sprintf("%s/metricDescriptors/%s", p, d),
				}); err != nil {
				return fmt.Errorf("DeleteMetricDescriptor delete: %w", err)
			}
			deleted++
			fmt.Printf("%s deleted\n", d)
			if deleted%1000 == 0 {
				fmt.Println("Sleeping 1 second to avoid quota issues...")
				time.Sleep(1 * time.Second)
			}
		}
	}
	fmt.Printf("Deleted %v descriptors, job done!\n", deleted)
	return nil
}

func confirmDelete() bool {
	fmt.Printf("Are you sure you want to delete the above metric descriptors?\n" +
		"WARNING: All underlying time series (potentially years of data) will be irreversibly removed! (y/N): ")
	r, _, err := bufio.NewReader(os.Stdin).ReadRune()
	if err != nil {
		log.Fatalln(err)
	}
	switch unicode.ToLower(r) {
	case 'y':
		return true
	default:
		return false
	}
}

func main() {
	flag.Parse()

	if *projectNames == "" {
		fmt.Println("-projects flag is required")
		flag.Usage()
		os.Exit(1)
	}
	if *metricTypeRegex == "" {
		fmt.Println("-metric_type_regex flag is required")
		flag.Usage()
		os.Exit(1)
	}
	// Anchor it to avoid further surprises.
	reExpr := fmt.Sprintf("^%s$", *metricTypeRegex)
	re, err := regexp.Compile(reExpr)
	if err != nil {
		log.Fatalf("error while compiling RE2 %v expression: %v", *metricTypeRegex, err)
	}
	// Run command.
	if err := deleteDescriptors(
		*cloudMonitoringEndpoint,
		strings.Split(*projectNames, ","),
		re,
		*serviceAccountEnvVar,
		*dryRun,
	); err != nil {
		log.Fatalf("command failed: %v", err)
	}
}
