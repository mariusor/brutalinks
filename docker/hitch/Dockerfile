FROM alpine:edge

ARG HOSTNAME
ENV HOSTNAME ${HOSTNAME}

RUN echo "http://dl-3.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories
RUN apk update && \
    apk upgrade
RUN apk add --no-cache hitch openssl

RUN adduser -h /var/lib/hitch -s /sbin/nologin -u 1000 -D hitch
RUN mkdir -p /etc/hitch/ /etc/ssl/hitch/
RUN chown -R root:hitch /etc/hitch /etc/ssl/hitch

RUN openssl req -subj "/C=AQ/ST=Omond/L=Omond/O=${HOSTNAME}/OU=none/CN=${HOSTNAME}" -newkey rsa:2048 -sha256 -keyout ${HOSTNAME}.key -nodes -x509 -days 365 -out ${HOSTNAME}.crt && \
	cat ${HOSTNAME}.key ${HOSTNAME}.crt > /etc/ssl/hitch/main.pem

COPY hitch.conf /etc/hitch/hitch.conf
RUN apk del openssl

EXPOSE 443

#USER hitch:hitch
CMD ["hitch", "--workers=2", "--config=/etc/hitch/hitch.conf"]
