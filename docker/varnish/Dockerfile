FROM alpine:3.8

ENV VARNISH_BACKEND_PORT 80

EXPOSE 80
EXPOSE 6081

RUN apk update && \
    apk upgrade && \
    apk add varnish

RUN mkdir -p /var/lib/varnish/`hostname` && chown varnish /var/lib/varnish/`hostname`

COPY default.vcl /etc/varnish/default.vcl
RUN chown -R varnish: /etc/varnish/

CMD ["varnishd", "-a :80", "-a 6081,proxy", "-f /etc/varnish/default.vcl"]
