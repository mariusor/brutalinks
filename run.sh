#!/bin/bash

IFS=$'\r\n'
GLOBIGNORE='*'

__env=($(<.env))
for i in ${__env[@]}; do
    name=${i%=*}
    quot=${i#*=}
    value=${quot//\"}
    export ${name}=${value}
done

go run $@
