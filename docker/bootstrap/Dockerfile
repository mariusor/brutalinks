ARG ENV
ARG HOSTNAME
FROM ${HOSTNAME}/builder:${ENV} as builder

FROM gcr.io/distroless/base

ARG ENV
ARG DB_HOST
ARG DB_NAME
ARG DB_USER
ARG DB_PASSWORD
ARG POSTGRES_PASSWORD

ENV DB_HOST $DB_HOST
ENV DB_NAME $DB_NAME
ENV DB_USER $DB_USER
ENV DB_PASSWORD $DB_PASSWORD
ENV POSTGRES_PASSWORD $POSTGRES_PASSWORD

COPY --from=builder /go/src/app/bin/bootstrap /bootstrap
COPY --from=builder /go/src/app/db /db
COPY .env* /

WORKDIR /

CMD ["/bootstrap", "-user", "postgres", "-seed"]

