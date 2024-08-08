# prw2gcm: Prometheus Remote Write (PRW) 2.0 to Google Cloud Monitoring (GCM) proxy

`prw2gcm` is a stateful binary capable of receiving [Prometheus Remote Write (PRW) 2.0](https://prometheus.io/docs/specs/remote_write_spec_2_0/), converting it and shipping via gRPC [Google Cloud Monitoring (GCM) API v3](https://cloud.google.com/monitoring/api/ref_v3/rpc), with automatic authentication if possible.

Currently, it's meant as a preview of a experimental logic that might be incorporated
into GCM one day.

## Protocol Support and Labelling

The proxy only works with the new [PRW 2.0](https://prometheus.io/docs/specs/remote_write_spec_2_0/) [`io.prometheus.write.v2.Request`](https://prometheus.io/docs/specs/remote_write_spec_2_0/#io-prometheus-write-v2-request) proto message.

This proxy adds 4 additional constraints on the incoming requests characteristics.

1. All `TimeSeries` MUST specify `Metadata.Type` field. Unspecified type will be rejected with 400 HTTP status code. at the moment.
2. All series with counter-like semantics MUST have valid `TimesSeries.CreatedTimestamp` specified e.g. Counters, Summaries and non-gauge Histograms. Otherwise, those samples will be rejected with 400 HTTP status code.
3. All classic histograms are by default rejected, with 400 HTTP status code, unless the proxy option `--unsafe.allow-classic-histograms` is enabled. The reason is transactionality/atomicity of classic histograms (in other words NOT self-contained histograms). Many systems, including Monarch requires all buckets (and sum and count) to be in the same metric write, for ingestion scalability. [This is hard to achieve for complex metrics that are using multiple disconnected series, for sender scalability reasons](https://prometheus.io/docs/specs/remote_write_spec_2_0/#future-plans:~:text=the%201.0.-,Transactionality,-There%20is%20still). Two solutions:
  * In future Prometheus (and hopefully more senders), all classic histograms will be able to be converted by Prometheus itself, in atomic way (on scrape) into `nhcb`, so [the native histogram with custom bucketing](https://github.com/prometheus/proposals/blob/main/proposals/2024-01-26_classic-histograms-stored-as-native-histograms.md). This is more efficient and transaction 1:1 alternative to classic histograms.
  * You can enable `--unsafe.allow-classic-histograms` if you are certain that, in general, all histogram series (buckets, sum and count) are part of the single request. It's unsafe, because we can't ensure transactionality, in such case, partial histograms might be either rejected or consumed with some buckets potentially missing.
4. Each series MUST contain mandatory labels with the following keys: `location`, `cluster`, `namespace`, `job`, `instance`. `project_id` can be specified, but if it's empty, the project ID the request is associated with (based on HTTP path) is used.

> Historical context on [PRW 1.0](https://prometheus.io/docs/specs/remote_write_spec/) support: Previous protocol (deprecated now) was not (and won't be) supported because Google Monarch, which is a DB behind GCM, required created timestamps for counter-like metrics, metric type and self-contained histograms. 

## Flags

```bash mdox-exec="bash hack/format_help.sh prw2gcm"
````

## Spinup

The proxy authenticates to GMP with credentials (typically a service account). The
receiving remote write endpoint API will be un-authenticated.

A proxy instance serves write traffic for a single Google Cloud Monitoring workspace, identified
by the project.

```bash
PROJECT_ID=$(gcloud config get-value core/project)
# Default gcloud credentials. Substitute for service account key in production.
CREDENTIALS=$HOME/.config/gcloud/application_default_credentials.json
```

For example, from this directory:

```bash
go run main.go \
  -listen-address=:19091 \
  -gcm.credentials-file=$CREDENTIALS \
  -unsafe.allow-classic-histograms
```

The proxy's PRW 2.0 endpoint will be then accessible via http://localhost:19091/v1/projects/<PROJECT_ID>/location/global/prometheus/api/v2/write,
where `PROJECT_ID` is your GCP project ID or number.

## Docker

You can also build a docker image from source using `make prw2gcm`.
