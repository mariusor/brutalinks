ARG ENV
ARG HOSTNAME
FROM ${HOSTNAME}/builder:${ENV} as builder

FROM gcr.io/distroless/base

ARG HOSTNAME
ARG LISTEN

EXPOSE $LISTEN

ENV HOSTNAME $HOSTNAME
ENV LISTEN $LISTEN
ENV ENV $ENV

COPY --from=builder /go/src/app/bin/app /bin/app
COPY --from=builder /go/src/app/assets /assets
COPY --from=builder /go/src/app/templates ./templates
COPY .env* ./

CMD ["/bin/app"]

