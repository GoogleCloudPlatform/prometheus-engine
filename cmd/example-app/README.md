# example-app
`example-app` is a toy application that emits Prometheus metrics and exemplars.

It can be used to demo and debug Prometheus monitoring infrastructure.

## Running on Kubernetes
If running managed-collection on a Kubernetes cluster, the `example-app` can be
deployed and monitored by:
```bash
kubectl apply -f ./example-app.yaml
```