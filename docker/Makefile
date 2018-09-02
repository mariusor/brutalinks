include ./.env

image:
	docker build --build-arg LISTEN=${LISTEN} \
		--build-arg HOSTNAME=${HOSTNAME} \
		-t mariusor/littr.go${VERSION} .

compose:
	docker-compose build
