include ./.env

$(HOSTNAME).pem:
	openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=$(HOSTNAME)/OU=none/CN=$(HOSTNAME)" -newkey rsa:2048 -sha256 -keyout $(HOSTNAME).key -nodes -x509 -days 365 -out $(HOSTNAME).crt
	cat $(HOSTNAME).key $(HOSTNAME).crt > $(HOSTNAME).pem

clean:
	$(RM) -rf ./{bin,templates,assets}
	$(RM) -rf ./{app,bootstrap}/bin
	$(RM) -rf app/{templates,assets}
	$(RM) -rf bootstrap/db
	$(RM) $(HOSTNAME).{key,crt,pem}

cert: $(HOSTNAME).pem

.PHONY: imagebuilder
imagebuilder:
	docker build -t littr/builder:$(ENV) .. -f Dockerfile.build

images: imagebuilder
	docker-compose up --build
