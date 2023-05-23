FROM golang:1.20.0-alpine AS builder

WORKDIR /app
RUN apk --no-cache add git make

COPY Makefile go.mod go.sum ./
RUN make mod tools

COPY . .
RUN make test lint build

FROM alpine

MAINTAINER Soren Mathiasen <sorenm@mymessages.dk>

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bin/k8s-rds /k8s-rds

ENTRYPOINT ["/k8s-rds"]
