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

cli: bootstrap votes keys poach

clean:
	$(RM) bin/*
	cd docker && $(MAKE) $@

run: app
	@./bin/app

assets: app cli
	mkdir -p docker/{app,bootstrap}/bin
	cp bin/app docker/app/bin/
	cp bin/bootstrap docker/bootstrap/bin/
	cp -r templates docker/app
	cp -r assets docker/app
	cp -r db docker/bootstrap

cert:
	cd docker && $(MAKE) $@

image: all assets
	cd docker && $(MAKE) $@

compose: all assets
	cd docker && $(MAKE) $@

.PHONY: test
test:
	$(TEST) -v ./...
