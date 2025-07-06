.PHONY: all build test clean docker-build docker-run install run

# Binary name
BINARY_NAME=keybench
DOCKER_IMAGE=keybench:latest

# Build variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOFILES=$(wildcard *.go)

# Build the binary
all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(GOBIN)/$(BINARY_NAME) ./cmd/main.go

run:
	@echo "Running $(BINARY_NAME)..."
	@go run ./cmd/main.go

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@go clean
	@rm -rf $(GOBIN)
	@rm -f coverage.out coverage.html

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE) .

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	@docker run --rm $(DOCKER_IMAGE)

# Install binary to system
install: build
	@echo "Installing $(BINARY_NAME)..."
	@go install ./cmd/main.go

# Run benchmarks
benchmark-example:
	@echo "Running example benchmark..."
	@go run cmd/main.go -a rsa,ecdsa,ed25519 -k 2048,256 -i 5 -p 2 -v

# Run linting
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...