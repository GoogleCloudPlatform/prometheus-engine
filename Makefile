GOAPPS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))

CLOUDSDK_CONFIG?=${HOME}/.config/gcloud
PROJECT_ID?=$(shell gcloud config get-value core/project)
GMP_CLUSTER?=gmp-test
GMP_LOCATION?=us-central1-c
API_DIR=pkg/operator/apis

TAG_NAME?=$(shell date "+gmp-%Y%d%m_%H%M")

define docker_build
	DOCKER_BUILDKIT=1 docker build $(1)
endef

help:        ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

all:         ## Build all go binaries.
all: $(GOAPPS)

clean:       ## Clean build time resources, primarily docker resources.
	docker container prune -f
	docker volume prune -f
	for i in `docker image ls | grep ^gmp/ | awk '{print $$1}'`; do docker image rm $$i; done

$(GOAPPS):   ## Build go binary in cmd/ (e.g. 'operator').
             ## Set 'DOCKER=1' env var to build within Docker instead of natively.
	@echo ">> building binaries"
ifeq ($(DOCKER),1)
	$(call docker_build, --tag gmp/$@ -f ./cmd/$@/Dockerfile .)
	mkdir -p build/bin
	echo -e 'FROM scratch\nCOPY --from=gmp/$@ /bin/$@ /$@' | $(call docker_build, -o ./build/bin -)
else
	CGO_ENABLED=0 go build -mod=vendor -o ./build/bin/$@ ./cmd/$@/*.go
endif

cloudbuild:  ## Build images on Google Cloud Build.
	@echo ">> building GMP images on Cloud Build with tag: $(TAG_NAME)"
	gcloud builds submit --config build.yaml --timeout=30m --substitutions=TAG_NAME="$(TAG_NAME)"

.PHONY: assets
assets:      ## Build and write UI assets to local go file.
	@echo ">> writing static assets to host machine"
	$(call docker_build, -f ./cmd/frontend/Dockerfile --target assets --tag gmp/assets .)
	echo -e 'FROM scratch\nCOPY --from=gmp/assets /app/pkg/ui/assets_vfsdata.go pkg/ui/assets_vfsdata.go' | $(call docker_build, -o . -)

test:        ## Run all tests. Writes real data to GCM API under PROJECT_ID environment variable.
             ## Use GMP_CLUSTER, GMP_LOCATION to specify timeseries labels.
	@echo ">> running tests"
ifeq ($(DOCKER), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic --build-arg RUNCMD='go test `go list ./... | grep -v operator/e2e`')
else
	go test `go list ./... | grep -v operator/e2e`
	go test `go list ./... | grep operator/e2e` -args -project-id=${PROJECT_ID} -cluster=${GMP_CLUSTER} -location=${GMP_LOCATION}
endif

kindclean:   ## Clean previous kind state.
kindclearn: clean
	docker volume rm -f gcloud-config

kindtest:    ## Run e2e test suite against fresh kind k8s cluster.
kindtest: kindclean
	@echo ">> building image"
	$(call docker_build, --tag gmp/kindtest -f hack/Dockerfile --target kindtest .)
	@echo ">> creating tmp gcloud config volume"
	docker volume create gcloud-config
	docker create -v gcloud-config:/data --name tmp busybox true
	docker cp $(CLOUDSDK_CONFIG) tmp:/data
	docker rm tmp
	@echo ">> running container"
	docker run --rm -v gcloud-config:/root/.config gmp/kindtest ./hack/kind-test.sh
	docker volume rm -f gcloud-config

format:      ## Format code.
             ## Set 'DRY_RUN=1' to verify if code is properly formatted.
	@echo ">> formatting code"
ifeq ($(DRY_RUN), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='go mod tidy && go mod vendor && go fmt ./... && git diff --exit-code go.mod go.sum *.go')
else
	$(call docker_build, . --target sync -o . -t gmp/sync \
		--build-arg RUNCMD='go mod tidy && go mod vendor && go fmt ./...')
endif

lint:        ## Lint code.
	@echo ">> linting code"
	DOCKER_BUILDKIT=1 docker run --rm -v $(pwd):/app -w /app golangci/golangci-lint:v1.43.0 golangci-lint run -v

codegen:     ## Refresh generated CRD go interfaces.
             ## Set 'DRY_RUN=1' to verify if latest code was regenerated.
	@echo ">> regenerating go apis"
ifeq ($(DRY_RUN), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/update-codegen.sh && git diff --exit-code *.go')
else
	@echo ">>> checking if there are uncommitted api code changes"
	git diff -s --exit-code $(API_DIR) || $(call docker_build, . --target sync -o . -t gmp/sync \
		--build-arg RUNCMD=./hack/update-codegen.sh)
endif

crdgen:      ## Refresh CRD OpenAPI YAML specs.
             ## Set 'DRY_RUN=1' to verify if latest manifest was regenerated.
ifeq ($(DRY_RUN), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/update-crdgen.sh && git diff --exit-code *.yaml')
else
	$(call docker_build, . --target sync -o . -t gmp/sync \
		--build-arg RUNCMD=./hack/update-crdgen.sh)
endif

examples:
ifeq ($(DRY_RUN), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/update-examples.sh && git diff --exit-code *.yaml')
else
	$(call docker_build, . --target hermetic -t gmp/sync \
		--build-arg RUNCMD='./hack/update-examples.sh')
endif

docgen:      ## Refresh API markdown documentation.
             ## Set 'DRY_RUN=1' to verify if latest API docs were regnerated.
ifeq ($(DRY_RUN), 1)
	$(call docker_build, . --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/update-docgen.sh && git diff --exit-code doc')
else
	$(call docker_build, . --target sync -o . -t gmp/sync \
		--build-arg RUNCMD=./hack/update-docgen.sh)
endif


presubmit: codegen assets format crdgen examples docgen test kindtest
