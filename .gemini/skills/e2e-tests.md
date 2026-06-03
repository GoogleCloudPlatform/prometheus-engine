# Skill: Run End-to-End (e2e) Tests

This skill describes how to execute the e2e test suite against a local Kubernetes cluster (Kind).

## Purpose
Use this skill when validating functionality of the operator, config-reloader, and query-frontend in a real, simulated Kubernetes environment.

## Prerequisites
- Docker daemon must be running.
- Port `5001` must be free for the Kind registry container (`kind-registry`).

## Execution Options

### Option 1: Run Full e2e Test Suite (Build and Run)
Builds all dependent images and runs the e2e test suite:
```bash
make e2e
```

### Option 2: Run e2e Tests without Rebuilding Images (Fast Loop)
If you only changed test files under `e2e/` and not operator/component code, you can skip the build phase:
```bash
make e2e-only
```

### Option 3: Run Specific e2e Test Case
To run a specific test, set the `TEST_RUN` environment variable to the exact name or regex matching the test function.
```bash
TEST_RUN=TestOperatorSync make e2e-only
```

### Option 4: Run E2E Test Suite in Interactive Shell
To inspect the Kind cluster directly or run `kubectl` commands against the test cluster, run:
```bash
make e2e-exec
```
This drops you into a bash terminal inside the test container environment.

## Advanced Configurations

- **Configure Parallelism**: Set `KIND_PARALLEL` (default: 5) to control how many tests run concurrently:
  ```bash
  KIND_PARALLEL=3 make e2e
  ```
- **Provide Credentials**: If tests need to validate writing/reading metrics from Google Cloud Monitoring (GCM):
  - Provide a GCP service account JSON key:
    ```bash
    export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
    make e2e
    ```

## Cleanup & Troubleshooting
- **Docker Resource Exhaustion**: If you get container/kind cluster failures, clean up all docker test containers:
  ```bash
  docker stop $(docker ps -a -q) && docker container prune -f
  ```
- **File Descriptor Limit**: If you run into `pod errors due to too many open files`, reduce the `KIND_PARALLEL` factor or increase your system's open file limit (`ulimit -n 65536`).
