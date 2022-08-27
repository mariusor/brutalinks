#!/usr/bin/env bash

#set -x

_workdir=${1:-../}
_image_name=${2:-brutalinks/builder}

_context=$(realpath "${_workdir}")

#FROM golang:1.19
_builder=$(buildah from docker.io/library/golang:1.19)

#ENV GO111MODULE=on
buildah config --env GO111MODULE=on "${_builder}"
#ENV GOWORK=off
buildah config --env GOWORK=off "${_builder}"

#ADD ./ /go/src/app
#_context=$(realpath ../)
buildah copy --ignorefile "${_context}/.containerignore" --contextdir "${_context}" "${_builder}" "${_context}" /go/src/app

#WORKDIR /go/src/app
buildah config --workingdir /go/src/app "${_builder}"

#RUN make download && go mod vendor
buildah run "${_builder}" make download
buildah run "${_builder}" go mod vendor

# commit
buildah commit "${_builder}" "${_image_name}"
