FROM scratch
COPY bin/app /bin/app
COPY ./assets /assets
COPY ./templates ./templates

CMD ["/bin/app"]

