ARG ENV
FROM quay.io/fedbox/fedbox:${ENV:-dev} as fedbox

FROM alpine

ARG HOSTNAME
ARG OAUTH_PW
ARG ADMIN_PW

VOLUME /storage

ENV HOSTNAME $HOSTNAME
ENV ENV $ENV
ENV OAUTH_PW $OAUTH_PW
ENV ADMIN_PW $ADMIN_PW

COPY --from=fedbox /bin/ctl /bin/ctl
COPY bootstrap.sh /bin/bootstrap.sh
COPY setupkeys.sh /bin/setupkeys.sh

RUN apk update && apk add expect && rm -rf /var/cache/apk
