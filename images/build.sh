#!/usr/bin/env bash

#set -x

_workdir=${1:-../}
_image_name=${2:-brutalinks/builder}
_go_version=${GO_VERSION:-1.21}

_context=$(realpath "${_workdir}")

_builder=$(buildah from docker.io/library/golang:${_go_version})

buildah config --env GO111MODULE=on "${_builder}"
buildah config --env GOWORK=off "${_builder}"
buildah config --env GOPROXY=direct "${_builder}"

buildah copy --ignorefile "${_context}/.containerignore" --contextdir "${_context}" "${_builder}" "${_context}" /go/src/app

buildah config --workingdir /go/src/app "${_builder}"

buildah run "${_builder}" make download
buildah run "${_builder}" go mod vendor

# commit
buildah commit "${_builder}" "${_image_name}"
