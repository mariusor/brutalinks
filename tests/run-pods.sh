#!/bin/bash

set -e

if podman container exists tests_fedbox; then
    podman stop tests_fedbox
    podman rm tests_fedbox
fi
if podman container exists tests_brutalinks; then
    podman stop tests_brutalinks
    podman rm tests_brutalinks
fi

if podman network exists tests_default; then
    podman network rm tests_default
fi

podman network create tests_default

podman run -d \
    --name=tests_fedbox \
    -v $(pwd)/fedbox/env:/.env \
    -v $(pwd)/fedbox:/storage \
    -e ENV=test \
    --net tests_default \
    --network-alias fedbox \
    --expose 443 \
    -p 4000:443 \
    quay.io/go-ap/fedbox:qa-fs \
    /bin/fedbox

podman run -d \
    --name=tests_brutalinks \
    -v $(pwd)/brutalinks/env:/.env \
    -v $(pwd)/brutalinks:/storage \
    -e LISTEN_HOST= \
    --net tests_default \
    --network-alias brutalinks \
    --expose 443 \
    -p 4001:443 \
    quay.io/go-ap/go-littr:qa \
    /bin/brutalinks
