#!/bin/bash

set -a
. .env || exit 1
set +a

if [[ -x "$1" ]]; then
 ./$@
else
go run $@
fi
