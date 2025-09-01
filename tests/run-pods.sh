#!/bin/bash

set -e

TEST_PORT=${TEST_PORT:-4499}
AUTH_IMAGE=${AUTH_IMAGE:-localhost/auth/app:dev-fs}
POINT_IMAGE=${POINT_IMAGE:-localhost/point/app:dev-fs}
FEDBOX_IMAGE=${FEDBOX_IMAGE:-localhost/fedbox/app:dev-fs}
IMAGE=${IMAGE:-localhost/brutalinks/app:dev}
NETWORK=${NETWORK:-tests_network}

if [ "${NETWORK}" != "host" ]; then
    if podman network exists "${NETWORK}"; then
        podman network rm -f "${NETWORK}"
    fi

    podman network create --subnet 10.6.6.0/24 --gateway 10.6.6.1 "${NETWORK}"
fi

## FedBOX

if [ "${NETWORK}" != "host" ]; then
  FEDBOX_NETWORK="--network-alias fedbox-internal --ip 10.6.6.61 "
fi
podman run -d --replace \
    --pull newer \
    --name=tests_fedbox \
    -v "$(pwd)/mocks/fedbox/env:/.env" \
    -v "$(pwd)/mocks/fedbox:/storage" \
    -e ENV=test \
    -e STORAGE=fs \
    -e LISTEN=:8443 \
    -e HOSTNAME=fedbox \
    --net "${NETWORK}" \
    --expose 8443 \
    ${FEDBOX_NETWORK} \
    "${FEDBOX_IMAGE}" \
    --env test

_fedbox_running=$(podman ps --filter name=tests_fedbox --format '{{ .Names }}')
if [ -z "${_fedbox_running}" ]; then
    echo "Unable to run fedbox test pod: ${FEDBOX_IMAGE}"
    podman logs tests_fedbox
    exit 1
fi

## OAuth2

if [ "${NETWORK}" != "host" ]; then
  AUTH_NETWORK="--net ${NETWORK} --network-alias auth-internal --ip 10.6.6.62"
fi

podman run -d --replace \
    --pull newer \
    --name=tests_auth \
    -v "$(pwd)/mocks/fedbox:/storage" \
    --expose 8080 \
    ${AUTH_NETWORK} \
    "${AUTH_IMAGE}" \
    --env test --listen :8080 --storage fs:///storage/%storage%/%env%

_auth_running=$(podman ps --filter name=tests_auth --format '{{ .Names }}')
if [ -z "${_auth_running}" ]; then
    echo "Unable to run test fedbox OAuth2 pod: ${AUTH_IMAGE}"
    podman logs tests_auth
    exit 1
fi

## WebFinger

if [ "${NETWORK}" != "host" ]; then
  POINT_NETWORK="--net ${NETWORK} --network-alias point-internal --ip 10.6.6.63"
fi

podman run -d --replace \
    --pull newer \
    --name=tests_point \
    -v "$(pwd)/mocks/fedbox:/storage" \
    --expose 8080 \
    ${POINT_NETWORK} \
    "${POINT_IMAGE}" \
    --env test --listen :8080 --storage fs:///storage/%storage%/%env% --root https://fedbox

_point_running=$(podman ps --filter name=tests_point --format '{{ .Names }}')
if [ -z "${_point_running}" ]; then
    echo "Unable to run test fedbox WebFinger pod: ${POINT_IMAGE}"
    podman logs tests_point
    exit 1
fi

if [ "${NETWORK}" != "host" ]; then
  CADDY_NETWORK="--net ${NETWORK} --network-alias fedbox --network-alias brutalinks --network-alias auth --network-alias point --ip 10.6.6.6 --expose 443 --expose 80"
fi
podman run --replace -d \
    -p "${TEST_PORT}":443 \
    --name=tests_caddy \
    -v "$(pwd)/mocks/Caddyfile:/etc/caddy/Caddyfile" \
    -v caddy_data:/data \
    ${CADDY_NETWORK} \
    docker.io/library/caddy:2

_caddy_running=$(podman ps --filter name=tests_caddy --format '{{ .Names }}')
if [ -z "${_caddy_running}" ]; then
    echo "Unable to run test pod for Caddy"
    podman logs tests_caddy
    exit 1
fi

if [ "${NETWORK}" != "host" ]; then
  BRUTALINKS_NETWORK="--net ${NETWORK} --network-alias brutalinks-internal --add-host fedbox:10.6.6.6 --add-host auth:10.6.6.6 --ip 10.6.6.66"
fi
podman run -d --replace \
    --pull newer \
    --name=tests_brutalinks \
    -v "$(pwd)/mocks/brutalinks/env:/.env" \
    -v "$(pwd)/mocks/brutalinks:/storage" \
    -e LISTEN_HOST=brutalinks \
    --expose 8443 \
    ${BRUTALINKS_NETWORK} \
    "${IMAGE}"
sleep 1

_brutalinks_running=$(podman ps --filter name=tests_brutalinks --format '{{ .Names }}')
if [ -z "${_brutalinks_running}" ]; then
    echo "Unable to run Brutalinks test pod: ${IMAGE}"
    podman logs tests_brutalinks
    exit 1
fi
echo "Brutalinks pod running: ${IMAGE}"
sleep 2
