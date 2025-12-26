#!/usr/bin/env bash

#set -ex
set -e
_context=$(realpath "../")

_environment=${ENV:-dev}
_hostname=${APP_HOSTNAME:-brutalinks}
_listen_port=${PORT:-3003}
_version=${VERSION:-HEAD}

_image_name=${1:-brutalinks:${_environment}}

echo "Building image ${_image_name} for host:${_hostname} env:${_environment} port:${_listen_port} version:${_version}"

HOST_GOCACHE=$(go env GOCACHE)
HOST_GOMODCACHE=$(go env GOMODCACHE)

GOCACHE=/root/.cache/go-build
GOMODCACHE=/go/pkg/mod

_builder=$(buildah from docker.io/library/golang:1.25-alpine)

buildah run "${_builder}" /sbin/apk update
buildah run "${_builder}" /sbin/apk add make bash openssl upx

buildah config --env GO111MODULE=on "${_builder}"
buildah config --env GOWORK=off "${_builder}"
buildah config --env "GOCACHE=${GOCACHE}" "${_builder}"
buildah config --env "GOMODCACHE=${GOMODCACHE}" "${_builder}"

buildah config --workingdir /go/src/app "${_builder}"

buildah run \
    --mount="type=bind,rw,source=${HOST_GOCACHE},destination=${GOCACHE}" \
    --mount="type=bind,rw,source=${HOST_GOMODCACHE},destination=${GOMODCACHE}" \
    --mount="type=bind,rw,source=${_context},destination=/go/src/app" \
    --mount=type=cache,rw,id=bin,target=/go/src/app/bin "${_builder}" \
    make ENV="${_environment}" VERSION="${_version}" APP_HOSTNAME="${_hostname}" clean all cert

if [ "${_environment}" = "dev" ]; then
    buildah run \
        --mount="type=bind,rw,source=${_context},destination=/go/src/app" \
        --mount=type=cache,rw,id=bin,target=/go/src/app/bin \
        "${_builder}" \
        cp -r /go/src/app/templates /go/src/app/bin
    buildah run \
        --mount="type=bind,rw,source=${_context},destination=/go/src/app" \
        --mount=type=cache,rw,id=bin,target=/go/src/app/bin \
        "${_builder}" \
        cp -r /go/src/app/assets /go/src/app/bin
    buildah run \
        --mount="type=bind,rw,source=${_context},destination=/go/src/app" \
        --mount=type=cache,rw,id=bin,target=/go/src/app/bin \
        "${_builder}" \
        cp -r /go/src/app/README.md /go/src/app/bin
fi

# copy binaries from cache to builder container fs
buildah run \
    --mount=type=cache,rw,id=bin,target=/tmp/bin "${_builder}" \
    cp -r /tmp/bin /go/src/

_image=$(buildah from gcr.io/distroless/static:latest)

buildah config --env "ENV=${_environment}" "${_image}"
buildah config --env "APP_HOSTNAME=${_hostname}" "${_image}"
buildah config --env "LISTEN=:${_listen_port}" "${_image}"
buildah config --env "KEY_PATH=/etc/ssl/certs/${_hostname}.key" "${_image}"
buildah config --env "CERT_PATH=/etc/ssl/certs/${_hostname}.crt" "${_image}"
buildah config --env HTTPS=true "${_image}"

buildah config --port "${_listen_port}" "${_image}"

buildah config --volume /storage "${_image}"
buildah config --volume /.env "${_image}"

buildah copy --from "${_builder}" "${_image}" /go/src/bin/brutalinks /bin/
buildah copy --from "${_builder}" "${_image}" "/go/src/bin/${_hostname}.key" /etc/ssl/certs/
buildah copy --from "${_builder}" "${_image}" "/go/src/bin/${_hostname}.crt" /etc/ssl/certs/
buildah copy --from "${_builder}" "${_image}" "/go/src/bin/${_hostname}.pem" /etc/ssl/certs/

if [ "${_environment}" = "dev" ]; then
  buildah copy --from "${_builder}" "${_image}" /go/src/bin/templates /templates
  buildah copy --from "${_builder}" "${_image}" /go/src/bin/assets /assets
  buildah copy --from "${_builder}" "${_image}" /go/src/bin/README.md /README.md

  buildah config --workingdir / "${_image}"
fi

buildah config --entrypoint '["/bin/brutalinks"]' "${_image}"

# commit
buildah commit "${_image}" "${_image_name}"
