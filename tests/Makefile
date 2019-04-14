export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: test run integration killapp clean

clean:
	@-$(RM) -f ./db

test:
	@cp -r ../db ./
	@$(MAKE) integration
	@-$(MAKE) killapp
	@-$(MAKE) clean

integration: run
	$(TEST) ./...

killapp:
	@-pkill -f '../bin/app -env test' &> /dev/null
	@-([ $$? -eq 0 ]) || true

run: killapp
	@../bin/app -env test &> /dev/null &
