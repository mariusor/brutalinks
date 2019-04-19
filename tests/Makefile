export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: test integration clean

clean:
	@-$(RM) -rf ./db

test:
	@-cp -r ../db ./
	@$(MAKE) integration
	@-$(MAKE) clean

integration:
	$(TEST) ./...
