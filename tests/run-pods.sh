#!/bin/bash

set -e

ENV=${ENV:-dev}
TEST_PORT=${TEST_PORT:-4499}
AUTH_IMAGE=${AUTH_IMAGE:-quay.io/go-ap/auth:qa}
FEDBOX_IMAGE=${FEDBOX_IMAGE:-quay.io/go-ap/fedbox:qa-fs}
IMAGE=${IMAGE:-localhost/brutalinks/app:${ENV}}

if podman network exists tests_network; then
    podman network rm -f tests_network
fi

podman network create --subnet 10.6.6.0/24 --gateway 10.6.6.1 tests_network

podman run -d --replace \
    --pull newer \
    --name=tests_fedbox \
    -v $(pwd)/mocks/fedbox/env:/.env \
    -v $(pwd)/mocks/fedbox:/storage \
    -e ENV=test \
    -e STORAGE=fs \
    -e LISTEN=:8443 \
    -e HOSTNAME=fedbox \
    --net tests_network \
    --network-alias fedbox-internal \
    --ip 10.6.6.61 \
    --expose 8443 \
    ${FEDBOX_IMAGE}

_fedbox_running=$(podman ps --filter name=tests_fedbox --format '{{ .Names }}' )
if [ -s ${_fedbox_running} ]; then
    echo "Unable to run fedbox test pod: ${FEDBOX_IMAGE}"
    exit 1
fi

podman run -d --replace \
    --pull newer \
    --name=tests_auth \
    -v $(pwd)/mocks/fedbox:/storage \
    --net tests_network \
    --ip 10.6.6.62 \
    --network-alias auth-internal \
    --expose 8080 \
    ${AUTH_IMAGE} \
    --env test --listen :8080 --storage fs:///storage/%storage%/%env%

_auth_running=$(podman ps --filter name=tests_auth --format '{{ .Names }}' )
if [ -s ${_auth_running} ]; then
    echo "Unable to run test fedbox OAuth2 pod: ${AUTH_IMAGE}"
    exit 1
fi

podman run --replace -d \
    -p ${TEST_PORT}:443 \
    --name=tests_caddy \
    -v $(pwd)/mocks/Caddyfile:/etc/caddy/Caddyfile \
    -v caddy_data:/data \
    --net tests_network \
    --network-alias fedbox \
    --network-alias brutalinks \
    --network-alias auth \
    --ip 10.6.6.6 \
    --expose 443 \
    --expose 80 \
    docker.io/library/caddy:2.7

_caddy_running=$(podman ps --filter name=tests_caddy --format '{{ .Names }}' )
if [ -s ${_caddy_running} ]; then
    echo "Unable to run test pod for Caddy"
    exit 1
fi

podman run -d --replace \
    --pull newer \
    --name=tests_brutalinks \
    -v $(pwd)/mocks/brutalinks/env:/.env \
    -v $(pwd)/mocks/brutalinks:/storage \
    -e LISTEN_HOST=brutalinks \
    --net tests_network \
    --add-host fedbox:10.6.6.6 \
    --add-host auth:10.6.6.6 \
    --network-alias brutalinks-internal \
    --ip 10.6.6.66 \
    --expose 8443 \
    "${IMAGE}"

_brutalinks_running=$(podman ps --filter name=tests_brutalinks --format '{{ .Names }}' )
if [ -s ${_brutalinks_running} ]; then
    echo "Unable to run brutalinks test pod: ${IMAGE}"
    exit 1
fi
echo "Brutalinks pod running: ${IMAGE}"

