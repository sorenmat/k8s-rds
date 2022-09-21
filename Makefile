NAME := k8s-rds
ENV := CGO_ENABLED=0
GO := $(ENV) go

IMAGE_NAME := $(shell echo "liskl/k8s-rds")
IMAGE_TAG := $(shell echo "$$(git rev-parse --abbrev-ref HEAD | sed 's,/,-,g')-$$(date '+%s')")

FULL_IMAGE_NAME := $(IMAGE_NAME):$(IMAGE_TAG)

all: mod test build

mod:
	$(GO) mod download
tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.43.0
test:
	$(GO) test ./... -v
lint:
	$(ENV) golangci-lint run
build:
	$(GO) build -o bin/$(NAME)

docker-build:
	docker build --no-cache --load -t $(FULL_IMAGE_NAME) .

docker-tag: docker-build
	docker tag $(FULL_IMAGE_NAME) $(IMAGE_NAME):latest

docker-push: docker-tag
	docker push "$(IMAGE_NAME):$(IMAGE_TAG)"
	docker push "$(IMAGE_NAME):latest"

.PHONY: all mod test build
