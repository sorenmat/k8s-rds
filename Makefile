NAME := k8s-rds
ENV := CGO_ENABLED=0
GO := $(ENV) go

all: mod test build

mod:
	$(GO) mod download
tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2
test:
	$(GO) test ./... -v
lint:
	$(ENV) golangci-lint run
build:
	$(GO) build -o bin/$(NAME)

.PHONY: all mod test build
