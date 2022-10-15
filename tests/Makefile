SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

TEST_HOST ?= 127.0.0.1
TEST_PORT ?= 4499

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
pwd := $(abspath $(dir mkfile_path))

.PHONY: test

pods:
	TEST_PORT=$(TEST_PORT) ./run-pods.sh

test:
	k6 run --no-usage-report --insecure-skip-tls-verify \
		--env TEST_HOST=https://$(TEST_HOST):$(TEST_PORT) ./script.js