package v1

import (
	"errors"
	"fmt"

	"github.com/prometheus/common/config"
)

func TLSVersionFromString(s string) (config.TLSVersion, error) {
	if s == "" {
		return 0, nil
	}
	if v, ok := config.TLSVersions[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown TLS version: %s", s)
}

func (c *TLS) ToPrometheusConfig() (*config.TLSConfig, error) {
	var errs []error
	minVersion, err := TLSVersionFromString(c.MinVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS min version: %w", err))
	}
	maxVersion, err := TLSVersionFromString(c.MaxVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS min version: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &config.TLSConfig{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}, nil
}
