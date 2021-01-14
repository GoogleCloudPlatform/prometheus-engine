# GPE Collector

This repository contains the gpe-operator for managed collection of Prometheus metrics
on GKE as well as the export library to convert and write Prometheus data to 
Google Cloud Monitoring (GCM)

## Operator

To update generated code when changes to Custom Resource Definitions are made run:

```bash
hack/update-codegen.sh
```
