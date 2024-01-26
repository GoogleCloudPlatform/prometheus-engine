package promtest

import (
	"testing"
	"time"

	"github.com/efficientgo/e2e"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dto "github.com/prometheus/client_model/go"
)

type noopBackend struct{}

func (n noopBackend) Ref() string {
	return "noop"
}

func (n noopBackend) start(t testing.TB, env e2e.Environment) (api v1.API, extraLset map[string]string) {
	return
}

func (n noopBackend) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration) {
}

// NoopBackend creates noop backend, useful when you want to skip one backend for
// local debugging purpose without changing test significantly.
func NoopBackend() noopBackend { return noopBackend{} }
