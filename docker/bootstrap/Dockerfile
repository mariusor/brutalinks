FROM golang:1.11

ARG DB_HOST
ARG DB_NAME
ARG DB_USER
ARG DB_PASSWORD
ARG POSTGRES_PASSWORD

ENV DB_HOST $DB_HOST
ENV DB_NAME $DB_NAME
ENV DB_USER $DB_USER
ENV DB_PASSWORD $DB_PASSWORD
ENV POSTGRES_PASSWORD $POSTGRES_PASSWORD

RUN apt update -yqq && apt install -yqq postgresql-client

ADD ./wait-for-db.sh /bin/wait-for-db.sh
ADD ./bin/bootstrap /bin/bootstrap
ADD ./db /db

CMD ["/bin/wait-for-db.sh", "db", "/bin/bootstrap", "-user", "postgres", "-seed"]

