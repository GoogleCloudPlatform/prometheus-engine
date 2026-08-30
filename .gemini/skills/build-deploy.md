# Skill: Build Binaries and Docker Images

This skill outlines how to build component binaries, build local Docker images, and submit images for cloud builds.

## Purpose
Use this skill when you need to compile components (such as operator, config-reloader, rule-evaluator) or build container images for deployment.

## Execution Options

### Option 1: Build All Component Binaries Locally
Compiles all Go binaries natively:
```bash
NO_DOCKER=1 make bin
```
* **Compiles components**: `config-reloader`, `operator`, `rule-evaluator`, `go-synthetic`, `frontend`.
* **Output directory**: Binaries are written to `./build/bin/`.

### Option 2: Build a Single Binary Locally
To build a specific component (e.g. `operator`):
```bash
NO_DOCKER=1 make operator
```

### Option 3: Build Docker Image Locally (default)
Builds the docker container image for a component locally using Docker:
```bash
make operator
```
* **Tag format**: Locally tagged as `gmp/<binary_name>` (e.g. `gmp/operator`).

### Option 4: Build and Push Docker Image to Registry
Builds, tags, and pushes the image to a custom container registry:
```bash
DOCKER_PUSH=1 IMAGE_REGISTRY=<your_registry> TAG_NAME=<your_tag> make operator
```
* **Output**: Pushes image to `<IMAGE_REGISTRY>/operator:<TAG_NAME>`.
* **Automatic Manifest Update**: Automatically updates image references in `manifests/` and `examples/`.

### Option 5: Build Multi-Arch Images using Google Cloud Build
Submits a build job to Google Cloud Build:
```bash
CLOUD_BUILD=1 IMAGE_REGISTRY=<your_registry> TAG_NAME=<your_tag> make operator
```
* **Prerequisites**: Requires `gcloud` installed, authenticated, and a GCP project set as active.

## Verification
- For local binaries: check that the file exists under `./build/bin/<name>` and is executable.
- For Docker builds: run `docker images | grep gmp` to verify the image has been built and tagged.
