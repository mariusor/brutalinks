SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

PROJECT_NAME := brutalinks
ENV ?= dev

LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
TEST_FLAGS ?= -count=1

GO ?= go
APPSOURCES := $(wildcard ./*.go internal/*/*.go cmd/brutalinks/*.go)
ASSETFILES := $(wildcard assets/*/* assets/*)

export CGO_ENABLED=0

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
	HASH=$(shell git rev-parse --short HEAD)
	VERSION ?= $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION ?= $(shell git describe --tags | tr '/' '-')
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all brutalinks download run clean images test assets help
.DEFAULT_GOAL := help
help: ## Help target that shows this message.
	@sed -rn 's/^([^:]+):.*[ ]##[ ](.+)/\1:\2/p' $(MAKEFILE_LIST) | column -ts: -l2

all: brutalinks

download: go.sum

go.sum: go.mod
	$(GO) mod download all
	$(GO) mod tidy

brutalinks: bin/brutalinks ## Builds the brutalinks binary.

ifneq ($(ENV),dev)
assets:  internal/assets/assets.gen.go
else
assets: ## Builds static javascript and css files for embedding in the production binaries.
endif

internal/assets/assets.gen.go: $(ASSETFILES)
	$(GO) run -tags $(ENV) ./internal/assets/cmd/minify.go -build "prod || qa" -glob assets/*,assets/css/*,assets/js/*,README.md -var AssetFS -o ./internal/assets/assets.gen.go

bin/brutalinks: go.mod go.sum $(APPSOURCES) assets
	$(BUILD) -tags $(ENV) -o $@ ./cmd/brutalinks

run: ./bin/brutalinks ## Runs the brutalinks binary.
	@./bin/brutalinks

clean: ## Cleanup the build workspace.
	-$(RM) bin/* internal/assets/*.gen.go
	$(GO) build clean
	$(MAKE) -C images $@

images: ## Build podman images.
	$(MAKE) -C images $@

test: TEST_TARGET := . ./internal/...
test: download go.sum ## Run unit tests for the service.
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test ## Run unit tests for the service with coverage.

integration: download ## Run integration tests for the service.
	make ENV=qa -C images builder build
	make ENV=qa -C tests pods test-podman
