#!/bin/bash

# Create GKE cluster
gcloud beta container --project "lees-gmp" clusters create "cluster-1-a" --zone "us-central1-c" --no-enable-basic-auth --cluster-version "1.28.9-gke.1000000" --release-channel "regular" --machine-type "e2-medium" --image-type "COS_CONTAINERD" --disk-type "pd-balanced" --disk-size "100" --metadata disable-legacy-endpoints=true --scopes "https://www.googleapis.com/auth/devstorage.read_only","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/monitoring","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management.readonly","https://www.googleapis.com/auth/trace.append" --num-nodes "3" --logging=SYSTEM,WORKLOAD --monitoring=SYSTEM --enable-ip-alias --network "projects/lees-gmp/global/networks/lees-network" --subnetwork "projects/lees-gmp/regions/us-central1/subnetworks/lees-network" --no-enable-intra-node-visibility --default-max-pods-per-node "110" --security-posture=standard --workload-vulnerability-scanning=disabled --no-enable-master-authorized-networks --addons HorizontalPodAutoscaling,HttpLoadBalancing,GcePersistentDiskCsiDriver --enable-autoupgrade --enable-autorepair --max-surge-upgrade 1 --max-unavailable-upgrade 0 --binauthz-evaluation-mode=DISABLED --no-enable-managed-prometheus --enable-shielded-nodes --node-locations "us-central1-c"

# Deploy Parca and Prometheus resources
kubectl create namespace parca
kubectl apply -f parca-server.yaml
kubectl apply -f $1
kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/prometheus-engine/main/examples/instrumentation/go-synthetic/go-synthetic.yaml

# Port-forward Parca service (optional)
kubectl -n parca port-forward service/parca 7070 &

sleep 1800

# Cleanup
kill %1  # Terminate the background port-forward process (if used)
kubectl delete cluster "cluster-1-a" --zone "us-central1-c"