# Export write

In this guide, GMP collectors are configured to send metrics to a central Prometheus receiver.
This is an example of how to use GMP collectors to export to a remote endpoint.

## Setup Prometheus Receiver

<b>Launch the Receiver:</b> Run the following command to start a Prometheus receiver that accepts write requests:
```bash
kubectl apply -f prometheus-receiver.yaml -n prometheus
```

## Configure GMP Collectors for Export
1. <b>Ensure Operator is using the latest code:</b>
    ```bash
    DOCKER_PUSH=1 make operator
    kubectl apply -f manifests/setup.yaml
    kubectl apply -f manifests/operator.yaml
    ```
2. <b>Edit the Operator Configuration:</b>
    ```bash
    kubectl -n gmp-public edit operatorconfig config
    ```
3. <b>Add Export Configuration:</b> Insert the following code block into your configuration file to specify the target destination for GMP collector write requests:
    ```bash
    exports:
      - url: http://prometheus-receiver.prometheus.svc:9090/api/v1/write
    ```