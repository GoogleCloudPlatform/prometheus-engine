// Copyright 2020 Google LLC
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

// Package setup contains common logic for setting up the export package across binaries.
package setup

import (
	"fmt"
	"os"
	"strconv"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/go-kit/kit/log"
	"cloud.google.com/go/compute/metadata"
	"github.com/prometheus/client_golang/prometheus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// Generally, global state is not a good approach and actively discouraged throughout
// the Prometheus code bases. However, this is the most practical way to inject the export
// path into lower layers of Prometheus without touching an excessive amount of functions
// in our fork to propagate it.
var globalExporter *export.Exporter

// SetGlobal sets the global instance of the GCM exporter.
func SetGlobal(exporter *export.Exporter) (err error) {
	globalExporter = exporter
	return err
}

// Global returns the global instance of the GCM exporter.
func Global() *export.Exporter {
	if globalExporter == nil {
		// This should usually be a panic but we set an inactive default exporter in this case
		// to not break existing tests in Prometheus.
		fmt.Fprintln(os.Stderr, "No global GCM exporter was set, setting default inactive exporter.")
		return export.NopExporter()
	}
	return globalExporter
}

// FromFlags returns a constructor for a new exporter that is configured through flags that are
// registered with the given application. The constructor must be called after the flags
// have been parsed.
func FromFlags(a *kingpin.Application, userAgent string) func(log.Logger, prometheus.Registerer) (*export.Exporter, error) {
	var opts export.ExporterOpts

	// Default target fields if we can detect them in GCP.
	if metadata.OnGCE() {
		opts.ProjectID, _ = metadata.ProjectID()
		// These attributes are set for GKE nodes.
		opts.Location, _ = metadata.InstanceAttributeValue("cluster-location")
		opts.Cluster, _ = metadata.InstanceAttributeValue("cluster-name")
	}

	a.Flag("export.disable", "Disable exporting to GCM.").
		Default("false").BoolVar(&opts.Disable)

	a.Flag("export.endpoint", "GCM API endpoint to send metric data to.").
		Default("monitoring.googleapis.com:443").StringVar(&opts.Endpoint)

	a.Flag("export.credentials-file", "Credentials file for authentication with the GCM API.").
		Default("").StringVar(&opts.CredentialsFile)

	a.Flag("export.label.project-id", fmt.Sprintf("Default project ID set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyProjectID)).
		Default(opts.ProjectID).StringVar(&opts.ProjectID)

	a.Flag("export.user-agent", "Override for the user agent used for requests against the GCM API.").
		Default(userAgent).StringVar(&opts.UserAgent)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("export.label.location", fmt.Sprintf("The default location set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyLocation)).
		Default(opts.Location).StringVar(&opts.Location)

	a.Flag("export.label.cluster", fmt.Sprintf("The default cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyCluster)).
		Default(opts.Cluster).StringVar(&opts.Cluster)

	a.Flag("export.match", `A Prometheus time series matcher. Can be repeated. Every time series must match at least one of the matchers to be exported. This flag can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data. (Example: --export.match='{job="prometheus"}' --export.match='{__name__=~"job:.*"})`).
		Default("").SetValue(&opts.Matchers)

	a.Flag("export.debug.metric-prefix", "Google Cloud Monitoring metric prefix to use.").
		Default(export.MetricTypePrefix).StringVar(&opts.MetricTypePrefix)

	a.Flag("export.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default("false").BoolVar(&opts.DisableAuth)

	a.Flag("export.debug.batch-size", "Maximum number of points to send in one batch to the GCM API.").
		Default(strconv.Itoa(export.BatchSizeMax)).UintVar(&opts.BatchSize)

	return func(logger log.Logger, metrics prometheus.Registerer) (*export.Exporter, error) {
		return export.New(logger, metrics, opts)
	}
}
