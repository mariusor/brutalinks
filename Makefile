include .env
export
export CGO_ENABLED=0
export GOOS=linux

GO := go
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'
SOURCES := $(wildcard ./app/*/*.go)

all: app cli

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	VERSION := :$(shell git describe --always --dirty=-git)
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION := :$(shell git describe --tags)
endif


app: main.go $(SOURCES)
	$(BUILD) -o bin/$@ ./main.go

bootstrap: cli/bootstrap/main.go
	$(BUILD) -o bin/$@ cli/bootstrap/main.go

votes: cli/votes/main.go
	$(BUILD) -o bin/$@ cli/votes/main.go

cli: bootstrap votes

clean:
	$(RM) bin/*

run: app
	@./bin/app

image:
include .env.docker
image:
	docker build --build-arg LISTEN=${LISTEN} \
		--build-arg HOSTNAME=${HOSTNAME} \
		-t mariusor/littr.go${VERSION} .

compose:
	cd docker && $(MAKE) compose
