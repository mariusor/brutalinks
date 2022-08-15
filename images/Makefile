M4 = /usr/bin/m4
M4_FLAGS =

ENV ?= prod
APP_HOSTNAME ?= brutalinks.local
PORT ?= 4001
FEDBOX_PORT ?= 4000
SERVER_HOSTNAME ?= fedbox.local
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
	./build.sh .. littr/builder

build:
	ENV=$(ENV) VERSION=$(VERSION) STORAGE=$(STORAGE) PORT=$(PORT) APP_HOSTNAME=$(APP_HOSTNAME) ./image.sh fedbox/littr:$(ENV)

run: build $(ENV_FILE)
	$(RUN_CMD) \
		-v $(ENV_FILE):/.env \
		-e API_URL=$(API_URL) \
		-p $(PORT):$(PORT) \
		-h $(APP_HOSTNAME) \
		fedbox/littr:$(ENV)

Caddyfile: Caddyfile.in
	$(M4) -DAPP_HOSTNAME=$(APP_HOSTNAME) -DAPP_PORT=$(PORT) \
		-DSERVER_HOSTNAME=$(SERVER_HOSTNAME) -DFEDBOX_PORT=$(FEDBOX_PORT) $< >$@

push: build
	$(TAG_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:$(ENV)
	$(PUSH_CMD) quay.io/go-ap/go-littr:$(ENV)
ifeq ($(ENV), dev)
	$(TAG_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:latest
	$(PUSH_CMD) quay.io/go-ap/go-littr:latest || true
endif
ifneq ($(VERSION), )
	$(TAG_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:$(VERSION)-$(ENV) || true
	$(PUSH_CMD) quay.io/go-ap/go-littr:$(VERSION)-$(ENV) || true
endif

start: Caddyfile
	$(COMPOSE_CMD) up --detach && \
	$(COMPOSE_CMD) logs -f

images: Caddyfile
	$(COMPOSE_CMD) up --build --detach && \
	$(COMPOSE_CMD) logs -f
