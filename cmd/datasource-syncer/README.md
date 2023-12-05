# Datasource Syncer

This CLI tool acts as a cron job which remotely syncs data to a given Grafana Prometheus datasource. This ensures that the Grafana datasource has the following set correctly:

* The Prometheus server URL
* GET HTTP method
* The Prometheus type and version
* Authentication by refreshing a oAuth2 access token periodically

By regularly refreshing the oAuth2 access token, you can configure Grafana to directly query Google Cloud Monitoring (Managed Service for Prometheus).

[Google access tokens have a lifetime of 1 hour.](https://cloud.google.com/docs/authentication/token-types#at-lifetime) This script runs every 30 minutes to ensure you have an uninterrupted connection between Grafana and Google Cloud Monitoring.

### Run

1. Figure out the URL of your Grafana instance. e.g. `https://your.grafana.net` for a Grafana Cloud instance or `http://localhost:3000` for a local instance.

2. [Make sure you have configured your Grafana Prometheus data source.](https://cloud.google.com/stackdriver/docs/managed-prometheus/query#grafana-datasource) The data source UID is found in the URL when configuring or exploring a data source. The data source UID is the last part of the URL, when configuring a data source, e.g. `https://your.grafana.net/connections/datasources/edit/<datasource_uid>`.

3. [Set up a Grafana service account](https://grafana.com/docs/grafana/latest/administration/service-accounts/#create-a-service-account-in-grafana) and generate a token.

### Kubernetes CronJob

Set up the following environment variables:

```bash
# These values are required.
DATASOURCE_UIDS=YOUR_DATASOURCE_UIDs
GRAFANA_API_TOKEN=YOUR_GRAFANA_SERVICE_ACCOUNT_TOKEN
GRAFANA_API_ENDPOINT=YOUR_GRAFANA_INSTANCE_URL
PROJECT_ID=PROJECT_ID_TO_QUERY_GCM
 # Optional Credentials file. Can be left empty if default credentials have sufficient permission.
CREDENTIALS=OPTIONAL_GOOGLE_CLOUD_SERVICE_ACCOUNT_WITH_GOOGLE_CLOUD_MONITORING_READ_ACCESS
```

Running the following Cron job will refresh the data source on initialization and every 30 minutes:

```bash
cat datasource-syncer.yaml \
| sed 's|$DATASOURCE_UIDS|'"$DATASOURCE_UIDS"'|; s|$GRAFANA_API_ENDPOINT|'"$GRAFANA_API_ENDPOINT"'|; s|$GRAFANA_API_TOKEN|'"$GRAFANA_API_TOKEN"'|; s|$PROJECT_ID|'"$PROJECT_ID"'|;' \
| kubectl apply -f -
```
### Query Across Multiple Projects

To query across multiple projects, you must [create a metrics scope](https://cloud.google.com/stackdriver/docs/managed-prometheus/query#scoping-intro) and authorize the local project's default compute service account to have monitoring.read access to the scoping project. If your local project is your scoping project, then this permission is granted by default and cross-project querying should work with no further configuration.

### Development 
```bash
go run main.go \
  --credentials-file=$CREDENTIALS \
  --datasource-uids=$DATASOURCE_UIDS \
  --grafana-api-token=$GRAFANA_API_TOKEN \
  --grafana-api-endpoint=$GRAFANA_API_ENDPOINT \
  --project-id=$PROJECT_ID
```
