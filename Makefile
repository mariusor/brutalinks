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
APPSOURCES := $(wildcard ./app/*.go internal/*/*.go)
ASSETFILES := $(wildcard templates/* templates/partials/* templates/partials/*/* assets/*/* assets/*)

export CGO_ENABLED=0

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

ifeq ($(VERSION), )
	ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
		BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
		HASH=$(shell git rev-parse --short HEAD)
		VERSION ?= $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
	endif
	ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
		VERSION ?= $(shell git describe --tags | tr '/' '-')
	endif
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all run clean images test assets download

all: brutalinks

brutalinks: bin/brutalinks

download:
	$(GO) mod download all
	$(GO) mod tidy

ifneq ($(ENV), dev)
assets: $(ASSETFILES) download
	go generate -tags $(ENV) ./assets.go
else
assets:
endif

bin/brutalinks: download assets go.mod cmd/brutalinks/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./cmd/brutalinks/main.go

run: ./bin/brutalinks
	@./bin/brutalinks

clean:
	-$(RM) bin/* internal/assets/*.gen.go
	$(GO) build clean
	$(MAKE) -C images $@

images:
	$(MAKE) -C images $@

test: TEST_TARGET := ./{app,internal}/...
test: download
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test

integration: ENV=prod
integration: assets download
	make -C tests
