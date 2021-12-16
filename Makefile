GOAPPS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))

CLOUDSDK_CONFIG?=${HOME}/.config/gcloud
PROJECT_ID?=$(shell gcloud config get-value core/project)
GMP_CLUSTER?=gmp-test-cluster
GMP_LOCATION?=us-central1-c
API_DIR=pkg/operator/apis

TAG_NAME?=$(shell date "+gmp-%Y%d%m_%H%M")

define docker_build
	DOCKER_BUILDKIT=1 docker build $(1)
endef

define assets_diff
	git fetch origin
	git diff -s --exit-code origin/main -- third_party
endef

help:        ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

all:         ## Build all go binaries.
all: $(GOAPPS)

clean:       ## Clean build time resources, primarily docker resources.
	docker container prune -f
	docker volume prune -f
	for i in `docker image ls | grep ^gmp/ | awk '{print $$1}'`; do docker image rm -f $$i; done

$(GOAPPS):   ## Build go binary in cmd/ (e.g. 'operator').
             ## Set NO_DOCKER=1 env var to build natively without Docker.
	@echo ">> building binaries"
ifeq ($(NO_DOCKER),1)
	CGO_ENABLED=0 go build -mod=vendor -o ./build/bin/$@ ./cmd/$@/*.go
else
	$(call docker_build, --tag gmp/$@ -f ./cmd/$@/Dockerfile .)
	mkdir -p build/bin
	echo -e 'FROM scratch\nCOPY --from=gmp/$@ /bin/$@ /$@' | $(call docker_build, -o ./build/bin -)
endif

cloudbuild:  ## Build images on Google Cloud Build.
	@echo ">> building GMP images on Cloud Build with tag: $(TAG_NAME)"
	gcloud builds submit --config build.yaml --timeout=30m --substitutions=TAG_NAME="$(TAG_NAME)"

.PHONY: assets
assets:      ## Build and write UI assets to local go file.
	@echo ">> writing static assets to host machine"
	$(call assets_diff) || $(call docker_build, -f ./cmd/frontend/Dockerfile --target sync -o . -t gmp/assets-sync .)

test:        ## Run all tests. Setting NO_DOCKER=1 writes real data to GCM API under PROJECT_ID environment variable.
             ## Use GMP_CLUSTER, GMP_LOCATION to specify timeseries labels.
	@echo ">> running tests"
ifeq ($(NO_DOCKER), 1)
	kubectl apply -f examples/setup.yaml
	kubectl apply -f cmd/operator/deploy/operator/01-priority-class.yaml
	kubectl apply -f cmd/operator/deploy/operator/03-clusterrole.yaml
	go test `go list ./... | grep -v operator/e2e`
	go test `go list ./... | grep operator/e2e` -args -project-id=${PROJECT_ID} -cluster=${GMP_CLUSTER} -location=${GMP_LOCATION}
else
	$(call docker_build, -f ./hack/Dockerfile --target hermetic -t gmp/hermetic \
	--build-arg RUNCMD='go test `go list ./... | grep -v operator/e2e`' .)
endif

kindclean: clean
	docker volume rm -f gcloud-config

kindtest:    ## Run e2e test suite against fresh kind k8s cluster.
kindtest: kindclean
	@echo ">> building image"
	$(call docker_build, -f hack/Dockerfile --target kindtest -t gmp/kindtest .)
	@echo ">> creating tmp gcloud config volume"
	docker volume create gcloud-config
	docker create -v gcloud-config:/data --name tmp busybox true
	docker cp $(CLOUDSDK_CONFIG) tmp:/data
	docker rm tmp
	@echo ">> running container"
	docker run --rm -v gcloud-config:/root/.config gmp/kindtest ./hack/kind-test.sh
	docker volume rm -f gcloud-config

lint:        ## Lint code.
	@echo ">> linting code"
	DOCKER_BUILDKIT=1 docker run --rm -v $(pwd):/app -w /app golangci/golangci-lint:v1.43.0 golangci-lint run -v

presubmit:   ## Validate and regenerate changes before submitting a PR 
             ## Use DRY_RUN=1 to only validate without regenerating changes.
presubmit: ps assets operator rule-evaluator config-reloader kindtest
ps:  
ifeq ($(DRY_RUN), 1)
	$(call docker_build, -f ./hack/Dockerfile --target hermetic -t gmp/hermetic \
		--build-arg RUNCMD='./hack/presubmit.sh all diff' .)
else
	$(call docker_build, -f ./hack/Dockerfile --target sync -o . -t gmp/sync \
		--build-arg RUNCMD='./hack/presubmit.sh' .)
	rm -rf vendor && mv vendor.tmp vendor
endif