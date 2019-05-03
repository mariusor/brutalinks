FROM golang:1.12

ARG ENV
ARG PWD

ENV GO111MODULE=on
ENV ENV=${ENV:-dev}

#VOLUME /go/src/app
# master
#RUN git clone https://github.com/mariusor/littr.go app

WORKDIR /go/src/app
COPY ./ ./

RUN go mod download || true
RUN make all

#CMD ["make", "all"]
