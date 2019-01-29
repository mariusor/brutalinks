FROM alpine:edge

ENV VARNISH_BACKEND_PORT 80

RUN echo "http://dl-3.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories
RUN apk update && \
    apk upgrade
RUN apk add --no-cache varnish

RUN mkdir -p /var/lib/varnish/$(hostname) && chown varnish /var/lib/varnish/$(hostname)

COPY default.vcl /etc/varnish/default.vcl

EXPOSE ${VARNISH_BACKEND_PORT}
EXPOSE 6081

CMD ["/bin/sh", "-c", "varnishd -a :80 -a :6081,PROXY -f /etc/varnish/default.vcl -s malloc,64M && varnishlog -b -i BereqMethod,BereqURL,BerespStatus,BerespReason,End"]
