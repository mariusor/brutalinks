include ./.env

APPSOURCES := $(wildcard ../app/*.go ../app/*/*.go ../internal/*/*.go) ../main.go

.PHONY: imagebuilder clean images cert

$(HOSTNAME).key $(HOSTNAME).crt:
	openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=$(HOSTNAME)/OU=none/CN=$(HOSTNAME)" -newkey rsa:2048 -sha256 -keyout $(HOSTNAME).key -nodes -x509 -days 365 -out $(HOSTNAME).crt

$(HOSTNAME).pem: $(HOSTNAME).key $(HOSTNAME).crt
	cat $(HOSTNAME).key $(HOSTNAME).crt > $(HOSTNAME).pem

cert: $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem}

imagebuilder: ../go.mod $(APPSSOURCES)
	docker build -t $(HOSTNAME)/builder:$(ENV)  .. -f Dockerfile.build

build: imagebuilder
	docker run $(HOSTNAME)/builder:$(ENV)

images: imagebuilder
	docker-compose up --build
