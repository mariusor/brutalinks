#!/bin/bash

set -e

TEST_PORT=${TEST_PORT:-4499}
IMAGE=${IMAGE:-localhost/brutalinks/app:qa}

if podman network exists tests_network; then
    podman network rm -f tests_network
fi

podman network create tests_network

podman run -d --replace \
    --pull newer \
    --name=tests_fedbox \
    -v $(pwd)/fedbox/env:/.env \
    -v $(pwd)/fedbox:/storage \
    -e ENV=test \
    -e LISTEN=:8443 \
    -e HOSTNAME=fedbox \
    --net tests_network \
    --network-alias fedbox_pub \
    --expose 8443 \
    quay.io/go-ap/fedbox:qa-fs \
    /bin/fedbox

_fedbox_running=$(podman ps --filter name=tests_fedbox --format '{{ .Names }}' )
if [ -s ${_fedbox_running} ]; then
    echo "Unable to run test pod for fedbox"
    exit 1
fi

podman run -d --replace \
    --pull newer \
    --name=tests_auth \
    -v $(pwd)/fedbox:/storage \
    --net tests_network \
    --network-alias auth \
    --expose 8443 \
    quay.io/go-ap/auth:qa \
    --env test --listen :8443 --storage fs:///storage/%storage%/%env%

_auth_running=$(podman ps --filter name=tests_auth --format '{{ .Names }}' )
if [ -s ${_auth_running} ]; then
    echo "Unable to run test pod for fedbox OAuth2"
    exit 1
fi

podman run --replace -d \
    -p ${TEST_PORT}:443 \
    --name=tests_caddy \
    -v $(pwd)/Caddyfile:/etc/caddy/Caddyfile \
    -v caddy_data:/data \
    --expose 443 \
    caddy
_caddy_running=$(podman ps --filter name=tests_caddy --format '{{ .Names }}' )
if [ -s ${_caddy_running} ]; then
    echo "Unable to run test pod for Caddy"
    exit 1
fi

set -x
podman run -d --replace \
    --pull newer \
    --name=tests_brutalinks \
    -v $(pwd)/brutalinks/env:/.env \
    -v $(pwd)/brutalinks:/storage \
    -e LISTEN_HOST= \
    --net tests_network \
    --network-alias brutalinks \
    --expose 8443 \
    "${IMAGE}" \
    /bin/brutalinks

_brutalinks_running=$(podman ps --filter name=tests_brutalinks --format '{{ .Names }}' )
if [ -s ${_brutalinks_running} ]; then
    echo "Unable to run test pod for brutalinks"
    exit 1
fi
