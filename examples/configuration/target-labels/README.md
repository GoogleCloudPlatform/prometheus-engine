# Target labels with fromPod

This example shows how to transfer labels from Kubernetes Pods onto Prometheus
target labels using [`targetLabels.fromPod`](../../doc/api.md#monitoring.googleapis.com/v1.TargetLabels).

`fromPod` mappings are applied in order. Each mapping copies a value from a Pod
label (`from`) onto a Prometheus target label (`to`). When `to` is omitted, the
target label name defaults to the same name as `from`.

This is useful when you want scraped metrics to include application-specific Pod
labels (for example `app.kubernetes.io/version`) without writing relabeling rules.

## Example

The [pod-monitoring-from-pod.yaml](pod-monitoring-from-pod.yaml) example copies the
`app.kubernetes.io/version` Pod label onto a `version` target label for all scraped
metrics.

### Steps

1. Deploy the example application:

    ```
    kubectl apply -f ../../example-app.yaml
    ```

2. Apply the `PodMonitoring` with `fromPod` mappings:

    ```
    kubectl apply -f ./pod-monitoring-from-pod.yaml
    ```

3. Verify that scraped metrics include the `version` label with the value from
   the Pod's `app.kubernetes.io/version` label.

## When to use `from` vs `to`

- Use `from` to select which Pod label to copy.
- Use `to` when the Prometheus target label should have a different name than the
  Pod label. If `to` is omitted, the target label keeps the same name as `from`.

For example, mapping `from: app.kubernetes.io/version` to `to: version` exposes a
shorter label name on scraped metrics.
