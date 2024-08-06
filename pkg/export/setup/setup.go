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
	"testing"
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
		if !testing.Testing() {
			panic("must set a global exporter")
		}

		fmt.Fprintln(os.Stderr, "No global GCM exporter was set, setting default inactive exporter.")

		// We don't want to change all upstream Prometheus unit tests, so let's just create
		// a disabled exporter. These are created on-demand to prevent race conditions
		// between tests.
		return export.NopExporter()
	}
	return globalExporter
}

// ExporterOptsFlags adds flags to the application, defaulting the options.
func ExporterOptsFlags(a *kingpin.Application, opts *export.ExporterOpts) {
	opts.DefaultUnsetFields()

	a.Flag("export.disable", "Disable exporting to GCM.").
		Default(strconv.FormatBool(opts.Disable)).
		BoolVar(&opts.Disable)

	a.Flag("export.endpoint", "GCM API endpoint to send metric data to.").
		Default(opts.Endpoint).
		StringVar(&opts.Endpoint)

	a.Flag("export.compression", "The compression format to use for gRPC requests ('none' or 'gzip').").
		Default(opts.Compression).
		EnumVar(&opts.Compression, export.CompressionNone, export.CompressionGZIP)

	a.Flag("export.credentials-file", "Credentials file for authentication with the GCM API.").
		Default(opts.CredentialsFile).
		StringVar(&opts.CredentialsFile)

	a.Flag("export.label.project-id", fmt.Sprintf("Default project ID set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyProjectID)).
		Default(opts.ProjectID).
		StringVar(&opts.ProjectID)

	a.Flag("export.user-agent-mode", fmt.Sprintf("Mode for user agent used for requests against the GCM API. Valid values are %q, %q, %q, %q or %q.", UAModeGKE, UAModeKubectl, UAModeAVMW, UAModeABM, UAModeUnspecified)).
		Default(opts.UserAgentMode).
		EnumVar(&opts.UserAgentMode, UAModeUnspecified, UAModeGKE, UAModeKubectl, UAModeAVMW, UAModeABM)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("export.label.location", fmt.Sprintf("The default location set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyLocation)).
		Default(opts.Location).
		StringVar(&opts.Location)

	a.Flag("export.label.cluster", fmt.Sprintf("The default cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", export.KeyCluster)).
		Default(opts.Cluster).
		StringVar(&opts.Cluster)

	a.Flag("export.match", `A Prometheus time series matcher. Can be repeated. Every time series must match at least one of the matchers to be exported. This flag can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data. (Example: --export.match='{job="prometheus"}' --export.match='{__name__=~"job:.*"})`).
		Default("").
		SetValue(&opts.Matchers)

	a.Flag("export.debug.metric-prefix", "Google Cloud Monitoring metric prefix to use.").
		Default(opts.MetricTypePrefix).
		StringVar(&opts.MetricTypePrefix)

	a.Flag("export.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default(strconv.FormatBool(opts.DisableAuth)).
		BoolVar(&opts.DisableAuth)

	a.Flag("export.debug.batch-size", "Maximum number of points to send in one batch to the GCM API.").
		Default(strconv.FormatUint(uint64(opts.Efficiency.BatchSize), 10)).
		UintVar(&opts.Efficiency.BatchSize)

	a.Flag("export.debug.shard-count", "Number of shards that track series to send.").
		Default(strconv.FormatUint(uint64(opts.Efficiency.ShardCount), 10)).
		UintVar(&opts.Efficiency.ShardCount)

	a.Flag("export.debug.shard-buffer-size", "The buffer size for each individual shard. Each element in buffer (queue) consists of sample and hash.").
		Default(strconv.FormatUint(uint64(opts.Efficiency.ShardBufferSize), 10)).
		UintVar(&opts.Efficiency.ShardBufferSize)

	a.Flag("export.token-url", "The request URL to generate token that's needed to ingest metrics to the project").
		Default(opts.TokenURL).
		StringVar(&opts.TokenURL)

	a.Flag("export.token-body", "The request Body to generate token that's needed to ingest metrics to the project.").
		Default(opts.TokenBody).
		StringVar(&opts.TokenBody)

	a.Flag("export.quota-project", "The projectID of an alternative project for quota attribution.").
		Default(opts.QuotaProject).
		StringVar(&opts.QuotaProject)
}

type Opts struct {
	ExporterOpts export.ExporterOpts
	MetadataOpts MetadataOpts
	HAOptions    HAOptions
}

// SetupFlags adds flags to the application, defaulting the options.
func (opts *Opts) SetupFlags(a *kingpin.Application) {
	ExporterOptsFlags(a, &opts.ExporterOpts)
	opts.MetadataOpts.SetupFlags(a)
	opts.HAOptions.SetupFlags(a)
}

func (opts *Opts) NewExporter(ctx context.Context, logger log.Logger, reg prometheus.Registerer) (*export.Exporter, error) {
	// In case the user reset the fields, default them.
	opts.ExporterOpts.DefaultUnsetFields()
	opts.MetadataOpts.DefaultUnsetFields()
	opts.HAOptions.DefaultUnsetFields()

	opts.MetadataOpts.ExtractMetadata(logger, &opts.ExporterOpts)
	lease, err := opts.HAOptions.NewLease(logger, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, fmt.Errorf("create lease: %w", err)
	}
	return export.New(ctx, logger, reg, opts.ExporterOpts, lease)
}

type HAOptions struct {
	Backend        string
	KubeConfigFile string
	KubeNamespace  string
	KubeName       string
}

// DefaultUnsetFields defaults any zero-valued fields.
func (opts *HAOptions) DefaultUnsetFields() {
	if opts.Backend == "" {
		opts.Backend = HABackendNone
	}
}

// SetupFlags adds flags to the application, defaulting the options.
func (opts *HAOptions) SetupFlags(a *kingpin.Application) {
	opts.DefaultUnsetFields()

	a.Flag("export.ha.backend", fmt.Sprintf("Which backend to use to coordinate HA pairs that both send metric data to the GCM API. Valid values are %q or %q", HABackendNone, HABackendKubernetes)).
		Default(opts.Backend).
		EnumVar(&opts.Backend, HABackendNone, HABackendKubernetes)
	a.Flag("export.ha.kube.config", "Path to kube config file.").
		Default(opts.KubeConfigFile).
		StringVar(&opts.KubeConfigFile)
	a.Flag("export.ha.kube.namespace", "Namespace for the HA locking resource. Must be identical across replicas. May be set through the KUBE_NAMESPACE environment variable.").
		Default(opts.KubeNamespace).
		OverrideDefaultFromEnvar("KUBE_NAMESPACE").
		StringVar(&opts.KubeNamespace)
	a.Flag("export.ha.kube.name", "Name for the HA locking resource. Must be identical across replicas. May be set through the KUBE_NAME environment variable.").
		Default(opts.KubeName).
		OverrideDefaultFromEnvar("KUBE_NAME").
		StringVar(&opts.KubeName)
}

func (opts *HAOptions) NewLease(logger log.Logger, reg prometheus.Registerer) (export.Lease, error) {
	_ = level.Debug(logger).Log("msg", "started constructing the GCM export logic")

	switch opts.Backend {
	case HABackendNone:
		return export.NopLease(), nil
	case HABackendKubernetes:
		kubecfg, err := loadKubeConfig(opts.KubeConfigFile)
		if err != nil {
			return nil, fmt.Errorf("loading kube config failed: %w", err)
		}
		lease, err := lease.NewKubernetes(
			logger,
			reg,
			kubecfg,
			opts.KubeNamespace,
			opts.KubeName,
			&lease.Options{},
		)
		if err != nil {
			return nil, fmt.Errorf("set up Kubernetes lease: %w", err)
		}
		return lease, nil
	default:
		return nil, fmt.Errorf("unexpected HA backend %q", opts.Backend)
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

type MetadataOpts struct {
	FetchTimeout time.Duration
}

// DefaultUnsetFields defaults any zero-valued fields.
func (o *MetadataOpts) DefaultUnsetFields() {
	if o.FetchTimeout == 0 {
		o.FetchTimeout = time.Second * 10
	}
}

// SetupFlags adds flags to the application, defaulting the options.
func (o *MetadataOpts) SetupFlags(a *kingpin.Application) {
	o.DefaultUnsetFields()

	a.Flag("export.debug.fetch-metadata-timeout", "The total timeout for the initial gathering of the best-effort GCP data from the metadata server. This data is used for special labels required by Prometheus metrics (e.g. project id, location, cluster name), as well as information for the user agent. This is done on startup, so make sure this work to be faster than your readiness and liveliness probes.").
		Default(o.FetchTimeout.String()).
		DurationVar(&o.FetchTimeout)
}

func (o *MetadataOpts) ExtractMetadata(logger log.Logger, exporterOpts *export.ExporterOpts) {
	if metadata.OnGCE() {
		// NOTE: OnGCE does not guarantee we will have all metadata entries or metadata
		// server is accessible.

		_ = level.Debug(logger).Log("msg", "detected we might run on GCE node; attempting metadata server access", "timeout", o.FetchTimeout.String())
		// When, potentially, on GCE we attempt to populate some, unspecified option entries
		// like project ID, cluster, location, zone and user agent from GCP metadata server.
		//
		// This will be used to get *some* data, if not specified override by flags, to
		// use if labels or external label settings does not have those set. Those will
		// be used as crucial labels for export to work against GCM's Prometheus target.
		//
		// Set a hard time limit due to readiness and liveliness probes during this stage.
		mctx, cancel := context.WithTimeout(context.Background(), o.FetchTimeout)
		tryPopulateUnspecifiedFromMetadata(mctx, logger, exporterOpts)
		cancel()
		_ = level.Debug(logger).Log("msg", "best-effort on-GCE metadata gathering finished")
	}
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
