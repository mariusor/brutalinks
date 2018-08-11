#!/bin/bash

IFS=$'\r\n'
GLOBIGNORE='*'

__env=($(<.env))
for i in ${__env[@]}; do
    name=${i%=*}
    quot=${i#*=}
    value=${quot//\"}
    if [[ -n "${name}" && -n "${value}" ]]; then
        export ${name}=${value}
    fi
done

vgo run $@
