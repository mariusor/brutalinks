include .env.tests
export
export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests run integration killapp

tests:
	@cp -r ../db ./
	@$(MAKE) integration
	@$(RM) -rf ./db

integration: run
	$(TEST) ./...

killapp:
	@-pkill -f "../bin/app -host $(HOSTNAME)" &> /dev/null
	@([ $$? -eq 0 ]) || true

run: killapp
	@../bin/app -host $(HOSTNAME) -port $(subst :,,$(LISTEN)) &> /dev/null &
