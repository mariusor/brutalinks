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
GO_VERSION ?= 1.23

TAG_CMD=podman tag
PUSH_CMD=podman push

.PHONY: clean images cert build builder run push start

.DEFAULT_GOAL := help

help: ## Help target that shows this message.
	@sed -rn 's/^([^:]+):.*[ ]##[ ](.+)/\1:\2/p' $(MAKEFILE_LIST) | column -ts: -l2

$(ENV_FILE):
	touch $(ENV_FILE)

brutalinks.pem:
	./gen-certs.sh brutalinks

cert: brutalinks.pem ## Builds the certificate to include in the image.

clean: ## Clean the workspace.
	@-$(RM) brutalinks.{key,crt,pem}

builder: ## Build the builder image for compiling the Brutalinks application.
	./build.sh .. brutalinks/builder

build: ## Build the Brutalinks application.
	ENV="$(ENV)" VERSION="$(VERSION)" PORT="$(PORT)" APP_HOSTNAME="$(APP_HOSTNAME)" ./image.sh "brutalinks/app:$(ENV)"

push: build ## Push the Brutalinks application images to Quay.io.
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
