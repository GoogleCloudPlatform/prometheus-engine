# Data Source Syncer

This CLI tool acts as a cron job which remotely syncs data to a given Grafana Prometheus data source. This ensures that the Grafana data source has the following set correctly:

* The Prometheus server URL
* GET HTTP method
* The Prometheus type and version
* Authentication by refreshing a oAuth2 access token periodically

By regularly refreshing the oAuth2 access token, you can configure Grafana to directly query Google Cloud Monitoring (Managed Service for Prometheus).

[Google access tokens have a lifetime of 1 hour.](https://cloud.google.com/docs/authentication/token-types#at-lifetime) This script runs every 30 minutes to ensure you have an uninterrupted connection between Grafana and Google Cloud Monitoring.

For instructions, see the [Google Cloud documentation for configuring Grafana to use Managed Service for Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus/query).

## Flags

```bash mdox-exec="bash hack/format_help.sh datasource-syncer"
Usage of datasource-syncer:
  -datasource-uids string
    	datasource-uids is a comma separated list of data source UIDs to update.
  -gcm-endpoint-override string
    	gcm-endpoint-override is the URL where queries should be sent to from Grafana. This should be left blank in almost all circumstances.
  -grafana-api-endpoint string
    	grafana-api-endpoint is the endpoint of the Grafana instance that contains the data sources to update.
  -grafana-api-token string
    	grafana-api-token used to access Grafana. Can be created using: https://grafana.com/docs/grafana/latest/administration/service-accounts/#create-a-service-account-in-grafana
  -insecure-skip-verify
    	Skip TLS certificate verification
  -project-id string
    	Project ID of the Google Cloud Monitoring scoping project to query. Queries sent to this project will union results from all projects within the scope.
  -query.credentials-file string
    	JSON-encoded credentials (service account or refresh token). Can be left empty if default credentials have sufficient permission.
  -tls-ca-cert string
    	Path to the server certificate authority
  -tls-cert string
    	Path to the server TLS certificate.
  -tls-key string
    	Path to the server TLS key.
```
