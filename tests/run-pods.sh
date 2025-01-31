#!/bin/bash

set -e

ENV=${ENV:-dev}
TEST_PORT=${TEST_PORT:-4499}
AUTH_IMAGE=${AUTH_IMAGE:-localhost/auth/app:dev}
FEDBOX_IMAGE=${FEDBOX_IMAGE:-localhost/fedbox/app:dev}
IMAGE=${IMAGE:-localhost/brutalinks/app:${ENV}}
NETWORK=${NETWORK:-tests_network}

if [ "${NETWORK}" != "host" ]; then
    if podman network exists ${NETWORK}; then
        podman network rm -f ${NETWORK}
    fi

    podman network create --subnet 10.6.6.0/24 --gateway 10.6.6.1 "${NETWORK}"
fi

podman run -d --replace \
    --pull newer \
    --name=tests_fedbox \
    -v $(pwd)/mocks/fedbox/env:/.env \
    -v $(pwd)/mocks/fedbox:/storage \
    -e ENV=test \
    -e STORAGE=fs \
    -e LISTEN=:8443 \
    -e HOSTNAME=fedbox \
    --net "${NETWORK}" \
    --network-alias fedbox-internal \
    --ip 10.6.6.61 \
    --expose 8443 \
    ${FEDBOX_IMAGE}

_fedbox_running=$(podman ps --filter name=tests_fedbox --format '{{ .Names }}')
if [ -z "${_fedbox_running}" ]; then
    echo "Unable to run fedbox test pod: ${FEDBOX_IMAGE}"
    exit 1
fi

podman run -d --replace \
    --pull newer \
    --name=tests_auth \
    -v $(pwd)/mocks/fedbox:/storage \
    --net "${NETWORK}" \
    --network-alias auth-internal \
    --ip 10.6.6.62 \
    --expose 8080 \
    ${AUTH_IMAGE} \
    --env test --listen :8080 --storage fs:///storage/%storage%/%env%

_auth_running=$(podman ps --filter name=tests_auth --format '{{ .Names }}')
if [ -z "${_auth_running}" ]; then
    echo "Unable to run test fedbox OAuth2 pod: ${AUTH_IMAGE}"
    exit 1
fi

podman run --replace -d \
    -p ${TEST_PORT}:443 \
    --name=tests_caddy \
    -v $(pwd)/mocks/Caddyfile:/etc/caddy/Caddyfile \
    -v caddy_data:/data \
    --net "${NETWORK}" \
    --network-alias fedbox \
    --network-alias brutalinks \
    --network-alias auth \
    --ip 10.6.6.6 \
    --expose 443 \
    --expose 80 \
    docker.io/library/caddy:2.7

_caddy_running=$(podman ps --filter name=tests_caddy --format '{{ .Names }}')
if [ -z "${_caddy_running}" ]; then
    echo "Unable to run test pod for Caddy"
    exit 1
fi

podman run -d --replace \
    --pull newer \
    --name=tests_brutalinks \
    -v $(pwd)/mocks/brutalinks/env:/.env \
    -v $(pwd)/mocks/brutalinks:/storage \
    -e LISTEN_HOST=brutalinks \
    --net "${NETWORK}" \
    --network-alias brutalinks-internal \
    --add-host fedbox:10.6.6.6 \
    --add-host auth:10.6.6.6 \
    --ip 10.6.6.66 \
    --expose 8443 \
    "${IMAGE}"
sleep 1

_brutalinks_running=$(podman ps --filter name=tests_brutalinks --format '{{ .Names }}')
if [ -z "${_brutalinks_running}" ]; then
    echo "Unable to run Brutalinks test pod: ${IMAGE}"
    exit 1
fi
echo "Brutalinks pod running: ${IMAGE}"
sleep 2

