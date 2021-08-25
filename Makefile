GOAPPS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))

CLOUDSDK_CONFIG?=${HOME}/.config/gcloud

help:        ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

all:         ## Build all go binaries.
all: $(GOAPPS)

docker:      ## Build docker images for all go binaries.
	$(foreach a,$(GOAPPS),DOCKER_BUILDKIT=1 docker build --tag gpe/$(a) -f ./cmd/$(a)/Dockerfile . ;)

$(GOAPPS):   ## Build go binary in cmd/ (e.g. 'operator').
             ## Set 'DOCKER_BUILD=1' env var to build within Docker instead of natively.
ifeq ($(DOCKER_BUILD),1)
	DOCKER_BUILDKIT=1 docker build --tag gpe/$@ -f ./cmd/$@/Dockerfile .
	mkdir -p build/bin
	echo -e 'FROM scratch\nCOPY --from=gpe/$@ /bin/$@ /$@' | DOCKER_BUILDKIT=1 docker build -o ./build/bin -
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
	DOCKER_BUILDKIT=1 docker build -f ./cmd/frontend/Dockerfile --target assets --tag gpe-tmp/assets .
	echo -e 'FROM scratch\nCOPY --from=gpe-tmp/assets /app/pkg/ui/assets_vfsdata.go pkg/ui/assets_vfsdata.go' | DOCKER_BUILDKIT=1 docker build -o . -
	docker image rm gpe-tmp/assets

test:        ## Run all unit tests.
	go test `go list ./... | grep -v operator/e2e`
	go test -short `go list ./... | grep operator/e2e` -args -project-id=test-proj -cluster=test-cluster

codegen:     ## Refresh generated CRD go interfaces.
	./hack/update-codegen.sh

crds:        ## Refresh CRD OpenAPI YAML specs.
	./hack/update-crdgen.sh

kindclean:   ## Clean previous kind state.
	docker container prune -f
	docker volume prune -f
	docker volume rm -f gcloud-config

kindtest:    ## Run e2e test suite against fresh kind k8s cluster.
kindtest: kindclean
	@echo ">> building image"
	DOCKER_BUILDKIT=1 docker build --tag gpe/kindtest -f hack/Dockerfile --target kindtest .
	@echo ">> creating tmp gcloud config volume"
	docker volume create gcloud-config
	docker create -v gcloud-config:/data --name tmp busybox true
	docker cp $(CLOUDSDK_CONFIG) tmp:/data
	docker rm tmp
	@echo ">> running container"
	docker run --rm -v gcloud-config:/root/.config gpe/kindtest ./hack/kind-test.sh
