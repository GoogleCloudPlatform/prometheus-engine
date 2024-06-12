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
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/lease"
	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/shlex"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// Blank import required to register auth handlers to talk use different auth mechanisms
	// for talking to the Kubernetes API server.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	// Supported HA backend modes.
	HABackendNone       = "none"
	HABackendKubernetes = "kube"
	// User agent environments.
	UAEnvGKE         = "gke"
	UAEnvGCE         = "gce"
	UAEnvUnspecified = "unspecified"

	// User agent modes.
	UAModeGKE         = "gke"
	UAModeKubectl     = "kubectl"
	UAModeUnspecified = "unspecified"
	UAModeAVMW        = "on-prem"
	UAModeABM         = "baremetal"
)

// Environment variable that contains additional command line arguments.
// It can be used to inject additional arguments when the regular ones cannot
// be easily modified.
const ExtraArgsEnvvar = "EXTRA_ARGS"

// Generally, global state is not a good approach and actively discouraged throughout
// the Prometheus code bases. However, this is the most practical way to inject the export
// path into lower layers of Prometheus without touching an excessive amount of functions
// in our fork to propagate it.
var globalExporter *export.Exporter

var ErrLocationGlobal = errors.New("Location must be set to a named Google Cloud " +
	"region and cannot be set to \"global\". Please choose the " +
	"Google Cloud region that is physically nearest to your cluster. " +
	"See https://www.cloudinfrastructuremap.com/")

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
//
// NOTE(bwplotka): This method should only setup flags, no extra logic should be done here
// as we don't have a logger ready, and nothing was logged for the binary yet.
// Potential risky logic can be moved to the returned function we return here.
// See b/344740239 on how hard is to debug regressions here.
func FromFlags(a *kingpin.Application, userAgentProduct string) func(context.Context, log.Logger, prometheus.Registerer) (*export.Exporter, error) {
	var (
		metadataFetchTimeout time.Duration
		opts                 export.ExporterOpts
	)
	opts.UserAgentProduct = userAgentProduct

	a.Flag("export.disable", "Disable exporting to GCM.").
		Default("false").BoolVar(&opts.Disable)

	a.Flag("export.endpoint", "GCM API endpoint to send metric data to.").
		Default("monitoring.googleapis.com:443").StringVar(&opts.Endpoint)

	a.Flag("export.compression", "The compression format to use for gRPC requests ('none' or 'gzip').").
		Default(export.CompressionNone).EnumVar(&opts.Compression, export.CompressionNone, export.CompressionGZIP)

	a.Flag("export.credentials-file", "Credentials file for authentication with the GCM API.").
		Default("").StringVar(&opts.CredentialsFile)

	a.Flag("export.label.project-id", fmt.Sprintf("Default project ID set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyProjectID)).StringVar(&opts.ProjectID)

	a.Flag("export.user-agent-mode", fmt.Sprintf("Mode for user agent used for requests against the GCM API. Valid values are %q, %q, %q, %q or %q.", UAModeGKE, UAModeKubectl, UAModeAVMW, UAModeABM, UAModeUnspecified)).
		Default(UAModeUnspecified).EnumVar(&opts.UserAgentMode, UAModeUnspecified, UAModeGKE, UAModeKubectl, UAModeAVMW, UAModeABM)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("export.label.location", fmt.Sprintf("The default location set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyLocation)).StringVar(&opts.Location)

	a.Flag("export.label.cluster", fmt.Sprintf("The default cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyCluster)).StringVar(&opts.Cluster)

	a.Flag("export.match", `A Prometheus time series matcher. Can be repeated. Every time series must match at least one of the matchers to be exported. This flag can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data. (Example: --export.match='{job="prometheus"}' --export.match='{__name__=~"job:.*"})`).
		Default("").SetValue(&opts.Matchers)

	a.Flag("export.debug.metric-prefix", "Google Cloud Monitoring metric prefix to use.").
		Default(export.MetricTypePrefix).StringVar(&opts.MetricTypePrefix)

	a.Flag("export.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default("false").BoolVar(&opts.DisableAuth)

	a.Flag("export.debug.batch-size", "Maximum number of points to send in one batch to the GCM API.").
		Default(strconv.Itoa(export.BatchSizeMax)).UintVar(&opts.Efficiency.BatchSize)

	a.Flag("export.debug.shard-count", "Number of shards that track series to send.").
		Default(strconv.Itoa(export.DefaultShardCount)).UintVar(&opts.Efficiency.ShardCount)

	a.Flag("export.debug.shard-buffer-size", "The buffer size for each individual shard. Each element in buffer (queue) consists of sample and hash.").
		Default(strconv.Itoa(export.DefaultShardBufferSize)).UintVar(&opts.Efficiency.ShardBufferSize)

	a.Flag("export.debug.fetch-metadata-timeout", "The total timeout for the initial gathering of the best-effort GCP data from the metadata server. This data is used for special labels required by Prometheus metrics (e.g. project id, location, cluster name), as well as information for the user agent. This is done on startup, so make sure this work to be faster than your readiness and liveliness probes.").
		Default("10s").DurationVar(&metadataFetchTimeout)

	a.Flag("export.token-url", "The request URL to generate token that's needed to ingest metrics to the project").
		StringVar(&opts.TokenURL)

	a.Flag("export.token-body", "The request Body to generate token that's needed to ingest metrics to the project.").
		StringVar(&opts.TokenBody)

	a.Flag("export.quota-project", "The projectID of an alternative project for quota attribution.").
		StringVar(&opts.QuotaProject)

	haBackend := a.Flag("export.ha.backend", fmt.Sprintf("Which backend to use to coordinate HA pairs that both send metric data to the GCM API. Valid values are %q or %q", HABackendNone, HABackendKubernetes)).
		Default(HABackendNone).Enum(HABackendNone, HABackendKubernetes)

	kubeConfigPath := a.Flag("export.ha.kube.config", "Path to kube config file.").
		Default("").String()
	kubeNamespace := a.Flag("export.ha.kube.namespace", "Namespace for the HA locking resource. Must be identical across replicas. May be set through the KUBE_NAMESPACE environment variable.").
		Default("").OverrideDefaultFromEnvar("KUBE_NAMESPACE").String()
	kubeName := a.Flag("export.ha.kube.name", "Name for the HA locking resource. Must be identical across replicas. May be set through the KUBE_NAME environment variable.").
		Default("").OverrideDefaultFromEnvar("KUBE_NAME").String()

	// NOTE(bwplotka): This function will be likely performed within "getting ready" period, so before readiness is
	// set to ready. Typical readiness can be as fast as 30s, so make sure this code timeouts faster than that.
	return func(ctx context.Context, logger log.Logger, metrics prometheus.Registerer) (*export.Exporter, error) {
		_ = level.Debug(logger).Log("msg", "started constructing the GCM export logic")

		if metadata.OnGCE() {
			// NOTE: OnGCE does not guarantee we will have all metadata entries or metadata
			// server is accessible.

			_ = level.Debug(logger).Log("msg", "detected we might run on GCE node; attempting metadata server access", "timeout", metadataFetchTimeout.String())
			// When, potentially, on GCE we attempt to populate some, unspecified option entries
			// like project ID, cluster, location, zone and user agent from GCP metadata server.
			//
			// This will be used to get *some* data, if not specified override by flags, to
			// use if labels or external label settings does not have those set. Those will
			// be used as crucial labels for export to work against GCM's Prometheus target.
			//
			// Set a hard time limit due to readiness and liveliness probes during this stage.
			mctx, cancel := context.WithTimeout(context.Background(), metadataFetchTimeout)
			tryPopulateUnspecifiedFromMetadata(mctx, logger, &opts)
			cancel()
			_ = level.Debug(logger).Log("msg", "best-effort on-GCE metadata gathering finished")
		}

		switch *haBackend {
		case HABackendNone:
		case HABackendKubernetes:
			kubecfg, err := loadKubeConfig(*kubeConfigPath)
			if err != nil {
				return nil, fmt.Errorf("loading kube config failed: %w", err)
			}
			opts.Lease, err = lease.NewKubernetes(
				logger,
				metrics,
				kubecfg,
				*kubeNamespace, *kubeName,
				&lease.Options{},
			)
			if err != nil {
				return nil, fmt.Errorf("set up Kubernetes lease: %w", err)
			}
		default:
			return nil, fmt.Errorf("unexpected HA backend %q", *haBackend)
		}
		return export.New(ctx, logger, metrics, opts)
	}
}

func loadKubeConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		cfg, err := rest.InClusterConfig()
		if err == nil {
			return cfg, nil
		}
		// Fallback to default config.
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = kubeconfigPath

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
}

// ExtraArgs returns additional command line arguments extracted from the EXTRA_ARGS.
// environment variable. It is parsed like a shell parses arguments.
// For example: EXTRA_ARGS="--foo=bar -x 123".
// It can be used like `flagset.Parse(append(os.Args[1:], ExtraArgs()...))`.
func ExtraArgs() ([]string, error) {
	return shlex.Split(os.Getenv(ExtraArgsEnvvar))
}

// tryPopulateUnspecifiedFromMetadata assumes we are in GCP, with unknown state
// of the metadata server. This is best-effort, so when the metadata server
// is not accessible on GCP, because it was disabled (404 errors), or not accessible
// (connection refused, slow network, e.g. sandbox + metadata disabled)
// it's a noop. Make sure to pass context with a timeout.
func tryPopulateUnspecifiedFromMetadata(ctx context.Context, logger log.Logger, opts *export.ExporterOpts) {
	const (
		projectIDPath       = "project/project-id"
		clusterNamePath     = "instance/attributes/cluster-name"
		clusterLocationPath = "instance/attributes/cluster-location"
		zonePath            = "instance/zone"
	)

	env := UAEnvGCE // This also means GKE with metadata server disabled.

	c := metadata.NewClient(nil)
	// Mimick metadata.InstanceAttributeValue("cluster-name") but with context.
	gkeClusterName, err := c.GetWithContext(ctx, clusterNamePath)
	if err != nil {
		_ = level.Debug(logger).Log("msg", "fetching entry from GCP metadata server failed; skipping", "key", clusterNamePath, "err", err)
	} else if gkeClusterName != "" {
		env = UAEnvGKE
		if opts.Cluster == "" {
			opts.Cluster = gkeClusterName
		}
	}

	if opts.ProjectID == "" {
		// Mimick metadata.ProjectID() but with context.
		projectID, err := c.GetWithContext(ctx, projectIDPath)
		if err != nil {
			_ = level.Debug(logger).Log("msg", "fetching entry from GCP metadata server failed; skipping", "key", projectIDPath, "err", err)
		} else {
			opts.ProjectID = strings.TrimSpace(projectID)
		}
	}
	if opts.Location == "" {
		// These attributes are set for GKE nodes. For the location, we first check
		// the cluster location, which may be a zone or a region. We must always use that value
		// to avoid collisions with other clusters, as the same cluster name may be reused
		// in different locations.
		// In particular, we cannot set the location to the node's zone for a regional cluster,
		// even though this would provide more accuracy, as there may also be a zonal cluster
		// with the same name.
		//
		// We only fallback to the node zone as the location if no cluster location exists to
		// default for deployments on GCE.

		// Mimick metadata.InstanceAttributeValue("cluster-location") but with context.
		loc, err := c.GetWithContext(ctx, clusterLocationPath)
		if err != nil {
			_ = level.Debug(logger).Log("msg", "fetching entry from GCP metadata server failed; falling back to zone", "key", clusterLocationPath, "err", err)
			zone, err := c.GetWithContext(ctx, zonePath)
			if err != nil {
				_ = level.Debug(logger).Log("msg", "fetching entry from GCP metadata server failed; skipping", "key", zonePath, "err", err)
			} else {
				zone = strings.TrimSpace(zone)
				// zone is of the form "projects/<projNum>/zones/<zoneName>".
				opts.Location = zone[strings.LastIndex(zone, "/")+1:]
			}
		} else {
			opts.Location = loc
		}
	}
	if opts.UserAgentEnv == UAEnvUnspecified {
		// We acknowledge that, if user set unspecified on purpose, this will override.
		opts.UserAgentEnv = env
	}
}
