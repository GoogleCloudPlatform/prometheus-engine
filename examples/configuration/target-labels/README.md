# Target labels and metadata using targetLabels

This example demonstrates how to configure target labels and pod metadata labels on scraped metrics using `targetLabels`.

Using `PodMonitoring` (also available for `ClusterPodMonitoring`)
[`spec.targetLabels`](../../../doc/api.md#monitoring.googleapis.com/v1.TargetLabels), you can:
- Transfer Kubernetes pod labels onto Prometheus target labels using `fromPod`.
- Control which pod metadata labels are attached to scraped targets using `metadata`.

## Transferring Pod Labels (`fromPod`)

Kubernetes pod labels frequently contain characters such as `/`, `.`, or `-` (for example, `app.kubernetes.io/name`), which are not valid in Prometheus metric label names (which must match `^[a-zA-Z_][a-zA-Z0-9_]*$`).

The `fromPod` field accepts a list of label mappings:
- **`from`**: The key of the pod label to transfer.
- **`to`**: (Optional) The name of the Prometheus target label. If omitted, defaults to the same name as `from`. Specifying `to` is necessary when `from` contains characters that are not valid in Prometheus label names, or to rename the label to conform to naming conventions and prevent collisions.

```yaml
targetLabels:
  fromPod:
  # Direct transfer when the pod label key is already a valid Prometheus label name:
  - from: app
  # Remapping is necessary when the Kubernetes label key contains characters like '/' or '-':
  - from: app.kubernetes.io/name
    to: app_name
  # Specify which pod metadata labels to include (defaults to [container, pod, top_level_controller_name, top_level_controller_type]):
  metadata:
  - pod
  - container
```

## Example

Apply the `PodMonitoring` resource configured with `targetLabels`:

```bash
kubectl apply -f ./examples/configuration/target-labels/pod-monitoring.yaml
```

The scraped metrics will now include the `app` and `app_name` labels from the Pod's labels, along with `pod` and `container` metadata labels.
