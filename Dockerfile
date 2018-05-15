FROM golang:alpine AS build-env
ADD . /go/src/github.com/sorenmat/k8s-rds
RUN cd /go/src/github.com/sorenmat/k8s-rds && go build -o k8s-rds


FROM alpine
MAINTAINER Soren Mathiasen <sorenm@mymessages.dk>
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
WORKDIR /app
COPY --from=build-env /go/src/github.com/sorenmat/k8s-rds/k8s-rds /app/
ENTRYPOINT ./k8s-rds
