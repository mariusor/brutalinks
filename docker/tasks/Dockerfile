ARG ENV
ARG HOSTNAME
FROM ${HOSTNAME}/builder:${ENV} as builder
FROM alpine:edge

#COPY --from=builder /go/src/app/bin/bootstrap /
COPY --from=builder /go/src/app/bin/keys /
COPY --from=builder /go/src/app/bin/votes /

RUN echo '*/10  *  *  *  *   /keys -seed `shuf -i 1-100 -n 1`' > /etc/crontabs/root
RUN echo '*     *  *  *  *   /votes -items -since 2m' >> /etc/crontabs/root
RUN echo '*/2   *  *  *  *   /votes -accounts -since 2m' >> /etc/crontabs/root

CMD ["crond", "-d4", "-f"]
