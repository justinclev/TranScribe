.PHONY: help test lint fmt build clean run-server run-cli docker-build docker-up docker-down install-tools tidy all

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_SERVER=bin/server
BINARY_CLI=bin/cli
DOCKER_IMAGE=transcribe
GO=go
GOFLAGS=-v

## help: Display this help message
help:
	@echo "TranScribe - Available Make Commands"
	@echo ""
	@echo "Development Commands:"
	@echo "  make test          - Run all tests"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make fmt           - Format all Go files"
	@echo "  make tidy          - Run go mod tidy"
	@echo "  make build         - Build server and CLI binaries"
	@echo "  make run-server    - Run the API server"
	@echo "  make run-cli       - Run the CLI (use ARGS='...' for arguments)"
	@echo "  make clean         - Remove build artifacts"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-build  - Build Docker image"
	@echo "  make docker-up     - Start services with docker-compose"
	@echo "  make docker-down   - Stop services with docker-compose"
	@echo ""
	@echo "Setup Commands:"
	@echo "  make install-tools - Install development tools (air, swag, etc.)"
	@echo "  make all           - Run fmt, tidy, lint, and test"
	@echo ""

## test: Run all tests
test:
	@echo "Running tests..."
	$(GO) test ./... -race -coverprofile=coverage.out

## test-verbose: Run tests with verbose output
test-verbose:
	@echo "Running tests (verbose)..."
	$(GO) test ./... -v -race -coverprofile=coverage.out

## coverage: Show test coverage
coverage: test
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	@if [ -f ./golangci-lint ]; then \
		./golangci-lint run ./...; \
	elif command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install with: make install-tools"; \
		exit 1; \
	fi

## fmt: Format all Go files
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## tidy: Run go mod tidy
tidy:
	@echo "Tidying go modules..."
	$(GO) mod tidy

## build: Build both server and CLI binaries
build: build-server build-cli

## build-server: Build the server binary
build-server:
	@echo "Building server..."
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_SERVER) ./cmd/server

## build-cli: Build the CLI binary
build-cli:
	@echo "Building CLI..."
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_CLI) ./cmd/cli

## run-server: Run the API server
run-server:
	@echo "Starting server..."
	$(GO) run ./cmd/server

## run-server-air: Run the API server with hot reloading (requires air)
run-server-air:
	@echo "Starting server with hot reload..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not found. Install with: make install-tools"; \
		exit 1; \
	fi

## run-cli: Run the CLI (use ARGS='...' for arguments)
run-cli:
	@echo "Running CLI..."
	$(GO) run ./cmd/cli $(ARGS)

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf tmp/
	rm -f coverage.out coverage.html
	$(GO) clean

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):latest .

## docker-up: Start services with docker-compose
docker-up:
	@echo "Starting Docker services..."
	docker-compose up -d

## docker-down: Stop services with docker-compose
docker-down:
	@echo "Stopping Docker services..."
	docker-compose down

## docker-logs: Show Docker logs
docker-logs:
	docker-compose logs -f

## install-tools: Install development tools
install-tools:
	@echo "Installing development tools..."
	@echo "Installing air for hot reloading..."
	$(GO) install github.com/cosmtrek/air@latest
	@echo "Installing swag for API documentation..."
	$(GO) install github.com/swaggo/swag/cmd/swag@latest
	@echo "Installing golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
	fi
	@echo "Installing pre-commit..."
	@if ! command -v pre-commit >/dev/null 2>&1; then \
		echo "Please install pre-commit manually:"; \
		echo "  - macOS: brew install pre-commit"; \
		echo "  - Other: pip install pre-commit"; \
	fi
	@echo "Tools installed successfully!"

## swag: Generate Swagger documentation
swag:
	@echo "Generating Swagger docs..."
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g cmd/server/main.go -o docs; \
	else \
		echo "swag not found. Install with: make install-tools"; \
		exit 1; \
	fi

## all: Run fmt, tidy, lint, and test
all: fmt tidy lint test
	@echo "All checks passed!"

## dev: Quick development workflow (fmt + test)
dev: fmt test
	@echo "Development checks passed!"
