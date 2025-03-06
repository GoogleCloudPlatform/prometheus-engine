# Auto generated binary variables helper managed by https://github.com/bwplotka/bingo v0.9. DO NOT EDIT.
# All tools are designed to be build inside $GOBIN.
BINGO_DIR := $(dir $(lastword $(MAKEFILE_LIST)))
GOPATH ?= $(shell go env GOPATH)
GOBIN  ?= $(firstword $(subst :, ,${GOPATH}))/bin
GO     ?= $(shell which go)

# Below generated variables ensure that every time a tool under each variable is invoked, the correct version
# will be used; reinstalling only if needed.
# For example for addlicense variable:
#
# In your main Makefile (for non array binaries):
#
#include .bingo/Variables.mk # Assuming -dir was set to .bingo .
#
#command: $(ADDLICENSE)
#	@echo "Running addlicense"
#	@$(ADDLICENSE) <flags/args..>
#
ADDLICENSE := $(GOBIN)/addlicense-v1.1.1
$(ADDLICENSE): $(BINGO_DIR)/addlicense.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/addlicense-v1.1.1"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=addlicense.mod -o=$(GOBIN)/addlicense-v1.1.1 "github.com/google/addlicense"

CONTROLLER_GEN := $(GOBIN)/controller-gen-v0.17.1-0.20250103184936-50893dee96da
$(CONTROLLER_GEN): $(BINGO_DIR)/controller-gen.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/controller-gen-v0.17.1-0.20250103184936-50893dee96da"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=controller-gen.mod -o=$(GOBIN)/controller-gen-v0.17.1-0.20250103184936-50893dee96da "sigs.k8s.io/controller-tools/cmd/controller-gen"

GEN_CRD_API_REFERENCE_DOCS := $(GOBIN)/gen-crd-api-reference-docs-v0.3.0
$(GEN_CRD_API_REFERENCE_DOCS): $(BINGO_DIR)/gen-crd-api-reference-docs.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/gen-crd-api-reference-docs-v0.3.0"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=gen-crd-api-reference-docs.mod -o=$(GOBIN)/gen-crd-api-reference-docs-v0.3.0 "github.com/ahmetb/gen-crd-api-reference-docs"

GOLANGCI_LINT := $(GOBIN)/golangci-lint-v1.64.6
$(GOLANGCI_LINT): $(BINGO_DIR)/golangci-lint.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/golangci-lint-v1.64.6"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=golangci-lint.mod -o=$(GOBIN)/golangci-lint-v1.64.6 "github.com/golangci/golangci-lint/cmd/golangci-lint"

HELM := $(GOBIN)/helm-v3.14.0
$(HELM): $(BINGO_DIR)/helm.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/helm-v3.14.0"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=helm.mod -o=$(GOBIN)/helm-v3.14.0 "helm.sh/helm/v3/cmd/helm"

MDOX := $(GOBIN)/mdox-v0.9.0
$(MDOX): $(BINGO_DIR)/mdox.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/mdox-v0.9.0"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=mdox.mod -o=$(GOBIN)/mdox-v0.9.0 "github.com/bwplotka/mdox"

YQ := $(GOBIN)/yq-v4.40.7
$(YQ): $(BINGO_DIR)/yq.mod
	@# Install binary/ries using Go 1.14+ build command. This is using bwplotka/bingo-controlled, separate go module with pinned dependencies.
	@echo "(re)installing $(GOBIN)/yq-v4.40.7"
	@cd $(BINGO_DIR) && GOWORK=off $(GO) build -mod=mod -modfile=yq.mod -o=$(GOBIN)/yq-v4.40.7 "github.com/mikefarah/yq/v4"

