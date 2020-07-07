export CGO_ENABLED=0
export VERSION=(unknown)
GO := go
ENV ?= dev
LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -trimpath -a -ldflags '$(LDFLAGS)'
APPSOURCES := $(wildcard ./app/*.go internal/*/*.go)
ASSETFILES := $(wildcard templates/* templates/partials/* templates/partials/*/* assets/*/* assets/*)

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	APPSOURCES += internal/assets/assets.gen.go
assets: internal/assets/assets.gen.go
endif

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --always --dirty=-git)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
export VERSION = $(shell git describe --tags)
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all run clean images test assets

all: app

assets:

internal/assets/assets.gen.go: $(ASSETFILES)
	go generate -tags $(ENV) ./assets.go

app: bin/app
bin/app: go.mod ./cli/app/main.go $(APPSOURCES)
	$(BUILD) -tags $(ENV) -o $@ ./cli/app/main.go

run: app
	@./bin/app

clean:
	-$(RM) bin/* internal/assets/assets.gen.go
	$(MAKE) -C docker $@

images:
	$(MAKE) -C docker $@

test: app
	$(TEST) ./{app,internal}/...
