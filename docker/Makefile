M4 = /usr/bin/m4
M4_FLAGS =

ENV ?= prod
HOSTNAME ?= brutalinks.local
PORT ?= 4001
FEDBOX_PORT ?= 4000
FEDBOX_HOSTNAME ?= fedbox.local
ENV_FILE ?= $(shell realpath .env.$(ENV))
ifeq ($(OAUTH2_CALLBACK_URL), )
	override OAUTH2_CALLBACK_URL := https://$(HOSTNAME)/auth/fedbox/callback
endif
ifeq ($(API_URL), )
	override API_URL := https://$(FEDBOX_HOSTNAME)
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

$(HOSTNAME).pem:
	./gen-certs.sh $(HOSTNAME)

cert: $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem} Caddyfile

builder:
	$(BUILD_CMD) -f Dockerfile.build -t littr/builder ..

build:
	$(BUILD_CMD) \
		--build-arg HOSTNAME=$(HOSTNAME) \
		--build-arg ENV=$(ENV) \
		--build-arg PORT=$(PORT) \
		-f app/Dockerfile.$(ENV) \
		-t fedbox/littr:$(ENV) ..

run: build $(ENV_FILE)
	$(RUN_CMD) \
		-v $(ENV_FILE):/.env \
		-e API_URL=$(API_URL) \
		-p $(PORT):$(PORT) \
		-h $(HOSTNAME) \
		fedbox/littr:$(ENV)

Caddyfile: Caddyfile.in
	$(M4) -DAPP_HOSTNAME=$(HOSTNAME) -DAPP_PORT=$(PORT) \
		-DFEDBOX_HOSTNAME=$(FEDBOX_HOSTNAME) -DFEDBOX_PORT=$(FEDBOX_PORT) $< >$@

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
