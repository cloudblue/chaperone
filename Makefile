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

.PHONY: docker-build
docker-build: ## Build Docker image (production: HTTPS only, no test tools)
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG) (production)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: docker-run
docker-run: ## Run Docker container (HTTP mode for testing)
	@echo "Running $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@echo "  Config: /app/config.yaml (TLS disabled, minimal allow_list)"
	@echo "  Override: -v /path/config.yaml:/app/config.yaml or -e CHAPERONE_*"
	docker run --rm -p 8443:8443 --name chaperone-test $(DOCKER_IMAGE):$(DOCKER_TAG)

DOCKER_TEST_TAG ?= $(DOCKER_TAG)-test
ECHOSERVER_IMAGE := echoserver
ECHOSERVER_TAG ?= test

.PHONY: docker-test
docker-test: ## Build and test Docker image (comprehensive validation)
	@# Build Chaperone from production Dockerfile with HTTP targets allowed
	@echo "Building Chaperone test image (production Dockerfile + ALLOW_INSECURE_TARGETS)..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg ALLOW_INSECURE_TARGETS=true \
		-t $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) .
	@# Build production image (default: ALLOW_INSECURE_TARGETS=false)
	@echo "Building Chaperone production image (HTTPS-only)..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@# Build echoserver from its own Dockerfile
	@echo "Building echoserver image..."
	@docker build \
		-f test/Dockerfile.echoserver \
		-t $(ECHOSERVER_IMAGE):$(ECHOSERVER_TAG) .
	@echo "=== Docker Validation Suite ==="
	@echo ""
	@# --- Setup: Create isolated Docker network ---
	@DOCKER_NET="chaperone-test-$$$$"; \
	ECHO_CONTAINER="echoserver-$$$$"; \
	PROXY_CONTAINER="chaperone-docker-test-$$$$"; \
	cleanup() { \
		echo ""; \
		echo "Cleaning up..."; \
		docker stop "$$PROXY_CONTAINER" 2>/dev/null || true; \
		docker stop "$$ECHO_CONTAINER" 2>/dev/null || true; \
		docker stop chaperone-prod-test-$$$$ 2>/dev/null || true; \
		docker rm chaperone-shutdown-test-$$$$ 2>/dev/null || true; \
		docker network rm "$$DOCKER_NET" 2>/dev/null || true; \
	}; \
	trap cleanup EXIT; \
	\
	echo "1. Creating test network..."; \
	docker network create "$$DOCKER_NET" > /dev/null; \
	echo "   ✓ Network $$DOCKER_NET created"; \
	\
	echo "2. Starting echo server..."; \
	docker run -d --rm \
		--name "$$ECHO_CONTAINER" \
		--network "$$DOCKER_NET" \
		--network-alias echoserver \
		$(ECHOSERVER_IMAGE):$(ECHOSERVER_TAG) > /dev/null; \
	echo "   ✓ Echo server running as echoserver:8080"; \
	\
	echo "3. Starting Chaperone proxy..."; \
	docker run -d --rm \
		--name "$$PROXY_CONTAINER" \
		--network "$$DOCKER_NET" \
		-p 18443:8443 \
		-p 19090:9090 \
		-v "$(CURDIR)/configs/docker-test.yaml:/app/config.yaml:ro" \
		-v "$(CURDIR)/test/testdata/docker-test-credentials.json:/app/credentials.json:ro" \
		$(DOCKER_IMAGE):$(DOCKER_TEST_TAG) \
		-config /app/config.yaml -credentials /app/credentials.json > /dev/null; \
	\
	echo "4. Waiting for proxy to be ready..."; \
	for i in $$(seq 1 30); do \
		if curl -sf http://localhost:18443/_ops/health > /dev/null 2>&1; then \
			break; \
		fi; \
		if [ "$$i" = "30" ]; then \
			echo "   ❌ Proxy failed to start within 15s"; \
			docker logs "$$PROXY_CONTAINER"; \
			exit 1; \
		fi; \
		sleep 0.5; \
	done; \
	echo "   ✓ Health endpoint returns 200"; \
	\
	echo "5. Version check..."; \
	curl -sf http://localhost:18443/_ops/version > /dev/null || { \
		echo "   ❌ Version check failed!"; \
		exit 1; \
	}; \
	echo "   ✓ Version endpoint returns 200"; \
	\
	echo ""; \
	echo "--- Proxy Round-Trip Test ---"; \
	echo "6. Sending proxy request (bearer credential injection)..."; \
	for i in $$(seq 1 10); do \
		ECHO_RESPONSE=$$(curl -sf http://localhost:18443/proxy \
			-H "X-Connect-Target-URL: http://echoserver:8080/test-path" \
			-H "X-Connect-Vendor-ID: docker-test-vendor" \
			-H "X-Connect-Marketplace-ID: test-mp" \
		) && break; \
		if [ "$$i" = "10" ]; then \
			echo "   ❌ Proxy request failed after 10 retries!"; \
			docker logs "$$PROXY_CONTAINER"; \
			exit 1; \
		fi; \
		sleep 0.5; \
	done; \
	echo "   ✓ Proxy returned 200"; \
	\
	echo "7. Validating credential injection..."; \
	AUTH_HEADER=$$(echo "$$ECHO_RESPONSE" | grep -o '"Authorization":"[^"]*"' | head -1); \
	if echo "$$AUTH_HEADER" | grep -q "Bearer docker-test-token-42"; then \
		echo "   ✓ Authorization: Bearer header injected correctly"; \
	else \
		echo "   ❌ Expected Authorization: Bearer docker-test-token-42"; \
		echo "   Got: $$AUTH_HEADER"; \
		echo "   Full response: $$ECHO_RESPONSE"; \
		exit 1; \
	fi; \
	\
	echo "8. Validating request path forwarded..."; \
	if echo "$$ECHO_RESPONSE" | grep -q '"path":"/test-path"'; then \
		echo "   ✓ Request path /test-path forwarded correctly"; \
	else \
		echo "   ❌ Expected path /test-path in echo response"; \
		echo "   Got: $$ECHO_RESPONSE"; \
		exit 1; \
	fi; \
	\
	echo "9. Validating HTTP method passthrough..."; \
	METHOD_RESPONSE=$$(curl -sf -X POST http://localhost:18443/proxy \
		-H "X-Connect-Target-URL: http://echoserver:8080/method-check" \
		-H "X-Connect-Vendor-ID: docker-test-vendor" \
		-d '{"test": true}' \
	) || { \
		echo "   ❌ POST proxy request failed!"; \
		exit 1; \
	}; \
	if echo "$$METHOD_RESPONSE" | grep -q '"method":"POST"'; then \
		echo "   ✓ HTTP method POST forwarded correctly"; \
	else \
		echo "   ❌ Expected method POST in echo response"; \
		echo "   Got: $$METHOD_RESPONSE"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "--- Security & Compliance ---"; \
	echo "10. Verifying non-root user..."; \
	USER=$$(docker inspect $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) --format '{{.Config.User}}'); \
	if [ "$$USER" = "nonroot:nonroot" ]; then \
		echo "   ✓ Running as nonroot:nonroot"; \
	else \
		echo "   ❌ Not running as non-root (found: $$USER)"; \
		exit 1; \
	fi; \
	\
	echo "11. Verifying distroless base (no shell)..."; \
	if ! docker run --rm --entrypoint /bin/sh $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) -c "exit 0" 2>/dev/null; then \
		echo "   ✓ No shell available (distroless confirmed)"; \
	else \
		echo "   ❌ Image has shell - not distroless!"; \
		exit 1; \
	fi; \
	\
	echo "12. Verifying image size..."; \
	SIZE_RAW=$$(docker images $(DOCKER_IMAGE):$(DOCKER_TEST_TAG) --format '{{.Size}}'); \
	SIZE_NUM=$$(echo "$$SIZE_RAW" | grep -oE '[0-9.]+'); \
	SIZE_UNIT=$$(echo "$$SIZE_RAW" | grep -oE '[A-Za-z]+'); \
	if [ "$$SIZE_UNIT" = "MB" ] && [ "$${SIZE_NUM%.*}" -lt 50 ]; then \
		echo "   ✓ Image size: $$SIZE_RAW (< 50MB target)"; \
	elif [ "$$SIZE_UNIT" = "KB" ] || [ "$$SIZE_UNIT" = "kB" ]; then \
		echo "   ✓ Image size: $$SIZE_RAW (< 50MB target)"; \
	else \
		echo "   ❌ Image too large: $$SIZE_RAW (target: < 50MB)"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "--- Telemetry ---"; \
	echo "13. Verifying Prometheus metrics endpoint..."; \
	METRICS_CT=$$(curl -sf -o /dev/null -w '%{content_type}' http://localhost:19090/metrics); \
	if ! echo "$$METRICS_CT" | grep -q 'text/plain'; then \
		echo "   ❌ Expected text/plain content type, got: $$METRICS_CT"; \
		exit 1; \
	fi; \
	METRICS_BODY=$$(curl -sf http://localhost:19090/metrics); \
	if echo "$$METRICS_BODY" | grep -q '^# HELP chaperone_requests_total'; then \
		echo "   ✓ /metrics returns valid Prometheus format with chaperone_requests_total"; \
	else \
		echo "   ❌ chaperone_requests_total not found in /metrics output"; \
		echo "   Got (first 5 lines):"; \
		echo "$$METRICS_BODY" | head -5; \
		exit 1; \
	fi; \
	if echo "$$METRICS_BODY" | grep -q '^chaperone_requests_total{'; then \
		echo "   ✓ chaperone_requests_total counter has observations (proxy traffic recorded)"; \
	else \
		echo "   ⚠  chaperone_requests_total has no observations yet (metrics registered but no matching labels)"; \
	fi; \
	if echo "$$METRICS_BODY" | grep -q '^# HELP chaperone_request_duration_seconds'; then \
		echo "   ✓ chaperone_request_duration_seconds histogram registered"; \
	else \
		echo "   ❌ chaperone_request_duration_seconds not found in /metrics output"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "--- Request Validation ---"; \
	echo "14. Verifying missing target URL returns 400..."; \
	MISSING_STATUS=$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:18443/proxy \
		-H "X-Connect-Vendor-ID: docker-test-vendor" \
	); \
	if [ "$$MISSING_STATUS" = "400" ]; then \
		echo "   ✓ Missing X-Connect-Target-URL returns 400 Bad Request"; \
	else \
		echo "   ❌ Expected 400, got $$MISSING_STATUS"; \
		exit 1; \
	fi; \
	\
	echo "15. Verifying blocked host returns 403..."; \
	BLOCKED_STATUS=$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:18443/proxy \
		-H "X-Connect-Target-URL: http://evil.example.com/steal-data" \
		-H "X-Connect-Vendor-ID: docker-test-vendor" \
	); \
	if [ "$$BLOCKED_STATUS" = "403" ]; then \
		echo "   ✓ Blocked host returns 403 Forbidden"; \
	else \
		echo "   ❌ Expected 403, got $$BLOCKED_STATUS"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "--- Production Secure Defaults ---"; \
	echo "16. Verifying production image rejects insecure HTTP targets..."; \
	HTTP_STATUS=$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:18443/proxy \
		-H "X-Connect-Target-URL: http://echoserver:8080/should-fail" \
		-H "X-Connect-Vendor-ID: docker-test-vendor" \
	); \
	echo "   Test image allows HTTP (status: $$HTTP_STATUS) - expected for test build"; \
	docker stop "$$PROXY_CONTAINER" > /dev/null 2>&1 || true; \
	echo "   Starting production image (ALLOW_INSECURE_TARGETS=false)..."; \
	PROD_CONTAINER="chaperone-prod-test-$$$$"; \
	docker run -d --rm \
		--name "$$PROD_CONTAINER" \
		--network "$$DOCKER_NET" \
		-p 18443:8443 \
		-v "$(CURDIR)/configs/docker-test.yaml:/app/config.yaml:ro" \
		-v "$(CURDIR)/test/testdata/docker-test-credentials.json:/app/credentials.json:ro" \
		$(DOCKER_IMAGE):$(DOCKER_TAG) \
		-config /app/config.yaml -credentials /app/credentials.json > /dev/null; \
	for i in $$(seq 1 30); do \
		if curl -sf http://localhost:18443/_ops/health > /dev/null 2>&1; then \
			break; \
		fi; \
		if [ "$$i" = "30" ]; then \
			echo "   ❌ Production proxy failed to start"; \
			docker logs "$$PROD_CONTAINER" 2>&1 || true; \
			docker stop "$$PROD_CONTAINER" 2>/dev/null || true; \
			exit 1; \
		fi; \
		sleep 0.5; \
	done; \
	PROD_HTTP_STATUS=$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:18443/proxy \
		-H "X-Connect-Target-URL: http://echoserver:8080/should-reject" \
		-H "X-Connect-Vendor-ID: docker-test-vendor" \
	); \
	docker stop "$$PROD_CONTAINER" > /dev/null 2>&1 || true; \
	if [ "$$PROD_HTTP_STATUS" = "400" ]; then \
		echo "   ✓ Production image rejects HTTP targets (400 Bad Request)"; \
	else \
		echo "   ❌ Expected 400, got $$PROD_HTTP_STATUS"; \
		echo "   Production image is NOT rejecting insecure targets!"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "--- Operational Behavior ---"; \
	echo "17. Verifying graceful shutdown (SIGTERM → exit 0)..."; \
	SHUTDOWN_CONTAINER="chaperone-shutdown-test-$$$$"; \
	docker run -d \
		--name "$$SHUTDOWN_CONTAINER" \
		--network "$$DOCKER_NET" \
		-v "$(CURDIR)/configs/docker-test.yaml:/app/config.yaml:ro" \
		-v "$(CURDIR)/test/testdata/docker-test-credentials.json:/app/credentials.json:ro" \
		$(DOCKER_IMAGE):$(DOCKER_TEST_TAG) \
		-config /app/config.yaml -credentials /app/credentials.json > /dev/null; \
	sleep 1; \
	docker stop --time 5 "$$SHUTDOWN_CONTAINER" > /dev/null 2>&1; \
	EXIT_CODE=$$(docker inspect "$$SHUTDOWN_CONTAINER" --format '{{.State.ExitCode}}' 2>/dev/null || echo "unknown"); \
	docker rm "$$SHUTDOWN_CONTAINER" > /dev/null 2>&1 || true; \
	if [ "$$EXIT_CODE" = "0" ]; then \
		echo "   ✓ Container exited cleanly after SIGTERM (exit 0)"; \
	else \
		echo "   ❌ Expected exit 0, got $$EXIT_CODE"; \
		exit 1; \
	fi; \
	\
	echo "18. Verifying bad config rejection..."; \
	BAD_CONFIG_OUTPUT=$$(docker run --rm \
		-v "$(CURDIR)/test/testdata/bad-config.yaml:/app/config.yaml:ro" \
		$(DOCKER_IMAGE):$(DOCKER_TEST_TAG) \
		-config /app/config.yaml 2>&1); \
	BAD_EXIT=$$?; \
	if [ "$$BAD_EXIT" != "0" ]; then \
		echo "   ✓ Malformed config rejected (exit code $$BAD_EXIT)"; \
	else \
		echo "   ❌ Expected non-zero exit for bad config, got 0"; \
		echo "   Output: $$BAD_CONFIG_OUTPUT"; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "=== Docker Validation Passed! ✓ ==="

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
