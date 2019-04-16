export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export VERSION=(unknown)
GO := go
ENV ?= dev
LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
APPSOURCES := $(wildcard ./app/*.go ./app/*/*.go) main.go

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all cli run clean cert images test

all: app cli

app: bin/app
bin/app: go.mod main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./main.go

bootstrap: bin/bootstrap
bin/bootstrap: go.mod cli/bootstrap/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ cli/bootstrap/main.go

votes: bin/votes
bin/votes: go.mod cli/votes/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ cli/votes/main.go

keys: bin/keys
bin/keys: go.mod cli/keys/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ cli/keys/main.go

poach: bin/poach
bin/poach: go.mod cli/poach/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ cli/poach/main.go

fetcher: bin/fetcher
bin/fetcher: go.mod cli/fetcher/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ cli/fetcher/main.go

cli: bootstrap votes keys poach fetcher

run: app
	@./bin/app

clean:
	-$(RM) bin/*
	$(MAKE) -C docker $@
	$(MAKE) -C tests $@

cert:
	$(MAKE) -C docker $@

images:
	$(MAKE) -C docker $@

test: app
	$(TEST) ./{app,cli,internal}/...
	$(MAKE) -C tests $@
