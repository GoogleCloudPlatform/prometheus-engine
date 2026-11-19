# Frontend

> [!NOTE]
> Consider using [datasource-syncer](../datasource-syncer/README.md) for direct Grafana integration with Google Cloud Monitoring without running an in-cluster proxy.

The `frontend` binary is an authenticating reverse proxy and query UI for Google Cloud Managed Service for Prometheus (GMP) that exposes a Prometheus-compatible HTTP API.
It primarily serves as a target URL for a Prometheus data source in Grafana (or other Prometheus API consumers) and hosts a subset of the upstream Prometheus web UI at `/graph`.

Currently, the following read-only Prometheus HTTP API endpoints are supported:

* `/api/v1/query` (`GET`, `POST`)
* `/api/v1/query_range` (`GET`, `POST`)
* `/api/v1/series` (`GET`, `POST`)
* `/api/v1/labels` (`GET`, `POST`)
* `/api/v1/label/{name}/values` (`GET`)
* `/api/v1/metadata` (`GET`)
* `/api/v1/rules` (`GET` — fan-out proxy to `--rules.target-urls`)
* `/api/v1/alerts` (`GET` — fan-out proxy to `--rules.target-urls`)

Administrative and scrape-target endpoints (`/api/v1/targets`, `/api/v1/status/*`, `/api/v1/admin/*`) are not supported.

## Running Locally

The `frontend` binary authenticates outbound requests to GMP using Google Cloud credentials (typically a service account key or Application Default Credentials) and serves the Prometheus HTTP API and UI locally.

Each `frontend` instance queries a single Google Cloud Monitoring scoping project (`--query.project-id`).

```bash
PROJECT_ID=$(gcloud config get-value core/project)
# Default gcloud credentials. Substitute a service account key in production.
CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"
```

From the **repository root**, run `./cmd/frontend` with `-tags builtinassets` so the embedded Prometheus UI assets (`pkg/ui/embed/static`) are compiled into the binary:

```bash
# Optional: rebuild embedded UI assets if you modified third_party/prometheus_ui/override/
./pkg/ui/build.sh

go run -tags builtinassets ./cmd/frontend \
  --web.listen-address=:19090 \
  --query.credentials-file="$CREDENTIALS" \
  --query.project-id="$PROJECT_ID"
```

> [!NOTE]
> Omitting `-tags builtinassets` selects `pkg/ui/assets_local.go`, which expects a `./static/` directory in the current working directory.
> Also note that the binary defaults `--web.listen-address` to `:19090` (`http://localhost:19090/graph`), whereas the sample Kubernetes Deployment in [`examples/frontend.yaml`](../../examples/frontend.yaml) passes `--web.listen-address=:9090`.
> When `--web.external-url` is provided, any port component on that URL is stripped automatically when constructing redirect and asset links.

## Flags

```bash mdox-exec="bash hack/format_help.sh frontend"
Usage of frontend:
  -log.level string
    	The level of logging. Can be one of 'debug', 'info', 'warn', 'error' (default "info")
  -query.credentials-file string
    	Path to a file with the JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.
  -query.project-id string
    	Project ID of the Google Cloud Monitoring workspace project to query.
  -query.target-url string
    	The URL to forward authenticated requests to. (PROJECT_ID is replaced with the --query.project-id flag.) (default "https://monitoring.googleapis.com/v1/projects/PROJECT_ID/location/global/prometheus")
  -rules.target-urls string
    	Comma separated lists of URLs that support HTTP Prometheus Alert and Rules APIs (/api/v1/alerts, /api/v1/rules), e.g. GMP rule-evaluator. NOTE: Results are merged as-is, no sorting and deduplication is done. (default "http://rule-evaluator.gmp-system.svc.cluster.local:19092")
  -web.external-url string
    	The URL under which the frontend is externally reachable (for example, if it is served via a reverse proxy). Used for generating relative and absolute links back to the frontend itself. If the URL has a path portion, it will be used to prefix served HTTP endpoints. If omitted, relevant URL components will be derived automatically.
  -web.listen-address string
    	Address on which to expose metrics and the query UI. (default ":19090")
```

## Docker

You can build the `gmp/frontend` Docker image from source from the repository root using:

```bash
make frontend
```

## Authentication

The `frontend` binary supports incoming HTTP Basic Authentication when both the `AUTH_USERNAME` and `AUTH_PASSWORD` environment variables are set on the `frontend` process or pod.

## UI Development

Refer to [`pkg/ui/README.md`](../../pkg/ui/README.md) for instructions on updating, building, and testing the embedded Prometheus UI.
