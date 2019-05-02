include ./.env

APPSOURCES := $(wildcard ../app/*.go ../app/*/*.go ../internal/*/*.go) ../main.go

$(HOSTNAME).pem: $(HOSTNAME).key $(HOSTNAME).crt
	openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=$(HOSTNAME)/OU=none/CN=$(HOSTNAME)" -newkey rsa:2048 -sha256 -keyout $(HOSTNAME).key -nodes -x509 -days 365 -out $(HOSTNAME).crt
	cat $(HOSTNAME).key $(HOSTNAME).crt > $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem}

cert: $(HOSTNAME).pem

.PHONY: imagebuilder
imagebuilder: ../go.mod $(APPSSOURCES)
	docker build --force-rm --build-arg ENV=prod -t $(HOSTNAME)/builder:$(ENV)  .. -f Dockerfile.build

images: imagebuilder
	docker-compose up --build
