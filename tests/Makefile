export CGO_ENABLED=0
GO := go
TEST_FLAGS ?= -count=1 -v
STORAGE ?= all

ENV ?= test
BUILDFLAGS ?= -a
TAGS =
TEST_TARGET = ./main_test.go

TEST := $(GO) test $(BUILDFLAGS)

.PHONY: test integration

download:
	$(GO) mod download all

test: download
	$(TEST) $(TEST_FLAGS) -tags "$(TAGS)" $(TEST_TARGET)

integration: test
