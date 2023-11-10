For further instructions, see https://cloud.google.com/stackdriver/docs/managed-prometheus/hpa

# custom-metrics-stackdriver-adapter example instructions

The Custom Metrics Stackdriver Adapter supports querying metrics from
Managed Service for Prometheus starting with
[version v0.13.1 of the adapter](https://github.com/GoogleCloudPlatform/k8s-stackdriver/releases/tag/cm-sd-adapter-v0.13.1).

To set up an example HPA configuration using the Custom Metrics Stackdriver
Adapter, do the following:

1. Set up Managed Service for Prometheus in your cluster.
2. Install Custom Metrics Stackdriver Adapter in your cluster.

   ```sh
   kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter/deploy/production/adapter_new_resource_model.yaml
   ```

3. Deploy an example Prometheus metrics exporter and an HPA resource:

   ```sh
   kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd/custom-metrics-prometheus-sd.yaml
   ```

   This command deploys an exporter application that emits the metric `foo` and
   an HPA resource. The HPA scales this application up to 5 replicas to achieve
   the target value for the metric `foo`.

4. Define a PodMonitoring resource by placing the following configuration in a file named `podmonitoring.yaml`.

   ```yaml
   apiVersion: monitoring.googleapis.com/v1
   kind: PodMonitoring
   metadata:
     name: prom-example
   spec:
     selector:
       matchLabels:
         run: custom-metric-prometheus-sd
     endpoints:
     - port: 8080
       interval: 30s
   ```

5. Deploy the new PodMonitoring resource:

   ```sh
   kubectl -n default apply -f podmonitoring.yaml
   ```

   Within a couple of minutes, Managed Service for Prometheus processes the
   metrics scraped from the exporter and stores them in Cloud Monitoring
   using a long-form name. Prometheus metrics are stored with the following conventions:
      - The prefix `prometheus.googleapis.com`.
      - This suffix is usually one of `gauge`, `counter`, `summary`, or `histogram`,
      although untyped metrics might have the `unknown` or `unknown:counter` suffix.
      To verify the suffix, look up the metric in Cloud Monitoring by using
      Metrics Explorer.

6. Update the deployed HPA to query the metric from Cloud Monitoring. The
   metric `foo` is ingested as `prometheus.googleapis.com/foo/gauge`. To make
   the metric queryable by the deployed HorizontalPodAutoscaler resource, you
   use the long-form name in the deployed HPA, but you have to modify it
   by replacing the all forward slashes (`/`)  with the pipe character (`|`):
   `prometheus.googleapis.com|foo|gauge`. For more information, see the
   [Metrics available from Stackdriver section](https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter#metrics-available-from-stackdriver)
   of the Custom Metrics Stackdriver Adapter repository.

   1. Update the deployed HPA by running the following command:

      ```sh
      kubectl edit hpa custom-metric-prometheus-sd
      ```

   2. Change the value of the `pods.metric.name` field from `foo` to
      `prometheus.googleapis.com|foo|gauge`. The `spec` section should look like
      the following:

      ```yaml
      spec:
         maxReplicas: 5
         metrics:
         - pods:
             metric:
               name: prometheus.googleapis.com|foo|gauge
             target:
               averageValue: "20"
               type: AverageValue
           type: Pods
         minReplicas: 1
      ```

   In this example, the HPA configuration looks for the average value of the
   metric `prometheus.googleapis.com/foo/gauge` to be `20`. Because the
   Deployment sets the value of the metric is `40`, the HPA controller increases
   the number of pods up to the value of the `maxReplicas` (`5`) field to try to
   reduce the average value of the metric across all pods to `20`.

   The HPA query is scoped to the namespace and cluster in which the HPA
   resource is installed, so identical metrics in other clusters and
   namespaces don't affect your autoscaling.

7. To watch the workload scale up, run the following command:

   ```sh
   kubectl get hpa custom-metric-prometheus-sd --watch
   ```

   The value of the `REPLICAS` field changes from `1` to `5`.

   <pre>
   NAME                          REFERENCE                                TARGETS        MINPODS   MAXPODS   REPLICAS   AGE
   custom-metric-prometheus-sd   Deployment/custom-metric-prometheus-sd   40/20          1         5         <b>5</b>          *
   </pre>

8. To scale down the deployment, update the target metric value
   to be higher than the exported metric value. In this example, the Deployment
   sets the value of the `prometheus.googleapis.com/foo/gauge` metric to `40`.
   If you set the target value to a number that is higher than `40`, then the
   deployment will scale down.

   For example, use `kubectl edit` to change the value of the `pods.target.averageValue`
   field in the HPA configuration from `20` to `100`.

   ```sh
   kubectl edit hpa custom-metric-prometheus-sd
   ```

   Modify the spec section to match the following:

   ```yaml
   spec:
     maxReplicas: 5
     metrics:
     - pods:
         metric:
           name: prometheus.googleapis.com|foo|gauge
         target:
           averageValue: "100"
           type: AverageValue
     type: Pods
     minReplicas: 1
   ```

9. To watch the workload scale down, run the following command:

   ```sh
   kubectl get hpa custom-metric-prometheus-sd --watch
   ```

   The value of the `REPLICAS` field changes from `5` to `1`. By design, this
   happens more slowly than when scaling the number of pods up:

   <pre>
   NAME                          REFERENCE                                TARGETS        MINPODS   MAXPODS   REPLICAS   AGE
   custom-metric-prometheus-sd   Deployment/custom-metric-prometheus-sd   40/100          1         5         <b>1</b>          *
   </pre>

10. To clean up the deployed example, run the following commands:

   ```sh
   kubectl delete -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter/deploy/production/adapter_new_resource_model.yaml
   kubectl delete -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd/custom-metrics-prometheus-sd.yaml
   kubectl delete podmonitoring/prom-example
   ```

For more information, see the [Prometheus example](https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/8d1799d8dc0069f573515ea6a241a3b6ed6fb3d2/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd) in the Custom Metrics Stackdriver Adapter repository, or see [Scaling an application](https://cloud.google.com/kubernetes-engine/docs/how-to/scaling-apps).

# prometheus-adapter example instructions
1. Set up Managed Service for Prometheus in your cluster.
1. Deploy the [frontend](../frontend.yaml).
1. Install [prometheus-adapter](prometheus-adapter.yaml).
1. Deploy the manifests in this directory, which include:
    * The [example application](example-app.yaml) deployment and service to emit metrics.
    * The [`PodMonitoring`](pod-monitoring.yaml) to scrape the example app.
    * The [`HorizonalPodAutoscaler`](hpa.yaml) to scale workload.
1. In separate terminal sessions:
    * Generate http load against `prometheus-example-app` service:
    ```
    kubectl run -i --tty load-generator --rm --image=busybox:1.28 --restart=Never -- /bin/sh -c "while sleep 0.01; do wget -q -O- http://prometheus-example-app; done"
    ```
    * Watch horizontal pod autoscaler:
    ```
    kubectl get hpa prometheus-example-app --watch
    ```
    * Watch workload scale up:
    ```
    kubectl get po -lapp.kubernetes.io/name=prometheus-example-app --watch
    ```

1. Stop http load generation via Ctrl+C and watch workload scale back down.
