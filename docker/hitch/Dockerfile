FROM alpine:3.8

ARG HITCH_VERSION=1.4.8
ARG HITCH_CHECKSUM=d52ba690d90c25bbfca73f5e0ed427738366dac12faf46fb5834e497cc2d1ac3

RUN echo "http://dl-3.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories
RUN apk update && \
    apk upgrade
RUN apk add --no-cache openssl libev
RUN apk add --no-cache --virtual .build-deps gcc curl libev-dev openssl-dev autoconf libtool py-docutils make automake pkgconfig gcc musl-dev byacc flex \
    && mkdir /usr/src \
    && cd /usr/src \
    && curl -SL -o hitch.tar.gz https://hitch-tls.org/source/hitch-${HITCH_VERSION}.tar.gz \
    && echo "${HITCH_CHECKSUM}  hitch.tar.gz" > sha256sums.txt \
    && sha256sum -c sha256sums.txt \
    && tar xzf hitch.tar.gz \
    && cd hitch-${HITCH_VERSION} \
    && ./configure --bindir=/usr/bin --sbindir=/usr/sbin --libexecdir=/usr/libexec --sysconfdir=/etc --localstatedir=/var --libdir=/usr/lib \
    && make \
    && make install \
    && cd /usr/src \
    && rm hitch.tar.gz sha256sums.txt \
    && apk del .build-deps

RUN adduser -h /var/lib/hitch -s /sbin/nologin -u 1000 -D hitch
RUN mkdir -p /etc/hitch/ /etc/ssl/hitch/
RUN chown -R root:hitch /etc/hitch /etc/ssl/hitch
RUN rm -rf /tmp/*

COPY hitch.conf /etc/hitch/hitch.conf
VOLUME ["/etc/ssl/hitch/littr.pem"]

EXPOSE 443

USER hitch:hitch
CMD ["hitch", "--workers=2", "--config=/etc/hitch/hitch.conf"]
