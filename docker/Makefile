M4 = /usr/bin/m4
M4_FLAGS =

ENV ?= dev
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

HAVE_BUILDAH := $(shell buildah -v dot 2> /dev/null)
ifdef HAVE_BUILDAH
	BUILD_CMD=buildah bud
	RUN_CMD=podman run
	ENV_CMD=buildah tag
	PUSH_CMD=buildah push
else
	BUILD_CMD=docker build
	RUN_CMD=docker run
	ENV_CMD=docker tag
	PUSH_CMD=docker push
endif

.PHONY: clean images cert build builder run push start

$(ENV_FILE):
	touch $(ENV_FILE)

$(HOSTNAME).pem:
	./gen-certs.sh $(HOSTNAME)

cert: $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem} Caddyfile

builder:
	$(BUILD_CMD) -f Dockerfile.build -t littr-builder ..

build:
	$(BUILD_CMD) \
		--build-arg HOSTNAME=$(HOSTNAME) \
		--build-arg ENV=$(ENV) \
		--build-arg PORT=$(PORT) \
		-f app/Dockerfile.$(ENV) \
		-t fedbox/littr:$(ENV) ..

run: build $(ENV_FILE)
	$(RUN) \
		-v $(ENV_FILE):/.env \
		-e API_URL=$(API_URL) \
		-p $(PORT):$(PORT) \
		-h $(HOSTNAME) \
		fedbox/littr:$(ENV)

Caddyfile: Caddyfile.in
	$(M4) -DAPP_HOSTNAME=$(HOSTNAME) -DAPP_PORT=$(PORT) \
		-DFEDBOX_HOSTNAME=$(FEDBOX_HOSTNAME) -DFEDBOX_PORT=$(FEDBOX_PORT) $< >$@

push: build
	$(ENV_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:latest
	$(ENV_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:$(ENV)
	$(PUSH_CMD) quay.io/go-ap/go-littr:latest
	$(PUSH_CMD) quay.io/go-ap/go-littr:$(ENV)
ifeq ($(ENV), dev)
	$(ENV_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:latest || true
	$(PUSH_CMD) quay.io/go-ap/go-littr:latest || true
endif
ifneq ($(VERSION), )
	$(ENV_CMD) fedbox/littr:$(ENV) quay.io/go-ap/go-littr:$(VERSION)-$(ENV) || true
	$(PUSH_CMD) quay.io/go-ap/go-littr:$(VERSION)-$(ENV) || true
endif

start: Caddyfile
	docker-compose up --detach && \
	docker-compose logs -f

images: Caddyfile
	docker-compose up --build --detach && \
	docker-compose logs -f
