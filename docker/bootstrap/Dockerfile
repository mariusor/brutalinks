ARG ENV
ARG HOSTNAME
ARG OAUTH_PW
FROM ${HOSTNAME}/builder:${ENV} as builder

FROM alpine

ARG LISTEN

EXPOSE $LISTEN
VOLUME /storage

ENV LISTEN $LISTEN
ENV ENV $ENV
ENV OAUTH_PW $OAUTH_PW

COPY .env* ./

COPY --from=builder /go/src/fedbox/bin/ctl /bin/ctl
COPY bootstrap.sh /bin/bootstrap.sh

