package v1

import (
	"github.com/prometheus/common/config"
)

func (c *TLS) ToPrometheusConfig() *config.TLSConfig {
	return &config.TLSConfig{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}
}
