SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

ENV ?= dev
APP_HOSTNAME ?= brutalinks.local
SERVER_HOSTNAME ?= fedbox.local
PORT ?= 4001
FEDBOX_PORT ?= 4000
VERSION ?= HEAD
ENV_FILE ?= $(shell realpath .env.$(ENV))
ifeq ($(OAUTH2_CALLBACK_URL), )
	override OAUTH2_CALLBACK_URL := https://$(APP_HOSTNAME)/auth/fedbox/callback
endif
ifeq ($(API_URL), )
	override API_URL := https://$(SERVER_HOSTNAME)
endif
export API_URL := $(API_URL)
export OAUTH2_CALLBACK_URL := $(OAUTH2_CALLBACK_URL)

BUILD_CMD=buildah bud
RUN_CMD=podman run
TAG_CMD=podman tag
PUSH_CMD=podman push
COMPOSE_CMD=podman-compose

.PHONY: clean images cert build builder run push start

$(ENV_FILE):
	touch $(ENV_FILE)

brutalinks.pem:
	./gen-certs.sh brutalinks

cert: brutalinks.pem

clean:
	@-$(RM) brutalinks.{key,crt,pem} Caddyfile

builder:
	./build.sh .. brutalinks/builder

build:
	ENV=$(ENV) VERSION=$(VERSION) PORT=$(PORT) APP_HOSTNAME=$(APP_HOSTNAME) ./image.sh brutalinks/app:$(ENV)

run: build $(ENV_FILE)
	$(RUN_CMD) \
		-v $(ENV_FILE):/.env \
		-e API_URL=$(API_URL) \
		-p $(PORT):$(PORT) \
		-h $(APP_HOSTNAME) \
		brutalinks/app:$(ENV)

Caddyfile: Caddyfile.in
	$(M4) -DAPP_HOSTNAME=$(APP_HOSTNAME) -DAPP_PORT=$(PORT) \
		-DSERVER_HOSTNAME=$(SERVER_HOSTNAME) -DFEDBOX_PORT=$(FEDBOX_PORT) $< >$@

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

start: Caddyfile
	$(COMPOSE_CMD) up --detach && \
	$(COMPOSE_CMD) logs -f

images: Caddyfile
	$(COMPOSE_CMD) up --build --detach && \
	$(COMPOSE_CMD) logs -f
