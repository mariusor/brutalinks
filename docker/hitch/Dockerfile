FROM alpine:edge

RUN echo "http://dl-3.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories
RUN apk update && \
    apk upgrade
RUN apk add --no-cache hitch

RUN adduser -h /var/lib/hitch -s /sbin/nologin -u 1000 -D hitch
RUN mkdir -p /etc/hitch/ /etc/ssl/hitch/
RUN chown -R root:hitch /etc/hitch /etc/ssl/hitch

COPY hitch.conf /etc/hitch/hitch.conf
VOLUME ["/etc/ssl/hitch/littr.pem"]

EXPOSE 443

#USER hitch:hitch
CMD ["hitch", "--workers=2", "--config=/etc/hitch/hitch.conf"]
