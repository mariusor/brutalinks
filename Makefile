export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export VERSION=(unknown)
GO := go
ENV ?= dev
LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
APPSOURCES := $(wildcard  ./activitypub/*.go ./app/*.go ./app/*/*.go internal/*/*.go) main.go

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always --dirty=-git)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all run clean images test

all: app

app: bin/app
bin/app: go.mod main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./main.go

run: app
	@./bin/app

clean:
	-$(RM) bin/*
	$(MAKE) -C docker $@

images:
	$(MAKE) -C docker $@

test: app
	$(TEST) ./{app,internal}/...
