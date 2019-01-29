ARG ENV
FROM littr/builder:${ENV} as builder
FROM gcr.io/distroless/base

ARG HOSTNAME
ARG LISTEN
ARG ENV

EXPOSE $LISTEN

ENV HOSTNAME $HOSTNAME
ENV LISTEN $LISTEN
ENV ENV $ENV

COPY --from=builder /go/src/app/bin/bootstrap /
COPY --from=builder /go/src/app/bin/keys /
COPY --from=builder /go/src/app/bin/votes /

CMD ["/votes"]

