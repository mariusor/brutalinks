include .env
export 
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export VERSION=unknown

GO := go
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'
SOURCES := $(wildcard ./app/*/*.go)

ifeq ($(ENV),)
	ENV := dev
endif

all: app cli

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
endif

app: bin/app
bin/app: main.go $(SOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./main.go

bootstrap: bin/bootstrap
bin/bootstrap: cli/bootstrap/main.go
	$(BUILD) -tags $(ENV) -o $@ cli/bootstrap/main.go

votes: bin/votes
bin/votes: cli/votes/main.go
	$(BUILD) -tags $(ENV) -o $@ cli/votes/main.go

keys: bin/keys
bin/keys: cli/keys/main.go
	$(BUILD) -tags $(ENV) -o $@ cli/keys/main.go

cli: bootstrap votes keys

clean:
	$(RM) bin/*
	cd docker && $(MAKE) $@

run: app
	@./bin/app
	cd docker && $(MAKE) $@

assets: app cli
	mkdir -p docker/{app,bootstrap}/bin
	cp bin/app docker/app/bin/
	cp bin/bootstrap docker/bootstrap/bin/
	cp -r templates docker/app
	cp -r assets docker/app
	cp -r db docker/bootstrap

cert:
	cd docker && $(MAKE) $@

image: app assets
	cd docker && $(MAKE) $@

compose: app assets
	cd docker && $(MAKE) $@
