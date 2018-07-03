#!/bin/bash

export GOPATH=$(pwd):$GOPATH
. .env

go run ./app/*.go
