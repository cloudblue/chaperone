# Chaperone Makefile
# Standard targets for build, test, lint, and development

# Build variables
BINARY_NAME := chaperone
BUILD_DIR := bin
CMD_PATH := ./cmd/chaperone

# Version information (override via environment or CI)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

# Default target
.PHONY: all
all: lint test build

# ============================================================================
# Build
# ============================================================================

.PHONY: build
build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: build-dev
build-dev: ## Build for development (faster, with debug symbols)
	@echo "Building $(BINARY_NAME) (dev)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: run
run: build-dev ## Build and run
	@$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# ============================================================================
# Testing
# ============================================================================

.PHONY: test
test: ## Run tests (both modules)
	go test -v ./...
	cd sdk && go test -v ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	go test -race -v ./...
	cd sdk && go test -race -v ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage
	go test -coverprofile=coverage.out ./...
	cd sdk && go test -coverprofile=coverage-sdk.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-short
test-short: ## Run short tests only
	go test -short -v ./...

# ============================================================================
# Code Quality
# ============================================================================

.PHONY: lint
lint: ## Run linters (both modules)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
		cd sdk && golangci-lint run; \
	else \
		echo "golangci-lint not installed. Run: make tools"; \
		exit 1; \
	fi

.PHONY: lint-fix
lint-fix: ## Run linters and fix issues
	golangci-lint run --fix
	cd sdk && golangci-lint run --fix

.PHONY: fmt
fmt: ## Format code (both modules)
	go fmt ./...
	gofmt -s -w .
	cd sdk && go fmt ./...
	cd sdk && gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...
	cd sdk && go vet ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod (both modules)
	go mod tidy
	go mod verify
	cd sdk && go mod tidy

# ============================================================================
# Development Tools
# ============================================================================

.PHONY: tools
tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
