#!/usr/bin/env bash

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")

rkt --debug --insecure-options=image \
    run \
    littr-me-postgres-${__version}-linux-amd64.aci --volume=data,kind=host,source=/tmp/data \
    littr-me-nginx-${__version}-linux-amd64.aci --net=host \
    littr-me-app-${__version}-linux-amd64.aci
