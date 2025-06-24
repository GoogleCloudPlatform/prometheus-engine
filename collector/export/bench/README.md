# Benchmarking

This directory contains a basic benchmarking script. It accepts a Prometheus binary
and spins it up twice, once exporting to a fake GCM endpoint and once just doing
regular Prometheus work.

Usage:

```
PROMETHEUS=path/to/prometheus ./run.sh
```

The Prometheus binary is typically built locally against the github.com/GoogleCloudPlatform/prometheus
repository.

When the binary is built with the Go race detector enabled (`-race` compile flag), the
load broad coverage of potential race conditions, which will be logged.

## Resource usage

The Prometheus server scrapes itself, which allows gauging the resource usage of the Prometheus
binary as well as the overhead relative to a regular Prometheus server.

The example app allows tweaking its flags to expose more or fewer metrics. Lowering the scrape
interval and increasing the metrics generally provides more realistic usage estimates.

To analyze resource usage, go to the UI at [localhost:9090](http://localhost:9090) and experiment
with the common `process_*` and `go_*` metrics which inform about resource usage and runtime
internals.

One can also provide an additional other Prometheus binary for comparison, e.g. to detect
performance regressions or improvements in the export package:

```
PROMETHEUS=path/to/new/prometheus PROMETHEUS_COMPARE=path/to/old/prometheus ./run.sh
```

The matching scrape section in the `prometheus.yml` needs to be uncommented.
