#!/usr/bin/env bash

#set -x

_environment=${ENV:-dev}
_hostname=${APP_HOSTNAME:-brutalinks}
_listen_port=${PORT:-3003}
_storage=${STORAGE:-all}
_version=${VERSION}

_image_name=${1:-fedbox:${_environment}-${_storage}}
_build_name=${2:-localhost/littr/builder}

#FROM littr/builder AS builder
_builder=$(buildah from ${_build_name}:latest)
if [[ -z ${_builder} ]]; then
    echo "Unable to find builder image: ${_build_name}"
    exit 1
fi

echo "Building image ${_image_name} for host=${_hostname} env:${_environment} port:${_listen_port}"

#ARG ENV
#ARG APP_HOSTNAME

#ENV GO111MODULE=on
#ENV ENV=${ENV:-dev}

#RUN make all && \
#    docker/gen-certs.sh app
buildah run ${_builder} make all
buildah run ${_builder} ./images/gen-certs.sh brutalinks

#FROM gcr.io/distroless/static
_image=$(buildah from gcr.io/distroless/static:latest)

#ARG APP_HOSTNAME
#ARG PORT

#ENV ENV=${ENV:-dev}
buildah config --env ENV=${_environment} ${_image}
#ENV LISTEN_HOSTNAME ${HOSTNAME:-brutalinks}
buildah config --env APP_HOSTNAME=${_hostname} ${_image}
#ENV LISTEN_PORT ${PORT:-3003}
buildah config --env LISTEN=:${_listen_port} ${_image}
#ENV KEY_PATH=/etc/ssl/certs/app.key
buildah config --env KEY_PATH=/etc/ssl/certs/brutalinks.key ${_image}
#ENV CERT_PATH=/etc/ssl/certs/app.crt
buildah config --env CERT_PATH=/etc/ssl/certs/brutalinks.crt ${_image}
#ENV HTTPS=true
buildah config --env HTTPS=true ${_image}

#EXPOSE $LISTEN_PORT
buildah config --port ${_listen_port} ${_image}

#VOLUME /storage
buildah config --volume /storage ${_image}
#VOLUME /.env
buildah config --volume /.env ${_image}

#COPY --from=builder /go/src/app/bin/brutalinks /bin/brutalinks
buildah copy --from ${_builder} ${_image} /go/src/app/bin/* /bin/
#COPY --from=builder /go/src/app/*.key /go/src/app/*.crt /go/src/app/*.pem /etc/ssl/certs/
buildah copy --from ${_builder} ${_image} /go/src/app/brutalinks.key /etc/ssl/certs/
buildah copy --from ${_builder} ${_image} /go/src/app/brutalinks.crt /etc/ssl/certs/
buildah copy --from ${_builder} ${_image} /go/src/app/brutalinks.pem /etc/ssl/certs/

if [[ ${ENV} -eq "dev" ]]; then
  #ADD ./templates /templates
  buildah copy --from ${_builder} ${_image} /go/src/app/templates /templates
  #ADD ./assets /assets
  buildah copy --from ${_builder} ${_image} /go/src/app/assets /assts
  #ADD ./README.md /README.md
  buildah copy --from ${_builder} ${_image} /go/src/app/README /README.md
fi

#CMD ["/bin/brutalinks"]
buildah config --entrypoint '["/bin/brutalinks"]' ${_image}

# commit
buildah commit ${_image} ${_image_name}
