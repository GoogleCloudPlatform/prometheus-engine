.PHONY: format
format:
	@echo ">> formatting code"
	go fmt ./...

.PHONY: vet
vet:
	@echo ">> vetting code"
	go vet ./...

UI_REACT_BASE=pkg/ui/build

.PHONY: assets
assets:
	@echo ">> writing assets"
	pkg/ui/build.sh

