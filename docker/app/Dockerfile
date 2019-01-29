ARG ENV
FROM littr/builder:${ENV} as builder
FROM gcr.io/distroless/base

ARG HOSTNAME
ARG LISTEN
ARG PORT

EXPOSE $LISTEN

ENV HOSTNAME $HOSTNAME
ENV LISTEN $LISTEN
ENV ENV $ENV

COPY --from=builder /go/src/app/bin/app /bin/app
COPY --from=builder /go/src/app/assets /assets
COPY --from=builder /go/src/app/templates ./templates

CMD ["/bin/app"]

