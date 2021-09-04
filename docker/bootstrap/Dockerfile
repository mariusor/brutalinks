ARG ENV
FROM quay.io/go-ap/fedbox:${ENV:-dev} as fedbox

FROM alpine

ARG FEDBOX_HOSTNAME
ARG OAUTH2_CALLBACK_URL
ARG OAUTH2_SECRET
ARG ADMIN_PW

VOLUME /storage

ENV FEDBOX_HOSTNAME $FEDBOX_HOSTNAME
ENV ENV $ENV
ENV OAUTH2_SECRET $OAUTH2_SECRET
ENV ADMIN_PW $ADMIN_PW
ENV OAUTH2_CALLBACK_URL $OAUTH2_CALLBACK_URL

COPY --from=fedbox /bin/ctl /bin/ctl
COPY bootstrap.sh /bin/bootstrap.sh
COPY useradd.sh /bin/useradd.sh
COPY clientadd.sh /bin/clientadd.sh

RUN apk update && apk add jq expect && rm -rf /var/cache/apk
