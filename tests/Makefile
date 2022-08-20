SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
pwd := $(abspath $(dir mkfile_path))

.PHONY: test

test:
	./run-pods.sh
	k6 run --no-usage-report ./script.js

