SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

TEST_HOST ?= brutalinks
TEST_PORT ?= 4499

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
pwd := $(abspath $(dir mkfile_path))

.PHONY: test

pods:
	TEST_PORT=$(TEST_PORT) ./run-pods.sh

test:
	k6 run --no-usage-report --insecure-skip-tls-verify \
		--env TEST_HOST=https://$(TEST_HOST):$(TEST_PORT) ./script.js

test-podman:
	podman run --replace \
		--interactive \
		--pull newer \
		--rm \
		--name=tests_k6 \
		--net tests_network \
		--network-alias k6 \
		--add-host fedbox:10.6.6.6 \
		--add-host brutalinks:10.6.6.6 \
		grafana/k6 \
		--no-usage-report --insecure-skip-tls-verify \
		--env TEST_HOST=https://brutalinks run - <script.js
