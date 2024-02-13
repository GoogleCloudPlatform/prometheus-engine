include .bingo/Variables.mk

GOCMDS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))
GOINSTR += $(notdir $(patsubst %/,%,$(dir $(shell find examples/instrumentation -name 'main.go'))))

CLOUDSDK_CONFIG?=${HOME}/.config/gcloud
PROJECT_ID?=$(shell gcloud config get-value core/project)
PROJECT_ID:=$(if $(PROJECT_ID),$(PROJECT_ID),gmp-project)
GMP_LOCATION?=us-central1-c
GMP_CLUSTER?=gmp-test-cluster
TEST_ARGS=-project-id=$(PROJECT_ID) -location=$(GMP_LOCATION) -cluster=$(GMP_CLUSTER)

API_DIR=pkg/operator/apis
LOCAL_CREDENTIALS=/tmp/gcm-editor.json
# If credentials are provided, ensure we mount them during e2e test.
ifneq ($(GOOGLE_APPLICATION_CREDENTIALS),)
E2E_DOCKER_ARGS := -v $(GOOGLE_APPLICATION_CREDENTIALS):$(LOCAL_CREDENTIALS)
endif

ifeq ($(KIND_PERSIST), 1)
E2E_DOCKER_ARGS += --env KIND_PERSIST=1
endif
REGISTRY_NAME=kind-registry
REGISTRY_PORT=5001
KIND_PARALLEL?=5

# For now assume the docker daemon is mounted through a unix socket.
# TODO(pintohutch): will this work if using a remote docker over tcp?
DOCKER_HOST?=unix:///var/run/docker.sock
DOCKER_VOLUME:=$(DOCKER_HOST:unix://%=%)

IMAGE_REGISTRY?=gcr.io/$(PROJECT_ID)/prometheus-engine
TAG_NAME?=$(shell date "+gmp-%Y%d%m_%H%M")

# If an individual test is not specified, run them all.
TEST_RUN?=$(shell go test ./e2e/... -list=. | grep -E 'Test*')

# TODO(TheSpiritXIII): Temporary env variables part of `export.go` unit tests.
export TEST_TAG=true

# Support gsed on OSX (installed via brew), falling back to sed. On Linux
# systems gsed won't be installed, so will use sed as expected.
SED?=$(shell which gsed 2>/dev/null || which sed)

# TODO(pintohutch): this is a bit hacky, but can be useful when testing.
# Ultimately this should be replaced with go templating.
define update_manifests
	find manifests examples -type f -name "*.yaml" -exec sed -i "s#image: .*/$(1):.*#image: ${IMAGE_REGISTRY}/$(1):${TAG_NAME}#g" {} \;
endef

define docker_build
	DOCKER_BUILDKIT=1 docker build --label "part-of=gmp" $(1)
endef

define docker_tag_push
	docker tag $(1) $(2)
	docker push $(2)
endef

define ensure_registry
	@echo ">> ensuring docker registry"
	if [ "$(shell docker inspect -f '{{.State.Running}}' "$(REGISTRY_NAME)" 2>/dev/null || true)" != 'true' ]; then \
		docker run \
		-d --restart=always -p "127.0.0.1:$(REGISTRY_PORT):5000" --network bridge --name "$(REGISTRY_NAME)" \
		registry:2; \
	fi
endef

.PHONY: help
help:        ## Show this help.
             ##
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

.PHONY: clean
clean:       ## Clean build time resources, primarily, unused docker images.
             ##
	docker rmi -f $(shell docker images -f "label=part-of=gmp" -q)

.PHONY: conform
conform:
	docker run --rm -v ${PWD}:/src -w /src ghcr.io/siderolabs/conform:v0.1.0-alpha.27 enforce

.PHONY: lint
lint: $(GOLANGCI_LINT)
	@echo ">> linting code"
	$(GOLANGCI_LINT) run --verbose --timeout 5m

$(GOCMDS):   ## Build go binary from cmd/ (e.g. 'operator').
             ## The following env variables configure the build, and are mutually exclusive:
             ## Set NO_DOCKER=1 to build natively without Docker.
             ## Set DOCKER_PUSH=1 to tag image with TAG_NAME and push to IMAGE_REGISTRY.
             ## Set CLOUD_BUILD=1 to build the image on Cloud Build, with multi-arch support.
             ## By default, IMAGE_REGISTRY=gcr.io/PROJECT_ID/prometheus-engine.
             ##
	$(MAKE) bin-go BIN_GO_NAME="$@" BIN_GO_DIR="cmd"

$(GOINSTR):  ## Build go binary from examples/instrumentation/ (e.g. 'go-synthetic').
             ## The following env variables configure the build, and are mutually exclusive:
             ## Set NO_DOCKER=1 to build natively without Docker.
             ## Set DOCKER_PUSH=1 to tag image with TAG_NAME and push to IMAGE_REGISTRY.
             ## Set CLOUD_BUILD=1 to build the image on Cloud Build, with multi-arch support.
             ## By default, IMAGE_REGISTRY=gcr.io/PROJECT_ID/prometheus-engine.
             ##
	$(MAKE) bin-go BIN_GO_NAME="$@" BIN_GO_DIR="examples/instrumentation"

BIN_GO_NAME =
BIN_GO_DIR =
bin-go:
	@echo ">> building binaries"
ifeq ($(NO_DOCKER), 1)
	if [ "$(BIN_GO_NAME)" = "frontend" ]; then pkg/ui/build.sh; fi
	CGO_ENABLED=0 go build -tags builtinassets -mod=vendor -o ./build/bin/$(BIN_GO_NAME) ./$(BIN_GO_DIR)/$(BIN_GO_NAME)/*.go
# If pushing, build and tag native arch image to GCR.
else ifeq ($(DOCKER_PUSH), 1)
	$(call docker_build, --tag gmp/$(BIN_GO_NAME) -f ./$(BIN_GO_DIR)/$(BIN_GO_NAME)/Dockerfile .)
	@echo ">> tagging and pushing images"
	$(call docker_tag_push,gmp/$(BIN_GO_NAME),${IMAGE_REGISTRY}/$(BIN_GO_NAME):${TAG_NAME})
	@echo ">> updating manifests with pushed images"
	$(call update_manifests,$(BIN_GO_NAME))
# Run on cloudbuild and tag multi-arch image to GCR.
# TODO(pintohutch): cache source tarball between binary builds?
else ifeq ($(CLOUD_BUILD), 1)
	@echo ">> building GMP images on Cloud Build with tag: $(TAG_NAME)"
	gcloud builds submit --config build.yaml --timeout=30m --substitutions=_IMAGE_REGISTRY=$(IMAGE_REGISTRY),_IMAGE=$(BIN_GO_NAME),TAG_NAME=$(TAG_NAME) --async
	$(call update_manifests,$(BIN_GO_NAME))
# Just build it locally.
else
	$(call docker_build, --tag gmp/$(BIN_GO_NAME) -f ./$(BIN_GO_DIR)/$(BIN_GO_NAME)/Dockerfile .)
endif

bin:         ## Build all go binaries from cmd/ and examples/instrumentation/.
             ## All env vars from $(GOCMDS) work here as well.
             ##
bin: $(GOCMDS) $(GOINSTR)

.PHONY: regen
regen:       ## Refresh autogenerated files and reformat code.
             ## Use CHECK=1 to only validate clean repo after run.
             ##
regen: $(ADDLICENSE)
ifeq ($(CHECK), 1)
	$(call docker_build, -f ./hack/Dockerfile --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/presubmit.sh all diff' .)
	$(ADDLICENSE) -check -ignore 'third_party/**' -ignore 'vendor/**' .
else
	$(call docker_build, -f ./hack/Dockerfile --target sync -o . -t gmp/sync \
		--build-arg RUNCMD='./hack/presubmit.sh all' .)
	rm -rf vendor && mv vendor.tmp vendor
	rm -rf pkg/operator/generated && mv pkg/operator/generated.tmp pkg/operator/generated
	$(ADDLICENSE) -ignore 'third_party/**' -ignore 'vendor/**' .
endif

.PHONY: test
test:        ## Run unit tests. Setting NO_DOCKER=1 runs test on host machine.
             ##
	@echo ">> running unit tests"
ifeq ($(NO_DOCKER), 1)
	go test `go list ./... | grep -v e2e | grep -v export/bench | grep -v export/gcm`
else
	# TODO(TheSpiritXIII): Temporary env variables part of `export.go` unit tests.
	$(call docker_build, -f ./hack/Dockerfile --target sync -o . -t gmp/hermetic \
		--build-arg RUNCMD='GIT_TAG="$(shell git describe --tags --abbrev=0)" TEST_TAG=true ./hack/presubmit.sh test' .)
	rm -rf vendor.tmp
endif

GCM_SECRET?=
.PHONY: test-export-gcm
test-export-gcm:  ## Run export unit tests that will use GCM if GCM_SECRET is present.
                  ## TODO(b/306337101): Move to cloud build.
ifneq ($(GCM_SECRET),)
	TEST_TAG=false go test -v ./pkg/export/gcm
else
	@echo "Secret not provided, skipping!"
endif

GCM_SECRET?=
.PHONY: test-script-gcm
test-script-gcm:  ## Run example/scripts unit tests that will use GCM if GCM_SECRET is present.
                  ## TODO(b/306337101): Move to cloud build.
ifneq ($(GCM_SECRET),)
	cd examples/scripts && go test -v .
else
	@echo "Secret not provided, skipping!"
endif

# TODO(pintohutch): re-enable e2e testing against an existing K8s cluster
# (e.g. GKE cluster) without relying on a fresh kind cluster.
# This was the previous behavior of running `NO_DOCKER=1 make e2e`. But maybe
# it deserves a dedicated make target.
.PHONY: e2e
e2e:         ## Run e2e test suite against fresh kind k8s clusters.
             ## By default it does not validate metrics written to GCM.
             ## Setting GOOGLE_APPLICATION_CREDENTIALS to the path of a
             ## service account key JSON file will attempt to write and read
             ## back metric data for full e2e validation.
e2e: config-reloader operator rule-evaluator go-synthetic
	$(call ensure_registry)
# We lose some isolation by sharing the host network with the kind containers.
# However, we avoid a gcloud-shell "Dockerception" and save on build times.
#
# Run tests in parallel.
# Limit for now, due to known issues when scaling up many kind nodes:
# https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files
	@echo ">> building kindtest image"
	$(call docker_build, -f hack/Dockerfile --target kindtest -t gmp/kindtest .)
	@echo ">> running kind tests"
# TODO(pintohutch): handle SIGINTs gracefully. For now a:
# docker stop $(docker ps -a -q) && docker container prune -f
# will do the trick.
	echo $(TEST_RUN) | tr ' ' '\n' | xargs -I {} -P$(KIND_PARALLEL) \
		docker run \
		--env GOOGLE_APPLICATION_CREDENTIALS="$(LOCAL_CREDENTIALS)" \
		--env PROJECT_ID="$(PROJECT_ID)" \
		--env GMP_LOCATION="$(GMP_LOCATION)" \
		--env BINARIES="$^" \
		--env REGISTRY_NAME=$(REGISTRY_NAME) \
		--env REGISTRY_PORT=$(REGISTRY_PORT) \
		--network host \
		--rm \
		-v $(DOCKER_VOLUME):/var/run/docker.sock \
		$(E2E_DOCKER_ARGS) \
		gmp/kindtest ./hack/kind-test.sh {}

.PHONY: presubmit
presubmit:   ## Regenerate all resources, build all images and run all tests.
             ## Steps from presubmit are validated on the CI, but feel free to
             ## run it if you see CI failures related to regenerating resources
             ## or when you want to do local check before submitting.
             ##
             ## Use `CHECK=1` to fail the command if repo state is not clean
             ## after presubmit (might require committing the changes).
             ##
presubmit: updateversions regen bin test

.PHONY: updateversions
export CURRENT_TAG = v0.9.0-gke.1
export CURRENT_PROM_TAG = v2.41.0-gmp.9-gke.0
export CURRENT_AM_TAG = v0.25.1-gmp.2-gke.0
export CURRENT_RE_TAG = v0.9.0-gke.1
export CURRENT_CONFIG_RELOADER_TAG = v0.9.0-gke.1
#TODO(macxamin) Sync CURRENT_DATASOURCE_SYNCER_TAG with CURRENT_TAG
export CURRENT_DATASOURCE_SYNCER_TAG = v0.10.0-gke.3
export CURRENT_BASH_TAG = 20220419
export LABEL_API_VERSION = 0.9.0
updateversions: ## Modify all manifests, so it contains the expected versions.
                ##
                ## TODO(bwplotka): CI does not check updateversions--add that there.
                ## Also, consider moving updateversion to hack/presubmit.sh for
                ## consistency.
                ##
updateversions: $(ADDLICENSE) $(HELM) $(SED) $(YQ)
	@echo ">> Updating prometheus-engine images in manifests to $(CURRENT_TAG)"
	@$(YQ) -i '.images.operator.tag = strenv(CURRENT_TAG)' ./charts/operator/values.yaml

	@echo ">> Updating prometheus images in manifests to $(CURRENT_PROM_TAG)"
	@$(YQ) -i '.images.prometheus.tag = strenv(CURRENT_PROM_TAG)' ./charts/operator/values.yaml

	@echo ">> Updating alertmanager images in manifests to $(CURRENT_AM_TAG)"
	@$(YQ) -i '.images.alertmanager.tag = strenv(CURRENT_AM_TAG)' ./charts/operator/values.yaml

	@echo ">> Updating rule-evaluator images in manifests to $(CURRENT_RE_TAG)"
	@$(YQ) -i '.images.ruleEvaluator.tag = strenv(CURRENT_RE_TAG)' ./charts/operator/values.yaml
	@$(YQ) -i '.images.ruleEvaluator.tag = strenv(CURRENT_RE_TAG)' ./charts/rule-evaluator/values.yaml

	@echo ">> Updating app.kubernetes.io/version to $(LABEL_API_VERSION)"
	@$(YQ) -i '.appVersion = strenv(LABEL_API_VERSION)' ./charts/operator/Chart.yaml

	@echo ">> Updating constant in export.go to $(LABEL_API_VERSION)"
	@$(SED) -i -r 's#	Version    = .*#	Version    = "$(LABEL_API_VERSION)"#g' pkg/export/export.go

	@echo ">> Updating datasource-syncer version to $(CURRENT_DATASOURCE_SYNCER_TAG)"
	@$(SED) -i -r 's#image: gcr.io/gke-release/prometheus-engine/datasource-syncer:.*#image: gcr.io/gke-release/prometheus-engine/datasource-syncer:$(CURRENT_DATASOURCE_SYNCER_TAG)#g' cmd/datasource-syncer/datasource-syncer.yaml

	@$(HELM) template ./charts/operator > manifests/operator.yaml
	@$(ADDLICENSE) manifests/operator.yaml