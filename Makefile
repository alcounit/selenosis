BINARY_NAME       := selenosis
DOCKER_REGISTRY   := 192.168.1.101:30000
IMAGE_NAME        := $(DOCKER_REGISTRY)/$(BINARY_NAME)
VERSION           ?= v2.0.0
PLATFORM          := linux/amd64

.PHONY: all docker-build docker-push deploy clean show-vars

all: docker-build docker-push

docker-build:
	docker build \
		--platform $(PLATFORM) \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_NAME):$(VERSION) \
		.

docker-push:
	docker push $(IMAGE_NAME):$(VERSION)

deploy: docker-build docker-push

clean:
	docker rmi $(IMAGE_NAME):$(VERSION) 2>/dev/null || true

show-vars:
	@echo "BINARY_NAME:     $(BINARY_NAME)"
	@echo "IMAGE_NAME:      $(IMAGE_NAME)"
	@echo "VERSION:         $(VERSION)"
	@echo "PLATFORM:        $(PLATFORM)"
