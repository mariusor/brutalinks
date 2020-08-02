include ./.env

APPSOURCES := $(wildcard ../app/*.go ../cli/*/*.go ../internal/*/*.go)

.PHONY: imagebuilder clean images cert

#BUILD=buildah build-using-dockerfile
#RUN=buildah run
#TAG=buildah tag
#PUSH=buildah push
BUILD=docker build
RUN=docker run
TAG=docker tag
PUSH=docker push

$(HOSTNAME).key $(HOSTNAME).crt:
	openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=$(HOSTNAME)/OU=none/CN=$(HOSTNAME)" -newkey rsa:2048 -sha256 -keyout $(HOSTNAME).key -nodes -x509 -days 365 -out $(HOSTNAME).crt

$(HOSTNAME).pem: $(HOSTNAME).key $(HOSTNAME).crt
	cat $(HOSTNAME).key $(HOSTNAME).crt > $(HOSTNAME).pem

cert: $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem}

build:
	$(BUILD) --build-arg ENV=$(ENV) --build-arg LISTEN=$(PORT) -f app/Dockerfile.$(ENV) -t $(HOSTNAME)/app:$(ENV) ..

run: build
	$(RUN) -e ENV=$(ENV) -e HOSTNAME=$(HOSTNAME) -p $(PORT):$(PORT) $(HOSTNAME)/app:$(ENV)

push: build
	$(TAG) $(HOSTNAME)/app:$(ENV) quay.io/fedbox/littr:latest
	$(TAG) $(HOSTNAME)/app:$(ENV) quay.io/fedbox/littr:$(ENV)
	$(PUSH) quay.io/fedbox/littr:latest
	$(PUSH) quay.io/fedbox/littr:$(ENV)

images: push
	docker-compose up --build
