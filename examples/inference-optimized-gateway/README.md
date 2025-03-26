# Inference Extension sample manifests

Please refer to the [Google Cloud documentation](https://cloud.google.com/stackdriver/docs/managed-prometheus/exporters/inference-optimized-gateway) for how to use these manifests.

## authorization

The metrics endpoint of the Inference Gateway is protected, you will need to apply the `secret.yaml` to create a secret in your cluster and pass the secret `inference-gateway-sa-metrics-reader-secret` to the ClusterPodMonitoring instance in the example.
