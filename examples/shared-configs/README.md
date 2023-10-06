# Shared Scrape Endpoint Configs

Sometimes you might want to share some scrape configs, for example HTTP-related
configs, across a multiple scrape endpoints within a single or multiple
`PodMonitoring` or `ClusterPodMonitoring` resources.
[Kustomize](https://kustomize.io/) is a great tool to help you do that.

## Prerequisites

To run this example, `kustomize` version `5.x` is required. You can check your
`kustomize` version with:

```bash
kustomize version
```

Alternatively, you can use the built-in support in `kubectl`:

```bash
kubectl version --output=yaml | grep kustomizeVersion
```

## Running

This example directory structure is as follows:

- `podmonitorings.yaml`: Your `PodMonitoring` or `ClusterPodMonitoring` resources.
- `common.yaml`: This example shows you how to setup a common `tls` configuration.
- `kustomization.yaml`: Applies `common.yaml` over `podmonitorings.yaml`.

To view the final YAML, run from this directory:

```bash
kustomize build .
```

Or with `kubectl`:

```bash
kubectl kustomize .
```
