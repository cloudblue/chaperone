# Copyright 2026 CloudBlue LLC
# SPDX-License-Identifier: Apache-2.0

# Chaperone Makefile
# Standard targets for build, test, lint, and development

# Build variables
BINARY_NAME := chaperone
ONBOARD_BINARY := chaperone-onboard
BUILD_DIR := bin
CMD_PATH := ./cmd/chaperone
ONBOARD_CMD_PATH := ./cmd/chaperone-onboard

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

ONBOARD_LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

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

.PHONY: ci
ci: fmt license-check lint test-race gosec govulncheck build ## Run all CI checks locally

# ============================================================================
# Build
# ============================================================================

.PHONY: build
build: ## Build the production binary (HTTPS targets only)
	@echo "Building $(BINARY_NAME) (production)..."
	@echo "  ALLOW_INSECURE_TARGETS=$(ALLOW_INSECURE_TARGETS)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: build-onboard
build-onboard: ## Build the onboarding CLI tool
	@echo "Building $(ONBOARD_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(ONBOARD_LDFLAGS) -o $(BUILD_DIR)/$(ONBOARD_BINARY) $(ONBOARD_CMD_PATH)

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
# Admin Portal
# ============================================================================

ADMIN_BINARY_NAME := chaperone-admin
ADMIN_MODULE_DIR := admin
ADMIN_CMD_PATH := ./cmd/chaperone-admin
ADMIN_UI_DIR := admin/ui

ADMIN_LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

ADMIN_LDFLAGS_DEV := -ldflags "\
	-X main.Version=$(VERSION)-dev \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

.PHONY: build-admin
build-admin: build-admin-ui ## Build the admin portal binary (production)
	@echo "Building $(ADMIN_BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	cd $(ADMIN_MODULE_DIR) && CGO_ENABLED=0 go build $(ADMIN_LDFLAGS) -o ../$(BUILD_DIR)/$(ADMIN_BINARY_NAME) $(ADMIN_CMD_PATH)

.PHONY: build-admin-dev
build-admin-dev: ## Build admin portal for development (no UI build needed)
	@echo "Building $(ADMIN_BINARY_NAME) (development)..."
	@mkdir -p $(BUILD_DIR)
	cd $(ADMIN_MODULE_DIR) && go build -tags dev $(ADMIN_LDFLAGS_DEV) -o ../$(BUILD_DIR)/$(ADMIN_BINARY_NAME) $(ADMIN_CMD_PATH)

.PHONY: build-admin-ui
build-admin-ui: ## Build the admin portal SPA
	@echo "Building admin UI..."
	cd $(ADMIN_UI_DIR) && pnpm install && pnpm build

.PHONY: run-admin
run-admin: build-admin-dev ## Build and run admin portal
	@$(BUILD_DIR)/$(ADMIN_BINARY_NAME)

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
test: ## Run tests (all modules)
	go test -v ./...
	cd sdk && go test -v ./...
	cd plugins/contrib && go test -v ./...
	cd admin && go test -tags dev -v ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	go test -race -v ./...
	cd sdk && go test -race -v ./...
	cd plugins/contrib && go test -race -v ./...
	cd admin && go test -race -tags dev -v ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage
	go test -coverprofile=coverage.out ./...
	cd sdk && go test -coverprofile=coverage-sdk.out ./...
	cd plugins/contrib && go test -coverprofile=coverage-contrib.out ./...
	cd admin && go test -tags dev -coverprofile=coverage-admin.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-short
test-short: ## Run short tests only
	go test -short -v ./...
	cd admin && go test -short -tags dev -v ./...

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

# Tool binary locations (installed via go install)
GOLANGCI_LINT := $(shell go env GOPATH)/bin/golangci-lint
ADDLICENSE := $(shell go env GOPATH)/bin/addlicense
GOSEC := $(shell go env GOPATH)/bin/gosec
GOVULNCHECK := $(shell go env GOPATH)/bin/govulncheck

.PHONY: lint
lint: ## Run linters (all modules)
	@if [ -x "$(GOLANGCI_LINT)" ]; then \
		$(GOLANGCI_LINT) run && \
		(cd sdk && $(GOLANGCI_LINT) run) && \
		(cd plugins/contrib && $(GOLANGCI_LINT) run) && \
		(cd admin && $(GOLANGCI_LINT) run); \
	else \
		echo "golangci-lint not installed. Run: make tools"; \
		exit 1; \
	fi

.PHONY: lint-fix
lint-fix: ## Run linters and fix issues
	$(GOLANGCI_LINT) run --fix
	cd sdk && $(GOLANGCI_LINT) run --fix
	cd plugins/contrib && $(GOLANGCI_LINT) run --fix
	cd admin && $(GOLANGCI_LINT) run --fix

.PHONY: fmt
fmt: ## Format code (all modules)
	go fmt ./...
	gofmt -s -w .
	cd sdk && go fmt ./...
	cd sdk && gofmt -s -w .
	cd plugins/contrib && go fmt ./...
	cd plugins/contrib && gofmt -s -w .
	cd admin && go fmt ./...
	cd admin && gofmt -s -w .

.PHONY: vet
vet: ## Run go vet (all modules)
	go vet ./...
	cd sdk && go vet ./...
	cd plugins/contrib && go vet ./...
	cd admin && go vet ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod (all modules)
	go mod tidy
	go mod verify
	cd sdk && go mod tidy
	cd plugins/contrib && go mod tidy && go mod verify
	cd admin && go mod tidy

.PHONY: gosec
gosec: ## Run gosec security scanner (all modules)
	@if [ -x "$(GOSEC)" ]; then \
		$(GOSEC) -exclude=G706 -exclude-dir=sdk -exclude-dir=plugins ./... && \
		(cd sdk && $(GOSEC) ./...) && \
		(cd admin && $(GOSEC) ./...); \
	else \
		echo "gosec not installed. Run: make tools"; \
		exit 1; \
	fi

.PHONY: govulncheck
govulncheck: ## Run govulncheck vulnerability scanner (all modules)
	@if [ -x "$(GOVULNCHECK)" ]; then \
		$(GOVULNCHECK) ./... && \
		(cd sdk && $(GOVULNCHECK) ./...) && \
		(cd admin && $(GOVULNCHECK) ./...); \
	else \
		echo "govulncheck not installed. Run: make tools"; \
		exit 1; \
	fi

# Common addlicense flags
ADDLICENSE_FLAGS := -f .copyright-header.tmpl \
	-ignore 'test/load/lib/**' \
	-ignore 'test/testdata/**' \
	-ignore 'plugins/reference/testdata/**' \
	-ignore '.golangci.yml' \
	-ignore '**/*.json' \
	-ignore '**/*.md' \
	-ignore '**/*.mod' \
	-ignore '**/*.sum' \
	-ignore '**/*.txt' \
	-ignore '**/*.pem' \
	-ignore '**/*.yaml' \
	-ignore 'bin/**' \
	-ignore 'certs/**' \
	-ignore '.ai/**' \
	-ignore '.claude/**' \
	-ignore 'admin/ui/**'

.PHONY: license-check
license-check: ## Check that all source files have copyright headers
	@if [ -x "$(ADDLICENSE)" ]; then \
		$(ADDLICENSE) -check $(ADDLICENSE_FLAGS) .; \
	else \
		echo "addlicense not installed. Run: make tools"; \
		exit 1; \
	fi

.PHONY: license-fix
license-fix: ## Add missing copyright headers to source files
	@if [ -x "$(ADDLICENSE)" ]; then \
		$(ADDLICENSE) $(ADDLICENSE_FLAGS) .; \
	else \
		echo "addlicense not installed. Run: make tools"; \
		exit 1; \
	fi

# ============================================================================
# Development Tools
# ============================================================================

# Tool versions to install (keep in sync with CI workflows)
GOLANGCI_LINT_VERSION := v2.8.0
GOSEC_VERSION := v2.23.0
GOVULNCHECK_VERSION := v1.1.4

.PHONY: tools
tools: ## Install development tools
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Installing addlicense..."
	go install github.com/google/addlicense@latest
	@echo "Installing gosec $(GOSEC_VERSION)..."
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	@echo "Installing govulncheck $(GOVULNCHECK_VERSION)..."
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@echo "Installing benchstat..."
	go install golang.org/x/perf/cmd/benchstat@latest

# ============================================================================
# Load Testing (k6)
# ============================================================================

.PHONY: check-k6
check-k6:
	@command -v k6 >/dev/null 2>&1 || { \
		echo "k6 is not installed. Install with:"; \
		echo "  brew install k6       # macOS"; \
		echo "  go install go.k6.io/k6@latest  # From source"; \
		exit 1; \
	}

.PHONY: load-target-start
load-target-start: ## Start the target echo server for load testing (background)
	@if [ -f .target-server.pid ] && kill -0 $$(cat .target-server.pid) 2>/dev/null; then \
		echo "Target server already running on :9999 (PID $$(cat .target-server.pid))"; \
	else \
		echo "Starting target server on :9999..."; \
		go run test/load/targetserver/main.go & echo $$! > .target-server.pid; \
		sleep 1; \
	fi

.PHONY: load-target-stop
load-target-stop: ## Stop the target echo server
	@if [ -f .target-server.pid ]; then \
		kill $$(cat .target-server.pid) 2>/dev/null && echo "Target server stopped" || echo "Target server not running"; \
		rm -f .target-server.pid; \
	else \
		echo "No PID file found — target server not managed by make"; \
	fi

.PHONY: load-test
load-test: load-baseline ## Run load tests (alias for load-baseline)

.PHONY: load-baseline
load-baseline: check-k6 gencerts-load load-target-start ## Run baseline load test (~6 min, 50 VUs)
	@echo "Running baseline load test..."
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/baseline.js

.PHONY: load-spike
load-spike: check-k6 gencerts-load load-target-start ## Run spike test (~5 min, 1000 VUs peak)
	@echo "Running spike test..."
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/spike.js

.PHONY: load-stress
load-stress: check-k6 gencerts-load load-target-start ## Run stress test (~17 min, 3000 VUs max)
	@echo "Running stress test (this takes ~17 minutes)..."
	@CURRENT_FD=$$(ulimit -n); if [ "$$CURRENT_FD" -lt 250000 ] 2>/dev/null; then \
		echo "WARNING: fd limit is $$CURRENT_FD (k6 recommends 250000 for high-VU tests)"; \
		echo "  Run: ulimit -n 250000"; \
		echo "  See: https://grafana.com/docs/k6/latest/set-up/fine-tune-os/"; \
	fi
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/stress.js

.PHONY: load-soak
load-soak: check-k6 gencerts-load load-target-start ## Run soak test (4+ hours, 200 VUs)
	@echo "WARNING: Soak test runs for 4+ hours"
	@printf "Continue? [y/N] " && read confirm && [ "$$confirm" = "y" ]
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/soak.js

.PHONY: load-mtls
load-mtls: check-k6 gencerts-load load-target-start ## Run mTLS load test (~7 min, 100 VUs)
	@echo "Running mTLS load test..."
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/mtls.js

.PHONY: load-smoke
load-smoke: check-k6 gencerts-load load-target-start ## Run smoke test (1 min quick validation, overrides baseline stages)
	@echo "Running smoke test (quick validation)..."
	@mkdir -p test/load/results
	K6_INSECURE_SKIP_TLS_VERIFY=true k6 run --vus 10 --duration 1m -e K6_SCENARIO=smoke test/load/baseline.js

.PHONY: gencerts-load
gencerts-load: ## Copy certificates for load testing (run make gencerts first)
	@if [ ! -f certs/client.crt ] || [ ! -f certs/client.key ] || [ ! -f certs/ca.crt ]; then \
		echo "Error: certificates not found. Run 'make gencerts' first."; \
		exit 1; \
	fi
	@echo "Copying certificates for load testing..."
	@mkdir -p test/load/certs
	@cp certs/client.crt certs/client.key certs/ca.crt test/load/certs/
	@echo "Certificates copied to test/load/certs/"

.PHONY: load-baseline-remote
load-baseline-remote: check-k6 ## Run baseline against remote (requires PROXY_URL)
	@if [ -z "$(PROXY_URL)" ]; then \
		echo "Error: PROXY_URL not set. Usage: make load-baseline-remote PROXY_URL=https://staging:8443"; \
		exit 1; \
	fi
	@mkdir -p test/load/results
	k6 run -e PROXY_URL="$(PROXY_URL)" $(if $(TARGET_URL),-e TARGET_URL="$(TARGET_URL)") test/load/baseline.js

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
