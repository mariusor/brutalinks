include ./.env

$(HOSTNAME).pem:
	openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=$(HOSTNAME)/OU=none/CN=$(HOSTNAME)" -newkey rsa:2048 -sha256 -keyout $(HOSTNAME).key -nodes -x509 -days 365 -out $(HOSTNAME).crt
	cat $(HOSTNAME).key $(HOSTNAME).crt > $(HOSTNAME).pem

clean:
	@-$(RM) $(HOSTNAME).{key,crt,pem}

cert: $(HOSTNAME).pem

.PHONY: imagebuilder
imagebuilder:
	docker build -t $(HOSTNAME)/builder:$(ENV)  .. -f Dockerfile.build
#	docker run -v $(PWD):/go/src/app $(HOSTNAME)/builder:$(ENV) "make all"

images: imagebuilder
	docker-compose up --build
