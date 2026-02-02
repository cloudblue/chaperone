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

# Security: Allow insecure HTTP targets (ONLY for development builds)
# Production builds MUST use ALLOW_INSECURE_TARGETS=false (default)
ALLOW_INSECURE_TARGETS ?= false

# Go build flags
LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X 'github.com/cloudblue/chaperone/internal/proxy.allowInsecureTargets=$(ALLOW_INSECURE_TARGETS)'"

# Development build flags (allows insecure targets for testing)
LDFLAGS_DEV := -ldflags "\
	-X main.Version=$(VERSION)-dev \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X 'github.com/cloudblue/chaperone/internal/proxy.allowInsecureTargets=true'"

# Default target
.PHONY: all
all: lint test build

# ============================================================================
# Build
# ============================================================================

.PHONY: build
build: ## Build the production binary (HTTPS targets only)
	@echo "Building $(BINARY_NAME) (production)..."
	@echo "  ALLOW_INSECURE_TARGETS=$(ALLOW_INSECURE_TARGETS)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: build-dev
build-dev: ## Build for development (allows HTTP targets, debug symbols)
	@echo "Building $(BINARY_NAME) (development)..."
	@echo "  ⚠️  WARNING: HTTP targets allowed - DO NOT USE IN PRODUCTION"
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS_DEV) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: run
run: build-dev ## Build and run
	@$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# ============================================================================
# Development Certificates
# ============================================================================

.PHONY: gencerts
gencerts: ## Generate test certificates for mTLS development (use DOMAINS="host1,ip1" for custom SANs)
	@go run ./cmd/gencerts $(if $(DOMAINS),-domains "$(DOMAINS)")

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

# golangci-lint binary location (installed via go install)
GOLANGCI_LINT := $(shell go env GOPATH)/bin/golangci-lint

.PHONY: lint
lint: ## Run linters (both modules)
	@if [ -x "$(GOLANGCI_LINT)" ]; then \
		$(GOLANGCI_LINT) run; \
		cd sdk && $(GOLANGCI_LINT) run; \
	else \
		echo "golangci-lint not installed. Run: make tools"; \
		exit 1; \
	fi

.PHONY: lint-fix
lint-fix: ## Run linters and fix issues
	$(GOLANGCI_LINT) run --fix
	cd sdk && $(GOLANGCI_LINT) run --fix

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

# golangci-lint version to install
GOLANGCI_LINT_VERSION := v2.8.0

.PHONY: tools
tools: ## Install development tools
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
