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

package v1

import (
	"net/url"

	"github.com/alecthomas/units"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/relabel"
)

// This object is forked to enable a different ScrapeConfig below.
type Config struct {
	GlobalConfig      promconfig.GlobalConfig   `yaml:"global"`
	Runtime           promconfig.RuntimeConfig  `yaml:"runtime,omitempty"`
	AlertingConfig    promconfig.AlertingConfig `yaml:"alerting,omitempty"`
	RuleFiles         []string                  `yaml:"rule_files,omitempty"`
	ScrapeConfigFiles []string                  `yaml:"scrape_config_files,omitempty"`
	ScrapeConfigs     []*ScrapeConfig           `yaml:"scrape_configs,omitempty"`
	StorageConfig     promconfig.StorageConfig  `yaml:"storage,omitempty"`
	TracingConfig     promconfig.TracingConfig  `yaml:"tracing,omitempty"`

	RemoteWriteConfigs []*promconfig.RemoteWriteConfig `yaml:"remote_write,omitempty"`
	RemoteReadConfigs  []*promconfig.RemoteReadConfig  `yaml:"remote_read,omitempty"`
}

// This object is temporarily forked to prevent the TrackTimestampsStaleness
// and EnableCompression fields from being output, to allow for backwards
// compatibility with Prometheus 2.45. This should be removed when
// GoogleCloudPlatform/prometheus is upgraded to v2.53+.
type ScrapeConfig struct {
	JobName                        string                      `yaml:"job_name"`
	HonorLabels                    bool                        `yaml:"honor_labels,omitempty"`
	HonorTimestamps                bool                        `yaml:"honor_timestamps"`
	TrackTimestampsStaleness       bool                        `yaml:"-"`
	Params                         url.Values                  `yaml:"params,omitempty"`
	ScrapeInterval                 model.Duration              `yaml:"scrape_interval,omitempty"`
	ScrapeTimeout                  model.Duration              `yaml:"scrape_timeout,omitempty"`
	ScrapeProtocols                []promconfig.ScrapeProtocol `yaml:"scrape_protocols,omitempty"`
	ScrapeClassicHistograms        bool                        `yaml:"scrape_classic_histograms,omitempty"`
	MetricsPath                    string                      `yaml:"metrics_path,omitempty"`
	Scheme                         string                      `yaml:"scheme,omitempty"`
	EnableCompression              bool                        `yaml:"-"`
	BodySizeLimit                  units.Base2Bytes            `yaml:"body_size_limit,omitempty"`
	SampleLimit                    uint                        `yaml:"sample_limit,omitempty"`
	TargetLimit                    uint                        `yaml:"target_limit,omitempty"`
	LabelLimit                     uint                        `yaml:"label_limit,omitempty"`
	LabelNameLengthLimit           uint                        `yaml:"label_name_length_limit,omitempty"`
	LabelValueLengthLimit          uint                        `yaml:"label_value_length_limit,omitempty"`
	NativeHistogramBucketLimit     uint                        `yaml:"native_histogram_bucket_limit,omitempty"`
	NativeHistogramMinBucketFactor float64                     `yaml:"native_histogram_min_bucket_factor,omitempty"`
	KeepDroppedTargets             uint                        `yaml:"keep_dropped_targets,omitempty"`
	ServiceDiscoveryConfigs        discovery.Configs           `yaml:"kubernetes_sd_configs,omitempty"`
	HTTPClientConfig               config.HTTPClientConfig     `yaml:",inline"`
	RelabelConfigs                 []*relabel.Config           `yaml:"relabel_configs,omitempty"`
	MetricRelabelConfigs           []*relabel.Config           `yaml:"metric_relabel_configs,omitempty"`
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c *ScrapeConfig) MarshalYAML() (any, error) {
	return discovery.MarshalYAMLWithInlineConfigs(c)
}
