SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

ENV ?= dev
APP_HOSTNAME ?= brutalinks.local
PORT ?= 4001
VERSION ?= HEAD
ENV_FILE ?= $(shell realpath .env.$(ENV))
GO_VERSION ?= 1.21

TAG_CMD=podman tag
PUSH_CMD=podman push

.PHONY: clean images cert build builder run push start

$(ENV_FILE):
	touch $(ENV_FILE)

brutalinks.pem:
	./gen-certs.sh brutalinks

cert: brutalinks.pem

clean:
	@-$(RM) brutalinks.{key,crt,pem}

builder:
	./build.sh .. brutalinks/builder

build:
	ENV="$(ENV)" VERSION="$(VERSION)" PORT="$(PORT)" APP_HOSTNAME="$(APP_HOSTNAME)" ./image.sh "brutalinks/app:$(ENV)"

push: build
	$(TAG_CMD) brutalinks/app:$(ENV) quay.io/go-ap/brutalinks:$(ENV)
	$(PUSH_CMD) quay.io/go-ap/brutalinks:$(ENV)
ifeq ($(ENV),dev)
	$(TAG_CMD) brutalinks/app:$(ENV) quay.io/go-ap/brutalinks:latest
	$(PUSH_CMD) quay.io/go-ap/brutalinks:latest || true
endif
ifneq ($(VERSION),)
	$(TAG_CMD) brutalinks/app:$(ENV) quay.io/go-ap/brutalinks:$(VERSION)-$(ENV) || true
	$(PUSH_CMD) quay.io/go-ap/brutalinks:$(VERSION)-$(ENV) || true
endif
