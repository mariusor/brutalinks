ARG ENV
ARG HOSTNAME
FROM ${HOSTNAME}/builder:${ENV} as builder

FROM gcr.io/distroless/base

ARG LISTEN

EXPOSE $LISTEN

ENV LISTEN $LISTEN
ENV ENV $ENV

COPY --from=builder /go/src/fedbox/bin/fedbox /bin/fedbox
#COPY --from=builder /go/src/fedbox/bin/ctl /bin/ctl
COPY .env* ./

#RUN /bin/ctl bootstrap

CMD ["/bin/fedbox"]

