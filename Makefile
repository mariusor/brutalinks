SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:

PROJECT_NAME := $(shell basename $(PWD))
ENV ?= dev
LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
TEST_FLAGS ?= -count=1

GO ?= go
APPSOURCES := $(wildcard ./app/*.go internal/*/*.go)
ASSETFILES := $(wildcard templates/* templates/partials/* templates/partials/*/* assets/*/* assets/*)

export CGO_ENABLED=0
export VERSION=HEAD

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always --dirty=-git)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
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
	$(MAKE) -C docker $@

images:
	$(MAKE) -C docker $@

test: TEST_TARGET := ./{app,internal}/...
test: download
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test

integration: ENV=prod
integration: assets download
	make -C tests
