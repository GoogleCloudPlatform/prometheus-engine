GOAPPS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))

CLOUDSDK_CONFIG?=${HOME}/.config/gcloud
PROJECT_ID?=$(shell gcloud config get-value core/project)
GMP_CLUSTER?=gmp-test
GMP_LOCATION?=us-central1-c

TAG_NAME?=$(shell date "+gmp-%Y%d%m_%H%M")

help:        ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

all:         ## Build all go binaries.
all: $(GOAPPS)

docker:
	$(foreach a,$(GOAPPS),DOCKER_BUILDKIT=1 docker build --tag gmp/$(a) -f ./cmd/$(a)/Dockerfile . ;)

$(GOAPPS):   ## Build go binary in cmd/ (e.g. 'operator').
             ## Set 'DOCKER_BUILD=1' env var to build within Docker instead of natively.
ifeq ($(DOCKER_BUILD),1)
	DOCKER_BUILDKIT=1 docker build --tag gmp/$@ -f ./cmd/$@/Dockerfile .
	mkdir -p build/bin
	echo -e 'FROM scratch\nCOPY --from=gmp/$@ /bin/$@ /$@' | DOCKER_BUILDKIT=1 docker build -o ./build/bin -
else
	CGO_ENABLED=0 go build -mod=vendor -o ./build/bin/$@ ./cmd/$@/*.go
endif

.PHONY: format
format:      ## Format code.
	@echo ">> formatting code"
	go fmt ./...

.PHONY: vet
vet:         ## Vet code.
	@echo ">> vetting code"
	go vet ./...

.PHONY: assets
assets:      ## Build and write UI assets as go file.
	@echo ">> writing static assets to host machine"
	DOCKER_BUILDKIT=1 docker build -f ./cmd/frontend/Dockerfile --target assets --tag gmp-tmp/assets .
	echo -e 'FROM scratch\nCOPY --from=gmp-tmp/assets /app/pkg/ui/assets_vfsdata.go pkg/ui/assets_vfsdata.go' | DOCKER_BUILDKIT=1 docker build -o . -
	docker image rm gmp-tmp/assets

test:        ## Run all tests. Writes real data to GCM API under PROJECT_ID environment variable.
             ## Use GMP_CLUSTER, GMP_LOCATION to specify timeseries labels.
	@echo ${PROJECT_ID}
	go test `go list ./... | grep -v operator/e2e`
	go test `go list ./... | grep operator/e2e` -args -project-id=${PROJECT_ID} -cluster=${GMP_CLUSTER} -location=${GMP_LOCATION}

codegen:     ## Refresh generated CRD go interfaces.
	./hack/update-codegen.sh

crds:        ## Refresh CRD OpenAPI YAML specs.
	./hack/update-crdgen.sh

docgen:      ## Refresh API markdown documentation.
	mkdir -p doc
	which po-docgen || (go get github.com/prometheus-operator/prometheus-operator && go install -mod=mod github.com/prometheus-operator/prometheus-operator/cmd/po-docgen)
	po-docgen api ./pkg/operator/apis/monitoring/v1alpha1/types.go > doc/api.md
	sed -i 's/Prometheus Operator/GMP CRDs/g' doc/api.md

kindclean:   ## Clean previous kind state.
	docker container prune -f
	docker volume prune -f
	docker volume rm -f gcloud-config

kindtest:    ## Run e2e test suite against fresh kind k8s cluster.
kindtest: kindclean
	@echo ">> building image"
	DOCKER_BUILDKIT=1 docker build --tag gmp/kindtest -f hack/Dockerfile --target kindtest .
	@echo ">> creating tmp gcloud config volume"
	docker volume create gcloud-config
	docker create -v gcloud-config:/data --name tmp busybox true
	docker cp $(CLOUDSDK_CONFIG) tmp:/data
	docker rm tmp
	@echo ">> running container"
	docker run --rm -v gcloud-config:/root/.config gmp/kindtest ./hack/kind-test.sh
	docker volume rm -f gcloud-config

cloudbuild:  ## Build images on Google Cloud Build.
	@echo ">> building GMP images on Cloud Build with tag: $(TAG_NAME)"
	gcloud builds submit --config build.yaml --timeout=30m --substitutions=TAG_NAME="$(TAG_NAME)"
