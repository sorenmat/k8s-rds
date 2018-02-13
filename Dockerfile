FROM alpine
MAINTAINER Soren Mathiasen <sorenm@mymessages.dk>
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
ADD cool-aide /cool-aide
ENTRYPOINT ["/cool-aide"]
