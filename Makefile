BINARY_NAME := selenosis
DOCKER_REGISTRY ?= ${REGISTRY}
IMAGE_NAME := $(DOCKER_REGISTRY)/$(BINARY_NAME)
VERSION ?= ${VERSION}
PLATFORM ?= linux/amd64
CONTAINER_TOOL ?= docker

.PHONY: fmt vet tidy test docker-build docker-push deploy clean show-vars

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test -race -count=1 -cover ./...

docker-build: tidy fmt vet test
	$(CONTAINER_TOOL)  build \
		--platform $(PLATFORM) \
		-t $(IMAGE_NAME):$(VERSION) \
		.

docker-push:
	$(CONTAINER_TOOL) push $(IMAGE_NAME):$(VERSION)

deploy: docker-build docker-push

clean:
	$(CONTAINER_TOOL) rmi $(IMAGE_NAME):$(VERSION) 2>/dev/null || true

show-vars:
	@echo "BINARY_NAME: $(BINARY_NAME)"
	@echo "DOCKER_REGISTRY: $(DOCKER_REGISTRY)"
	@echo "IMAGE_NAME: $(IMAGE_NAME)"
	@echo "VERSION: $(VERSION)"
	@echo "PLATFORM: $(PLATFORM)"
	@echo "CONTAINER_TOOL: $(CONTAINER_TOOL)"
