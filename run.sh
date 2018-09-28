#!/bin/bash

set -a
. .env
set +a

go run $@
