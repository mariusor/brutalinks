SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
pwd := $(abspath $(dir mkfile_path))

COMPOSE=podman-compose

.PHONY: up down

down:
	$(COMPOSE) down

up:
	$(COMPOSE) up -d

