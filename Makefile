.PHONY: help build run test frontend docker clean

BINARY := camspeak
IMAGE   := ghcr.io/jeeftor/camspeak
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: frontend ## Build the camspeak binary
	go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY) .

run: ## Run the server locally
	go run main.go serve

frontend: ## Build the frontend assets
	cd frontend && bun install --frozen-lockfile && bun run build

test: ## Run the test suite
	gotestsum --format testdox ./...

docker: ## Build the multi-arch Docker image
	docker buildx build --build-arg VERSION=$(VERSION) --platform linux/amd64,linux/arm64 -t $(IMAGE):dev .

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -rf frontend/dist
