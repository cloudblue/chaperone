# Chaperone Makefile
# Standard targets for build, test, lint, and development

# Build variables
BINARY_NAME := chaperone
BUILD_DIR := bin
CMD_PATH := ./cmd/chaperone

# Prevent Go from auto-downloading a different toolchain version.
# This avoids silent compile/tool version mismatches when the local
# Go installation lags behind go.mod. If you see a version error,
# run: brew upgrade go
export GOTOOLCHAIN := local

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

# Development build flags (allows insecure targets and profiling for testing)
LDFLAGS_DEV := -ldflags "\
	-X main.Version=$(VERSION)-dev \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X 'github.com/cloudblue/chaperone/internal/proxy.allowInsecureTargets=true' \
	-X 'github.com/cloudblue/chaperone/internal/telemetry.allowProfiling=true'"

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

.PHONY: test-integration
test-integration: ## Run integration tests
	go test -v -tags=integration ./...

# ============================================================================
# Benchmarks (root module only; SDK has no hot-path code)
# ============================================================================

# benchstat for comparing benchmark runs (installed via go install)
BENCHSTAT := $(shell go env GOPATH)/bin/benchstat

.PHONY: bench
bench: ## Run all benchmarks
	@echo "Running benchmarks..."
	go test -run='^$$' -bench=. -benchmem -count=6 ./... | tee benchmark-current.txt
	@echo ""
	@echo "Results saved to benchmark-current.txt"

.PHONY: bench-short
bench-short: ## Run benchmarks with fewer iterations (quick check)
	go test -run='^$$' -bench=. -benchmem -count=1 ./...

.PHONY: bench-save
bench-save: ## Save current benchmarks as baseline
	@echo "Running benchmarks and saving as baseline..."
	go test -run='^$$' -bench=. -benchmem -count=6 ./... > benchmark-baseline.txt
	@echo "Baseline saved to benchmark-baseline.txt"

.PHONY: bench-compare
bench-compare: ## Compare current benchmarks against baseline
	@if [ ! -f benchmark-baseline.txt ]; then \
		echo "No baseline found. Run 'make bench-save' first."; \
		exit 1; \
	fi
	@if [ ! -x "$(BENCHSTAT)" ]; then \
		echo "benchstat not installed. Installing..."; \
		go install golang.org/x/perf/cmd/benchstat@latest; \
	fi
	@echo "Running current benchmarks..."
	@go test -run='^$$' -bench=. -benchmem -count=6 ./... > benchmark-current.txt
	@echo ""
	@echo "=== Benchmark Comparison ==="
	@$(BENCHSTAT) benchmark-baseline.txt benchmark-current.txt

.PHONY: bench-profile
bench-profile: ## Run benchmarks with CPU profiling (single package, -cpuprofile limitation)
	@echo "Running benchmarks with CPU profiling..."
	go test -run='^$$' -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./internal/proxy/
	@echo ""
	@echo "CPU profile: cpu.prof"
	@echo "Memory profile: mem.prof"
	@echo "Analyze with: go tool pprof cpu.prof"

# ============================================================================
# Docker
# ============================================================================

# Docker image settings
DOCKER_IMAGE := chaperone
DOCKER_TAG ?= poc
DOCKER_TEST_TAG ?= $(DOCKER_TAG)-test
ECHOSERVER_IMAGE := echoserver
ECHOSERVER_TAG ?= test

.PHONY: docker-build
docker-build: ## Build Docker image (production: HTTPS only, no test tools)
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG) (production)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: docker-build-test
docker-build-test: ## Build Docker image for testing (allows HTTP targets)
	@echo "Building Chaperone test image (production Dockerfile + ALLOW_INSECURE_TARGETS)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg ALLOW_INSECURE_TARGETS=true \
		-t $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) .

.PHONY: docker-build-echoserver
docker-build-echoserver: ## Build the echo server image for integration testing
	@echo "Building echoserver image..."
	docker build \
		-f test/Dockerfile.echoserver \
		-t $(ECHOSERVER_IMAGE):$(ECHOSERVER_TAG) .

.PHONY: docker-run
docker-run: ## Run Docker container (HTTP mode for testing)
	@echo "Running $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@echo "  Config: /app/config.yaml (TLS disabled, minimal allow_list)"
	@echo "  Override: -v /path/config.yaml:/app/config.yaml or -e CHAPERONE_*"
	docker run --rm -p 8443:8443 --name chaperone-test $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-test
docker-test: docker-build-test docker-build docker-build-echoserver ## Build and test Docker image (comprehensive validation)
	@test/scripts/docker-validation-suite.sh \
		--proxy-image "$(DOCKER_IMAGE):$(DOCKER_TEST_TAG)" \
		--prod-image "$(DOCKER_IMAGE):$(DOCKER_TAG)" \
		--echo-image "$(ECHOSERVER_IMAGE):$(ECHOSERVER_TAG)" \
		--config "$(CURDIR)/configs/docker-test.yaml" \
		--credentials "$(CURDIR)/test/testdata/docker-test-credentials.json"

.PHONY: docker-size
docker-size: ## Show Docker image size
	@docker images $(DOCKER_IMAGE):$(DOCKER_TAG) --format "{{.Repository}}:{{.Tag}}\t{{.Size}}"

.PHONY: docker-clean
docker-clean: ## Remove Docker images (production, test, and echoserver)
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) 2>/dev/null || true
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) 2>/dev/null || true
	docker rmi $(ECHOSERVER_IMAGE):$(ECHOSERVER_TAG) 2>/dev/null || true

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
	@echo "Installing benchstat..."
	go install golang.org/x/perf/cmd/benchstat@latest

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
