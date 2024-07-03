# Frontend

> NOTE: Consider using [datasource-syncer](../datasource-syncer) for Grafana integration.

The frontend binary is a thin query frontend for Google Cloud Managed Service
for Prometheus (GMP) that looks and feels like a regular Prometheus server.
It primarily serves as a target URL for a Prometheus datasource in Grafana
and to access GMP via the well-known Prometheus UI.

Currently, the following API endpoints are supported:

* `api/v1/query`
* `api/v1/query_range`
* `api/v1/label/__name__/values`

## Spinup

The frontend authenticates to GMP with credentials (typically a service account) and re-exposes
the JSON/HTTP API unauthenticated (or for use with a custom authentication mechanism).

A frontend instance serves read traffic for a single Google Cloud Monitoring workspace, identified
by the project.

```bash
PROJECT_ID=$(gcloud config get-value core/project)
# Default gcloud credentials. Substitute for service account key in production.
CREDENTIALS=$HOME/.config/gcloud/application_default_credentials.json
```

For example, from this directory:

```bash
bash ../../pkg/ui/build.sh # If you want to use UI.
go run main.go \
  --web.listen-address=:19090 \
  --query.credentials-file=$CREDENTIALS \
  --query.project-id=$PROJECT_ID
```

Access the frontend UI in your browser at http://localhost:19090.

## Flags

```bash mdox-exec="bash hack/format_help.sh frontend"
Usage of frontend:
  -query.credentials-file string
    	JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.
  -query.project-id string
    	Project ID of the Google Cloud Monitoring workspace project to query.
  -query.target-url string
    	The URL to forward authenticated requests to. (PROJECT_ID is replaced with the --query.project-id flag.) (default "https://monitoring.googleapis.com/v1/projects/PROJECT_ID/location/global/prometheus")
  -web.external-url string
    	The URL under which the frontend is externally reachable (for example, if it is served via a reverse proxy). Used for generating relative and absolute links back to the frontend itself. If the URL has a path portion, it will be used to prefix served HTTP endpoints. If omitted, relevant URL components will be derived automatically.
  -web.listen-address string
    	Address on which to expose metrics and the query UI. (default ":19090")
```

## Docker

You can also build a docker image from source using `make frontend`.

## Authentication

The frontend supports incoming authentication using basic auth by providing a
username and password on the incoming request. These are validated against the
`AUTH_USERNAME` and `AUTH_PASSWORD` environment variables, which must be set
on the frontend pod.

## UI Development

Refer to [pkg/ui](/pkg/ui/README.md) for more information on how to develop or
update UI.
