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

app: main.go $(SOURCES)
	$(BUILD) -tags $(ENV) -o bin/$@ ./main.go

bootstrap: cli/bootstrap/main.go
	$(BUILD) -tags $(ENV) -o bin/$@ cli/bootstrap/main.go

votes: cli/votes/main.go
	$(BUILD) -tags $(ENV) -o bin/$@ cli/votes/main.go

keys: cli/keys/main.go
	$(BUILD) -tags $(ENV) -o bin/$@ cli/keys/main.go

cli: bootstrap votes keys

clean:
	$(RM) bin/*
	$(RM) -rf docker/bin
	$(RM) -rf docker/templates
	$(RM) -rf docker/assets
	$(RM) -rf docker/{app,bootstrap}/bin
	$(RM) -rf docker/app/templates
	$(RM) -rf docker/app/assets
	$(RM) -rf docker/bootstrap/db

run: app
	@./bin/app

assets: app cli
	mkdir -p docker/{app,bootstrap}/bin
	cp bin/app docker/app/bin/
	cp bin/bootstrap docker/bootstrap/bin/
	cp -r templates docker/app
	cp -r assets docker/app
	cp -r db docker/bootstrap

image: app assets
	cd docker && $(MAKE) $@

compose: app assets
	cd docker && $(MAKE) $@
