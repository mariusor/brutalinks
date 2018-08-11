include .env
export
export CGO_ENABLED=0
export GOOS=linux

GO := vgo
BUILD := $(GO) build -a -ldflags '-extldflags "-static"'

littr: main.go $(wildcard ./app/*.go) $(wildcard ./api/*.go) $(wildcard ./models/*.go)
	$(BUILD) -o $@ ./main.go

bootstrap: cli/bootstrap/main.go
	$(BUILD) -o $@ cli/bootstrap/main.go

votes: cli/votes/main.go
	$(BUILD) -o $@ cli/votes/main.go

cli: bootstrap votes
	env

clean:
	$(RM) littr votes bootstrap

run: littr
	@./littr
