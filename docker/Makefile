M4 = /usr/bin/m4
M4_FLAGS =

APPSOURCES := $(wildcard ../app/*.go ../cli/*/*.go ../internal/*/*.go)

ENV ?= dev
PORT ?= 4001
HOSTNAME ?= brutalinks.local
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

.PHONY: clean images cert build run push start

HAVE_BUILDAH := $(shell buildah -v dot 2> /dev/null)
ifdef HAVE_BUILDAH
	BUILD=buildah build-using-dockerfile
	RUN=buildah run
	TAG=buildah tag
	PUSH=buildah push
else
	BUILD=docker build
	RUN=docker run
	TAG=docker tag
	PUSH=docker push
endif

$(HOSTNAME).pem:
	gen-certs.sh $(HOSTNAME)

cert: $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem} Caddyfile

build: $(APPSOURCES)
	$(BUILD) \
		--build-arg HOSTNAME=$(HOSTNAME) \
		--build-arg ENV=$(ENV) \
		--build-arg PORT=$(PORT) \
		-f app/Dockerfile.$(ENV) \
		-t fedbox/littr:$(ENV) ..

$(ENV_FILE):
	touch $(ENV_FILE)

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
	$(TAG) fedbox/littr:$(ENV) quay.io/fedbox/littr:latest
	$(TAG) fedbox/littr:$(ENV) quay.io/fedbox/littr:$(ENV)
	$(PUSH) quay.io/fedbox/littr:latest
	$(PUSH) quay.io/fedbox/littr:$(ENV)

start: Caddyfile
	docker-compose up --detach && \
	docker-compose logs -f

images: Caddyfile
	docker-compose up --build --detach && \
	docker-compose logs -f
