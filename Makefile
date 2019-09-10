NAME := k8s-rds
ENV := CGO_ENABLED=0
GO := $(ENV) go

all: mod test build

mod:
	$(GO) mod download
tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint
test:
	$(GO) test ./... -v
lint:
	$(ENV) golangci-lint
build:
	$(GO) build -o bin/$(NAME)

.PHONY: all mod test build
