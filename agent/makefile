APP_NAME=jarvis-agent
IMAGE=docker.io/jm98/$(APP_NAME):latest
NAMESPACE=jarvis

# Default target
.PHONY: all
all: build docker push deploy

# Build the Go binary
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(APP_NAME) main.go

# Build the container image
.PHONY: docker
docker-build:
	docker build -t $(IMAGE) .

# Push the container image to registry
.PHONY: push
docker-push:
	docker push $(IMAGE)

# Apply DaemonSet manifest
.PHONY: deploy
deploy:
	kubectl apply -k deploy/

# Clean up build artifacts
.PHONY: clean
clean:
	rm -f $(APP_NAME)