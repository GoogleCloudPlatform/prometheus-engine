# Collection Benchmarking

This directory contains utilities to benchmark the GPE collection stack on
GKE clusters.

## Spinup

Make sure that your `gcloud` CLI is [setup properly](https://cloud.google.com/sdk/docs/quickstart).

### Create Cluster

Define the cluster name, location, and scale:

```bash
BASE_DIR=$(git rev-parse --show-toplevel)
PROJECT_ID=$(gcloud config get-value core/project)
ZONE=us-central1-b # recommended for benchmarks
CLUSTER=gpe-"bench-$USER"
NODE_COUNT=5
NODE_TYPE=e2-medium
```

Create the a cluster:

```bash
gcloud container clusters create "$CLUSTER" \
    --zone "$ZONE" --machine-type="$NODE_TYPE" --num-nodes="$NODE_COUNT" &&
gcloud container clusters get-credentials "$CLUSTER" --zone "$ZONE"
```

### Build Container Images

While this is running, we can build container images for the benchmark. You can repeat
the steps in this section to update the benchmark setup on code changes.

#### gpe-collector

Build the container images from the current head of the repository:

```bash
IMAGE_TAG=$(date "+bench_%Y%d%m_%H%M")
RELOADER_IMAGE="gcr.io/$PROJECT_ID/gpe-config-reloader:$IMAGE_TAG"

pushd "$BASE_DIR" &&
gcloud builds submit --config build.yaml --substitutions=TAG_NAME="$IMAGE_TAG" &&
popd
```

#### gpe-prometheus

Make sure that you have the gpe-prometheus repository checked out in the same directory
as this repository.

Then build the container images including any changes to the libraries it uses from gpe-collector:

```bash
PROMETHEUS_IMAGE_TAG=$(date "+bench_%Y%d%m_%H%M")
PROMETHEUS_IMAGE="gcr.io/$PROJECT_ID/prometheus:$PROMETHEUS_IMAGE_TAG"

pushd "$BASE_DIR/../gpe-prometheus" &&
make promu &&
go mod vendor &&
promu crossbuild -p linux/amd64 &&
gcloud builds submit --tag "$PROMETHEUS_IMAGE" &&
popd
```

### Deploy

Deploy the base monitoring stack:

```bash
kubectl apply -f "$BASE_DIR/deploy/operator/" &&
kubectl apply -f "$BASE_DIR/deploy/" --recursive
```

Next, define a size of our example workload and deploy it. You may rerun this step
as needed to change size.

```bash
APP_DEPLOYMENTS=3
APP_REPLICAS=30
APP_CPU_BURN=0 # Burn CPU operations to simulate resource pressure
APP_MEM_BALLAST=0 # Memory usage in megabytes

for i in $(seq 1 $APP_DEPLOYMENTS); do 
  REPLICAS=$APP_REPLICAS IMAGE_TAG=$IMAGE_TAG INDEX=$i CPU_BURN=$APP_CPU_BURN MEM_BALLAST=$APP_MEM_BALLAST PROJECT_ID=$PROJECT_ID \
    envsubst < "$BASE_DIR/bench/deployment.yaml" | kubectl apply -f -;
done
kubectl apply -f "$BASE_DIR/bench/pod_monitoring.yaml" 
```

Lastly, we run the operator locally. Doing that instead of deploying it inside of the cluster
doesn't affect any behavior but makes quick iteration quicker.

```bash
go run $BASE_DIR/cmd/operator/*.go \
  --image-collector="$PROMETHEUS_IMAGE" \
  --image-config-reloader="$RELOADER_IMAGE" \
  --priority-class=gpe-critical \
  --cloud-monitoring-endpoint=staging-monitoring.sandbox.googleapis.com:443
```

You may terminate the operator, rebuild images as needed by following the steps above, and
start it again to deploy the new versions.


## Teardown

To teardown the setup, simply delete the cluster:

```bash
gcloud container clusters delete "$CLUSTER" --zone "$ZONE"
```

## Evaluation

Go to the Cloud Monitoring metric explorer for your project and check whether all targets are
being scraped via the following MQL query (substitute the `$CLUSTER` name manually):

```
fetch prometheus_target
| metric 'external.googleapis.com/gpe/up/gauge'
| filter (resource.cluster == '$CLUSTER')
| group_by [resource.job], [sum(val())]
```

Further interesting cluster-wide queries are:

```
# Number of active streams by job.
fetch prometheus_target
| metric 'external.googleapis.com/gpe/scrape_samples_scraped/gauge'
| filter resource.cluster == '$CLUSTER'
| group_by [resource.job], [sum(val())]

# Total number of scraped Prometheus samples per second.
fetch prometheus_target
| metric 'external.googleapis.com/gpe/prometheus_tsdb_head_samples_appended_total/counter'
| filter resource.cluster == '$CLUSTER'
| align rate(1m)
| every 1m
| group_by [], [sum(val())]
```

If no metrics show up, directly connect to one of the collector pods and inspect the "Targets",
"Configuration, or "Service Discovery" pages in the Prometheus UI for further debugging.

```bash
COLLECTOR_POD=$(kubectl -n gpe-system get pod -l "app.kubernetes.io/name=collector" -o name | head -n 1)
kubectl -n gpe-system port-forward --address 0.0.0.0 $COLLECTOR_POD 9090
```

To inspect resource usage, provides Prometheus node_exporter metrics for node-wide resource consumption
as well as cAdvisor metrics for container-level resource usage. They can either query them through MQL
for an entire cluster, or in the collector's Prometheus UI for an individual node.

Some interesting PromQL queries:

```
# Percentage of total node CPU in use.
1 - avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[2m]))

# CPU usage (fraction of a core) by container.
sum by(container) (rate(container_cpu_usage_seconds_total{container!="", container!="POD"}[2m]))

# Memory usage by container.
sum by(container) (container_memory_usage_bytes{container!="", container!="POD"})

# Number of actively scraped Prometheus time series.
sum by(job) (scrape_samples_scraped)

# Rate at which Prometheus samples are scraped.
rate(prometheus_tsdb_head_samples_appended_total[2m])

# Rate at which GCM samples are exported. This is expected to be lower as histogram series
# map to a single GCM distribution.
rate(gcm_collector_samples_exported_total[2m])

# Rate at which samples are dropped in the collector because they cannot be exported fast enough.
rate(gcm_collector_samples_dropped_total[2m])
```
