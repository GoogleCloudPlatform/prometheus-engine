# Frontend

The frontend binary is a thin query frontend for Google Cloud Prometheus Engine (GPE) that looks
and feels like a regular Prometheus server. It primarily serves as a target URL for a Prometheus
datasource in Grafana and to access GPE via the well-known Prometheus UI.

Currently, the following API endpoints are supported:

* `api/v1/query`
* `api/v1/query_range`
* `api/v1/label/__name__/values`

## Spinup

The frontend authenticates to GPE with credentials (typically a service account) and re-exposes
the JSON/HTTP API unauthenticated (or for use with a custom authentication mechanism).

A frontend instance serves read traffic for a single Google Cloud Monitoring workspace, identified
by the project.

```bash
PROJECT_ID=$(gcloud config get-value core/project)
# Default gcloud credentials. Substitute for service account key in production.
CREDENTIALS=$HOME/.config/gcloud/application_default_credentials.json
```

```bash
go run main.go \
  --web.listen-address=:9090 \
  --query.credentials-file=$CREDENTIALS \
  --query.project-id=$PROJECT_ID
```

Access the frontend UI in your browser at http://localhost:9090.