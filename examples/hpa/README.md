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
