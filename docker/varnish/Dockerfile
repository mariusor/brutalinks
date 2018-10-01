FROM alpine:3.8

ENV VARNISH_BACKEND_PORT 80

EXPOSE 80
EXPOSE 6081

RUN apk update && \
    apk upgrade && \
    apk add varnish

RUN mkdir -p /var/lib/varnish/`hostname` && chown varnish /var/lib/varnish/`hostname`

COPY default.vcl /etc/varnish/default.vcl
#RUN chown :varnish /etc/varnish/default.vcl
#RUN chmod g+r /etc/varnish/default.vcl

CMD ["/bin/sh", "-c", "varnishd -a :80 -a :6081,proxy -f /etc/varnish/default.vcl -s malloc,64M && varnishlog"]
