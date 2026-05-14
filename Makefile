PHONY: build run docker-build docker-run test clean

# Binary name
BINARY=agent

# Build the binary locally
build:
	go build -o bin/$(BINARY) ./cmd/agent

# Run locally (requires env vars set)
run: build
	./bin/$(BINARY)

# Build Docker image
docker-build:
	docker build -t ai-agent: latest .

# Run with Docker (example with Claude)
docker-run:
	docker run --rm \
		-e ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY) \
		-v $(PWD)/workspace:/workspace \
		-v $(PWD)/skills:/skills \
		ai-agent: latest

# Run with Docker Compose
compose-up:
	docker-compose up --build

# Run tests
test:
	go test ./ ...

# Run tests with verbose output
test-v:
	go test -v ./ ...

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Format code
fmt:
	go fmt ./ ...

# Lint (requires golangci-lint)
lint:
	go lint run ./ ...

deps:
	go mod tidy
	go mod download