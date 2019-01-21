FROM scratch

ARG HOSTNAME
ARG LISTEN
ARG PORT
ARG ENV

EXPOSE $LISTEN

ENV HOSTNAME $HOSTNAME
ENV LISTEN $LISTEN
ENV ENV $ENV

ADD ./bin/app /bin/app
ADD ./assets /assets
ADD ./templates ./templates

CMD ["/bin/app"]

