export CGO_ENABLED=0
GO := go
TEST_FLAGS ?= -count=1 -v
STORAGE ?= all

ENV ?= test
BUILDFLAGS ?= -a
TAGS =
TEST_TARGET = .

TEST := $(GO) test $(BUILDFLAGS)

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
pwd := $(abspath $(dir mkfile_path))

HAVE_PODMAN_COMPOSE := $(shell podman-compose -v &2> /dev/null)
ifdef HAVE_PODMAN_COMPOSE
	COMPOSE_UP=podman-compose up
	COMPOSE_DOWN=podman-compose down
else
	COMPOSE_UP=docker-compose up
	COMPOSE_DOWN=docker-compose down
endif

.PHONY: test integration fedbox brutalinks

integration: test

test: compose
	podman ps
	#$(COMPOSE_DOWN)

compose:
	$(COMPOSE_UP) -d

