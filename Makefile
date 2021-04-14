GOAPPS := $(notdir $(patsubst %/,%,$(dir $(shell find cmd -name 'main.go'))))

all: $(GOAPPS)

docker:
	$(foreach a,$(GOAPPS),DOCKER_BUILDKIT=1 docker build --tag gpe/$(a) -f ./cmd/$(a)/Dockerfile . ;)

$(GOAPPS):
ifeq ($(DOCKER_BUILD),1)
	DOCKER_BUILDKIT=1 docker build --tag gpe/$@ -f ./cmd/$@/Dockerfile .
	mkdir -p build/bin
	echo -e 'FROM scratch\nCOPY --from=gpe/$@ /bin/$@ /$@' | DOCKER_BUILDKIT=1 docker build -o ./build/bin -
else
	CGO_ENABLED=0 go build -mod=mod -o ./build/bin/$@ ./cmd/$@/*.go
endif

.PHONY: format
format:
	@echo ">> formatting code"
	go fmt ./...

.PHONY: vet
vet:
	@echo ">> vetting code"
	go vet ./...

.PHONY: assets
assets:
	@echo ">> writing static assets to host machine"
	DOCKER_BUILDKIT=1 docker build -f ./cmd/frontend/Dockerfile --target assets --tag gpe-tmp/assets .
	echo -e 'FROM scratch\nCOPY --from=gpe-tmp/assets /app/pkg/ui/assets_vfsdata.go pkg/ui/assets_vfsdata.go' | DOCKER_BUILDKIT=1 docker build -o . -
	docker image rm gpe-tmp/assets
