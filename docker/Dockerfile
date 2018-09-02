FROM scratch

ARG HOSTNAME
ARG LISTEN

EXPOSE $LISTEN

ENV HOSTNAME $HOSTNAME
ENV LISTEN $LISTEN

ADD ./bin/app /bin/app
ADD ./assets /assets
ADD ./templates ./templates

CMD ["/bin/app"]

