# Go envs
FROM golang:alpine as dev
RUN apk add --update --no-cache git curl
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
ARG PACKAGE
ENV APP_HOME $GOPATH/src/github.com/cloud104/k8s-rds/$PACKAGE
WORKDIR $APP_HOME
COPY Gopkg.toml Gopkg.lock ./
RUN dep ensure -vendor-only
VOLUME ["$APP_HOME"]

# Builder
FROM dev as builder
COPY . .
ARG PACKAGE
RUN CGO_ENABLED=0 GOOS=linux go build -o /entrypoint

# Prod
FROM drone/ca-certs
COPY --from=builder /entrypoint /usr/local/bin/entrypoint
EXPOSE 80
ENTRYPOINT ["entrypoint"]
