include .env
export
export CGO_ENABLED=0
export GOOS=linux

GO := go
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'
SOURCES := $(wildcard ./app/*.go) $(wildcard ./api/*.go) $(wildcard ./models/*.go)

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
