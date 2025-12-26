SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

PROJECT_NAME := brutalinks
APP_HOSTNAME ?= $(PROJECT_NAME)
ENV ?= dev

LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
TEST_FLAGS ?= -count=1

UPX = upx
GO ?= go
APPSOURCES := $(wildcard ./*.go internal/*/*.go cmd/brutalinks/*.go)
ASSETFILES := $(wildcard assets/*/* assets/*)

export CGO_ENABLED=0
export GOEXPERIMENT=greenteagc

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
	HASH=$(shell git rev-parse --short HEAD)
	VERSION ?= $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION ?= $(shell git describe --tags | tr '/' '-')
endif

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all cert brutalinks clean test images test assets download help

.DEFAULT_GOAL := help

help: ## Help target that shows this message.
	@sed -rn 's/^([^:]+):.*[ ]##[ ](.+)/\1:\2/p' $(MAKEFILE_LIST) | column -ts: -l2

all: brutalinks ##

download: go.sum ## Downloads dependencies and tidies the go.mod file.

go.sum: go.mod
	$(GO) mod download all
	$(GO) mod tidy

brutalinks: bin/brutalinks ## Builds the brutalinks binary.
bin/brutalinks: go.mod go.sum $(APPSOURCES) assets
	$(BUILD) -tags $(ENV) -o $@ ./cmd/brutalinks
ifneq ($(ENV),dev)
	$(UPX) -q --mono --no-progress --best $@ || true
endif

assets: ## Builds static javascript and css files for embedding in the production binaries.
ifneq ($(ENV),dev)
assets: internal/assets/assets.gen.go
endif

internal/assets/assets.gen.go: $(ASSETFILES)
	$(GO) run -tags $(ENV) ./internal/assets/cmd/minify.go -build "prod || qa" -glob assets/*,assets/css/*,assets/js/*,README.md -var AssetFS -o ./internal/assets/assets.gen.go

cert: bin/$(APP_HOSTNAME).pem ## Create a certificate.
bin/$(APP_HOSTNAME).pem: bin/$(APP_HOSTNAME).key bin/$(APP_HOSTNAME).crt
bin/$(APP_HOSTNAME).key bin/$(APP_HOSTNAME).crt:
	./tools/gen-certs.sh ./bin/$(APP_HOSTNAME)

clean: ## Cleanup the build workspace.
	-$(RM) -r bin/* internal/assets/*.gen.go

images: ## Build podman images.
	$(MAKE) -C images $@

test: TEST_TARGET := . ./internal/...
test: download go.sum ## Run unit tests for the service.
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: ## Run unit tests for the service with coverage.
	mkdir -p /tmp/brutalinks-coverage
	$(TEST) $(TEST_FLAGS) -coverpkg git.sr.ht/~mariusor/brutalinks -covermode=count -args -test.gocoverdir="/tmp/brutalinks-coverage" $(TEST_TARGET)
	$(GO) tool covdata percent -i=/tmp/brutalinks-coverage -o $(PROJECT).coverprofile
	$(RM) -r /tmp/brutalinks-coverage

integration: download ## Run integration tests for the service.
	if [ -z "${IMAGE}" ]; then
		make ENV=dev -C images builder build
	fi
	make -C tests pods test-podman
