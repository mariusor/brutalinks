export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests run integration killapp

tests:
	@cp -r ../db ./
	@$(MAKE) integration
	@$(RM) -rf ./db
	@$(RM) -rf ./app

integration: run
	$(TEST) ./...

killapp:
	@-pkill -f "./bin/app -env test" &> /dev/null
	@-([ $$? -eq 0 ]) || true

run: killapp
	@cp -rf ../bin/app ./
	@./app -env test &> /dev/null &
