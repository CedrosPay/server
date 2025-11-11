.PHONY: help build test run clean install dev docker-build docker-run docker-up docker-down docker-logs lint fmt tidy

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Docker configuration
DOCKER_IMAGE := cedrospay-server
DOCKER_TAG := $(VERSION)

# Default target
help:
	@echo "Cedros Pay Server - Available Commands:"
	@echo ""
	@echo "Development:"
	@echo "  make build            Build the server binary"
	@echo "  make test             Run all tests"
	@echo "  make test-coverage    Run tests with coverage report"
	@echo "  make run              Run server (auto-detects config)"
	@echo "  make run ARGS=\"...\"    Run with custom args (e.g. ARGS=\"-config path.yaml\")"
	@echo "  make dev              Run with live reload (requires air)"
	@echo "  make clean            Remove build artifacts"
	@echo "  make install          Install dependencies"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint             Run linters (requires golangci-lint)"
	@echo "  make fmt              Format code with gofmt"
	@echo "  make tidy             Tidy and verify go.mod"
	@echo "  make pre-commit       Run all checks before committing"
	@echo ""
	@echo "Docker - Simple (server only):"
	@echo "  make docker-build     Build Docker image"
	@echo "  make docker-run       Run server in Docker"
	@echo "  make docker-simple-up Start server with simple compose"
	@echo "  make docker-simple-down Stop simple compose"
	@echo ""
	@echo "Docker - Full Stack (server + postgres + redis):"
	@echo "  make docker-up        Start all services with docker-compose"
	@echo "  make docker-down      Stop all services"
	@echo "  make docker-logs      View service logs"
	@echo "  make docker-ps        Show running containers"
	@echo "  make docker-clean     Remove all containers and volumes"
	@echo ""
	@echo "Production:"
	@echo "  make prod-build       Build optimized production binary"
	@echo "  make docker-push      Push Docker image to registry"
	@echo ""

# Build the server binary
build:
	@echo "Building server (version: $(VERSION))..."
	@go build $(LDFLAGS) -o bin/server ./cmd/server
	@echo "✓ Binary created at bin/server"

# Run all tests
test:
	@echo "Running tests..."
	@go test ./... -v -race -coverprofile=coverage.out
	@echo "✓ Tests passed"

# Run tests with coverage report
test-coverage: test
	@go tool cover -html=coverage.out

# Run the server (auto-detects config, or pass ARGS="-config path/to/config.yaml")
run:
	@echo "Starting server..."
	@go run $(LDFLAGS) ./cmd/server $(ARGS)

# Run with live reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Error: 'air' not found. Install with: go install github.com/cosmtrek/air@latest"; \
		exit 1; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ dist/ coverage.out
	@echo "✓ Clean complete"

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@echo "✓ Dependencies installed"

# Run linters (requires golangci-lint)
lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "Error: 'golangci-lint' not found. Install from: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	@gofmt -s -w .
	@echo "✓ Code formatted"

# Tidy dependencies and verify
tidy:
	@echo "Tidying go.mod..."
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "Error: go.mod or go.sum has uncommitted changes after tidy" && exit 1)
	@echo "✓ go.mod is tidy"

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Run server in Docker (standalone)
docker-run:
	@echo "Running server in Docker..."
	@docker run --rm -p 8080:8080 --env-file .env $(DOCKER_IMAGE):$(DOCKER_TAG)

# Start services with docker-compose (full stack)
docker-up:
	@echo "Starting all services with docker-compose..."
	@VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) docker-compose up -d
	@echo "✓ Services started. View logs with: make docker-logs"

# Stop docker-compose services
docker-down:
	@echo "Stopping all services..."
	@docker-compose down
	@echo "✓ Services stopped"

# Start server only (simple compose)
docker-simple-up:
	@echo "Starting server with simple compose..."
	@VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) docker-compose -f docker-compose.simple.yml up -d
	@echo "✓ Server started at http://localhost:8080"

# Stop simple compose
docker-simple-down:
	@echo "Stopping server..."
	@docker-compose -f docker-compose.simple.yml down
	@echo "✓ Server stopped"

# View docker-compose logs
docker-logs:
	@docker-compose logs -f

# Show running containers
docker-ps:
	@docker-compose ps

# Clean up all containers and volumes
docker-clean:
	@echo "Cleaning up Docker resources..."
	@docker-compose down -v
	@docker system prune -f
	@echo "✓ Docker resources cleaned"

# Push Docker image to registry (set DOCKER_REGISTRY env var)
docker-push:
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "Error: DOCKER_REGISTRY not set. Export DOCKER_REGISTRY=your-registry"; \
		exit 1; \
	fi
	@echo "Pushing to $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker tag $(DOCKER_IMAGE):latest $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):latest
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):latest
	@echo "✓ Image pushed to registry"

# Build optimized production binary
prod-build:
	@echo "Building production binary (version: $(VERSION))..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="-w -s $(LDFLAGS)" \
		-o bin/server-linux-amd64 \
		./cmd/server
	@echo "✓ Production binary created at bin/server-linux-amd64"

# Quick check before commit (fmt, tidy, test)
pre-commit: fmt tidy test
	@echo "✓ Pre-commit checks passed"
