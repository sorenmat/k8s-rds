FROM eu.gcr.io/tradeshift-base/tradeshift-golang:17 AS builder

WORKDIR /app
RUN apk --no-cache add git make

COPY Makefile go.mod go.sum ./
RUN make mod tools

COPY . .
RUN make test lint build

FROM eu.gcr.io/tradeshift-base/tradeshift-alpine:latest

MAINTAINER Soren Mathiasen <sorenm@mymessages.dk>

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bin/k8s-rds /k8s-rds

RUN chown tradeshift /k8s-rds
USER tradeshift

ENTRYPOINT ["/k8s-rds"]
