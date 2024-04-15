# Data Source Syncer

This CLI tool acts as a cron job which remotely syncs data to a given Grafana Prometheus data source. This ensures that the Grafana data source has the following set correctly:

* The Prometheus server URL
* GET HTTP method
* The Prometheus type and version
* Authentication by refreshing a oAuth2 access token periodically

By regularly refreshing the oAuth2 access token, you can configure Grafana to directly query Google Cloud Monitoring (Managed Service for Prometheus).

[Google access tokens have a lifetime of 1 hour.](https://cloud.google.com/docs/authentication/token-types#at-lifetime) This script runs every 30 minutes to ensure you have an uninterrupted connection between Grafana and Google Cloud Monitoring.

For instructions, see the [Google Cloud documentation for configuring Grafana to use Managed Service for Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus/query).
