#!/bin/bash

set -e

TEST_PORT=${TEST_PORT:-4499}
IMAGE=${IMAGE:-quay.io/go-ap/go-littr:qa}

if podman container exists tests_fedbox; then
    podman stop tests_fedbox
    podman rm tests_fedbox
fi
if podman container exists tests_brutalinks; then
    podman stop tests_brutalinks
    podman rm tests_brutalinks
fi

if podman network exists tests_network; then
    podman network rm tests_network
fi

podman network create tests_network

podman run -d \
    --pull newer \
    --name=tests_fedbox \
    -v $(pwd)/fedbox/env:/.env \
    -v $(pwd)/fedbox:/storage \
    -e ENV=test \
    -e LISTEN=:443 \
    -e HOSTNAME=fedbox \
    --net tests_network \
    --network-alias fedbox \
    --expose 443 \
    quay.io/go-ap/fedbox:qa-fs \
    /bin/fedbox

_fedbox_running=$(podman ps --filter name=tests_fedbox --format '{{ .Names }}' )
if [ -s ${_fedbox_running} ]; then
    echo "Unable to run test pod for fedbox"
    exit 1
fi

podman run -d \
    --pull newer \
    --name=tests_brutalinks \
    -v $(pwd)/brutalinks/env:/.env \
    -v $(pwd)/brutalinks:/storage \
    -e LISTEN_HOST= \
    --net tests_network \
    --network-alias brutalinks \
    --expose 443 \
    -p "${TEST_PORT}:443" \
    "${IMAGE}" \
    /bin/brutalinks

_brutalinks_running=$(podman ps --filter name=tests_brutalinks --format '{{ .Names }}' )
if [ -s ${_brutalinks_running} ]; then
    echo "Unable to run test pod for brutalinks"
    exit 1
fi
