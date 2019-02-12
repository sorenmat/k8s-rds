FROM alpine:latest
RUN apk --no-cache add ca-certificates

COPY k8s-rds .

CMD ["/k8s-rds"]