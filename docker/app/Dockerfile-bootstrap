FROM scratch

ARG DB_HOST
ARG DB_NAME
ARG DB_USER
ARG DB_PASSWORD

ENV DB_HOST $DB_HOST
ENV DB_NAME $DB_NAME
ENV DB_USER $DB_USER
ENV DB_PASSWORD $DB_PASSWORD

ADD ./bin/bootstrap /bin/bootstrap
ADD ./db /db

CMD ["/bin/bootstrap", "-user", "postgres"]

