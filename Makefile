SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:

PROJECT_NAME := $(shell basename $(PWD))
ENV ?= dev
LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -trimpath -a -ldflags '$(LDFLAGS)'
TEST_FLAGS ?= -count=1

GO := go
APPSOURCES := $(wildcard ./app/*.go internal/*/*.go)
ASSETFILES := $(wildcard templates/* templates/partials/* templates/partials/*/* assets/*/* assets/*)

export CGO_ENABLED=0
export VERSION=(unknown)

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	APPSOURCES += internal/assets/assets.gen.go

assets: internal/assets/assets.gen.go
else:
assets:
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

all: app

download:
	$(GO) mod tidy

internal/assets/assets.gen.go: $(ASSETFILES)
	go generate -tags $(ENV) ./assets.go

app: bin/app
bin/app: go.mod cmd/app/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./cmd/app/main.go

run: app
	@./bin/app

clean:
	-$(RM) bin/* internal/assets/assets.gen.go
	$(MAKE) -C docker $@

images:
	$(MAKE) -C docker $@

test: TEST_TARGET := ./{app,internal}/...
test: app
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test
