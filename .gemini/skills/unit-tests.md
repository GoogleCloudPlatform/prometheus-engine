# Skill: Run and Debug Unit Tests

This skill describes how to run, target, and debug unit tests within the `prometheus-engine` repository.

## Purpose
Use this skill when you need to verify changes, reproduce unit test failures, or check benchmark/script test results.

## Execution Options

### Option 1: Run Unit Tests Natively (Recommended for speed)
Runs tests directly on the host machine using the local Go environment.
```bash
NO_DOCKER=1 make test
```
* **Notes**: This is faster because it does not rebuild Docker images, but requires Go and dependent tooling to be installed on the host.

### Option 2: Run Unit Tests Hermetically (Inside Docker)
Builds a hermetic build environment using Docker to run the tests in isolation.
```bash
make test
```
* **Notes**: This guarantees all dependencies are present and avoids environmental discrepancies.

### Option 3: Run Specific Unit Tests
To run specific tests or packages to debug/validate changes quickly:
```bash
go test ./<package_path>/... -run <TestNamePattern>
```
* **Example**:
  ```bash
  go test ./pkg/operator/... -run TestWebhook
  ```

### Option 4: Run GCM Script Tests
Runs script tests targeting Google Cloud Monitoring integration.
```bash
make test-script-gcm
```
* **Prerequisites**: Requires `GCM_SECRET` env variable to be set. If not present, the task will skip execution.

## Verification
- Ensure the test command outputs `ok` and returns exit code `0`.
- If tests fail, analyze the test outputs for failures, panic traces, or logs.
