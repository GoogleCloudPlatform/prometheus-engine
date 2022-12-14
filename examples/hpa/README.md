See https://cloud.google.com/stackdriver/docs/managed-prometheus/hpa for more detailed instructions.

# custom metrics stackdriver adapter example instructions
1. Setup managed-collection on cluster.
1. Install Custom Metrics Stackdriver Adapter in your cluster:

    ```
    kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/deploy/production/adapter_new_resource_model.yaml
    ```
    
1. Deploy an example Prometheus metrics exporter and an HPA resource:

   ```
   kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd/custom-metrics-prometheus-sd.yaml
   ```

1. Define a PodMonitoring resource by placing the following configuration in a file named podmonitoring.yaml:

   ```
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
1. Deploy it:

   ```
   kubectl -n default apply -f podmonitoring.yaml
   ```
   
1. Update the deployed HPA to query the metric from Cloud Monitoring. The metric `foo` is ingested as `prometheus.googleapis.com/foo/gauge`. To make the metric queryable by the deployed HorizontalPodAutoscaler resource, you use the long-form name in the deployed HPA, but you have to modify it by replacing the all forward slashes (`/`) with the pipe character (`|`): `prometheus.googleapis.com|foo|gauge`.

      1. Update the deployed HPA by running the following command:

      ```
      kubectl edit hpa custom-metric-prometheus-sd
      ```
      
      2. Change the value of the `pods.metric.name` field from `foo` to `prometheus.googleapis.com|foo|gauge`. The spec section should look like the following:
      ```
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

1. To watch the workload scale up, run the following command:

```
kubectl get hpa custom-metric-prometheus-sd --watch
```






# prometheus-adapter example instructions
1. Setup managed-collection on cluster.
1. Deploy the [frontend](../frontend.yaml).
1. Install prometheus-adapter on cluster using instructions from
   the deployment [README](https://github.com/kubernetes-sigs/prometheus-adapter/blob/9008b12a0173e2604e794c1614081b63c17e0340/deploy/README.md).
1. Deploy the manifests in this directory, which include:
    * `prometheus-example-app` deployment and service to emit metrics.
    * PodMonitoring to scrape `prometheus-example-app`.
    * HorizonalPodAutoscaler to scale workload.
    * Overwrite for `adapter-config` ConfigMap to expose `http_requests_per_second` to the custom metrics API.
1. Edit the `custom-metrics-apiserver` Deployment to change the Prometheus URL arg to:
    ```
    - --prometheus-url=http://frontend.default.svc:9090/
    ```
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
