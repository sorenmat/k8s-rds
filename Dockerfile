FROM alpine
MAINTAINER Soren Mathiasen <sorenm@mymessages.dk>
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
ADD k8s-rds /k8s-rds
ENTRYPOINT ["/k8s-rds"]
