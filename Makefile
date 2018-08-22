include .env
export
export CGO_ENABLED=0
export GOOS=linux

GO := vgo
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'

app: main.go $(wildcard ./app/*.go) $(wildcard ./api/*.go) $(wildcard ./models/*.go)
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
