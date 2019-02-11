FROM golang

RUN mkdir -p /go/src/github.com/cloud104/k8s-rds
WORKDIR /go/src/github.com/cloud104/k8s-rds
COPY . .
RUN go build -ldflags "-linkmode external -extldflags -static" -a main.go

FROM scratch
COPY --from=0 /go/src/github.com/cloud104/k8s-rds /k8s-rds
ENTRYPOINT ["/k8s-rds"]