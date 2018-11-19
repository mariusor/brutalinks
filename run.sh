#!/bin/bash

set -a
. .env
set +a

if [[ -x "$1" ]]; then
 ./$@
else
go run $@
fi
