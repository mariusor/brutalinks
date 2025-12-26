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

TAG_CMD=podman tag
PUSH_CMD=podman push

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
	HASH=$(shell git rev-parse --short HEAD)
	VERSION = $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION = $(shell git describe --tags | tr '/' '-')
endif

.PHONY: clean images cert build builder run push start

.DEFAULT_GOAL := help

$(ENV_FILE):
	touch $(ENV_FILE)

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
