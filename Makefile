include .env
export
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export VERSION=(unknown)
GO := go
TEST := $(GO) test
APPSOURCES := $(wildcard ./app/*.go ./app/*/*.go)

ifeq ($(ENV),)
	ENV := dev
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
endif

BUILD := $(GO) build -a -ldflags '-X main.version=$(VERSION) -extldflags "-static"'

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

clean:
	$(RM) bin/*
	cd docker && $(MAKE) $@

run: app
	@./bin/app -host $(HOSTNAME)

cert:
	cd docker && $(MAKE) $@

images:
	cd docker && $(MAKE) $@

.PHONY: tests
tests:
	$(TEST) ./...
	cd tests && $(MAKE) bootstrap
	cd tests && $(MAKE) $@
