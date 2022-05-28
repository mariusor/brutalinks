export CGO_ENABLED=0
GO := go
TEST_FLAGS ?= -count=1 -v
STORAGE ?= all

ENV ?= test
BUILDFLAGS ?= -a
TAGS ?= prod
TEST_TARGET = .

TEST := $(GO) test $(BUILDFLAGS)

.PHONY: test integration download

integration: test

../internal/assets/assets.gen.go:
	error(failure)

test: ../internal/assets/assets.gen.go download
	$(TEST) $(TEST_FLAGS) -tags "$(TAGS)" $(TEST_TARGET)

download:
	$(GO) mod download all

