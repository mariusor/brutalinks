#!/bin/bash

set -e

TEST_PORT=${TEST_PORT:-4499}

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
    --pull always \
    --name=tests_fedbox \
    -v $(pwd)/fedbox/env:/.env \
    -v $(pwd)/fedbox:/storage \
    -e ENV=test \
    -e LISTEN=:443 \
    -e HOSTNAME=fedbox \
    --net tests_default \
    --network-alias fedbox \
    --expose 443 \
    quay.io/go-ap/fedbox:qa-fs \
    /bin/fedbox

podman run -d \
    --pull always \
    --name=tests_brutalinks \
    -v $(pwd)/brutalinks/env:/.env \
    -v $(pwd)/brutalinks:/storage \
    -e LISTEN_HOST= \
    --net tests_default \
    --network-alias brutalinks \
    --expose 443 \
    -p "${TEST_PORT}:443" \
    quay.io/go-ap/go-littr:qa \
    /bin/brutalinks
