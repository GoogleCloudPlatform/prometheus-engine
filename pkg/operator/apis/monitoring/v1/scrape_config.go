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
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
)

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

type ScrapeConfig struct {
	promconfig.ScrapeConfig  `yaml:",inline"`
	TrackTimestampsStaleness bool `yaml:"-"`
	EnableCompression        bool `yaml:"-"`
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c *ScrapeConfig) MarshalYAML() (any, error) {
	return discovery.MarshalYAMLWithInlineConfigs(c)
}
