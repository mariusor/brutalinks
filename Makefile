include .env
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

app: main.go $(SOURCES)
	$(BUILD) -tags $(ENV) -o bin/$@ ./main.go

bootstrap: cli/bootstrap/main.go
	$(BUILD) -o bin/$@ cli/bootstrap/main.go

votes: cli/votes/main.go
	$(BUILD) -o bin/$@ cli/votes/main.go

keys: cli/keys/main.go
	$(BUILD) -o bin/$@ cli/keys/main.go

cli: bootstrap votes keys

clean:
	$(RM) bin/*
	$(RM) -rf docker/bin
	$(RM) -rf docker/templates
	$(RM) -rf docker/assets
	$(RM) -rf docker/app/bin
	$(RM) -rf docker/app/templates
	$(RM) -rf docker/app/assets
	$(RM) -rf docker/app/db

run: app
	@./bin/app

assets: app cli
	cp -r bin docker/app
	cp -r templates docker/app
	cp -r assets docker/app
	cp -r db docker/app

image: app assets
	cd docker && $(MAKE) $@

compose: app assets
	cd docker && $(MAKE) $@
