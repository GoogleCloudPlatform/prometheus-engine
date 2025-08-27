package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func mustGetGCMServiceAccount() []byte {
	// TODO(bwplotka): Move it to https://cloud.google.com/build CI.
	saJSON := []byte(os.Getenv("GCM_SECRET"))
	if len(saJSON) == 0 {
		panic("gcmServiceAccountOrFail: no GCM_SECRET env var provided, can't run the repro")
	}
	return saJSON
}

func main() {
	gcmSA := mustGetGCMServiceAccount()
	creds, err := google.CredentialsFromJSON(context.Background(), gcmSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		panic(err)
	}
	if err := listMetrics(os.Stderr, creds.ProjectID, creds); err != nil {
		panic(err)
	}
}

// listMetrics lists all the metrics available to be monitored in the API.
// Slightly adapted https://cloud.google.com/monitoring/docs/samples/monitoring-list-descriptors#monitoring_list_descriptors-go
func listMetrics(w io.Writer, projectID string, creds *google.Credentials) error {
	ctx := context.Background()
	c, err := monitoring.NewMetricClient(ctx, option.WithCredentials(creds))
	if err != nil {
		return err
	}
	defer c.Close()

	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name: "projects/" + projectID,
	}
	iter := c.ListMetricDescriptors(ctx, req)

	lastDescriptor := ""
	descriptors := 0
	for {
		resp, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("could not list metrics after %d iterations (last descriptor: %v): %w", descriptors, lastDescriptor, err)
		}
		descriptors++
		lastDescriptor = resp.GetType()
	}
	fmt.Fprintln(w, "Done")
	return nil
}
