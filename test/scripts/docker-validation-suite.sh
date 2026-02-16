#!/usr/bin/env bash
# Copyright 2026 CloudBlue LLC
# SPDX-License-Identifier: Apache-2.0

# Docker Validation Suite for Chaperone
#
# Comprehensive integration tests run against Docker containers.
# Validates proxy round-trip, security, telemetry, and operational behavior.
#
# Usage:
#   ./test/scripts/docker-validation-suite.sh \
#       --proxy-image chaperone:poc-test \
#       --prod-image  chaperone:poc \
#       --echo-image  echoserver:test \
#       --config      configs/docker-test.yaml \
#       --credentials test/testdata/docker-test-credentials.json
#
# Prerequisites: docker, curl, jq

set -euo pipefail

# =============================================================================
# Configuration (from flags)
# =============================================================================

PROXY_IMAGE=""
PROD_IMAGE=""
ECHO_IMAGE=""
CONFIG_PATH=""
CREDENTIALS_PATH=""

# Container/network names (unique per invocation to allow parallel runs)
UNIQUE_ID="$$"
DOCKER_NET="chaperone-test-${UNIQUE_ID}"
ECHO_CONTAINER="echoserver-${UNIQUE_ID}"
PROXY_CONTAINER="chaperone-docker-test-${UNIQUE_ID}"
PROD_CONTAINER="chaperone-prod-test-${UNIQUE_ID}"
SHUTDOWN_CONTAINER="chaperone-shutdown-test-${UNIQUE_ID}"

# Ports
PROXY_PORT=18443
ADMIN_PORT=19090

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# =============================================================================
# Argument parsing
# =============================================================================

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --proxy-image)  PROXY_IMAGE="$2";      shift 2 ;;
            --prod-image)   PROD_IMAGE="$2";        shift 2 ;;
            --echo-image)   ECHO_IMAGE="$2";        shift 2 ;;
            --config)       CONFIG_PATH="$2";       shift 2 ;;
            --credentials)  CREDENTIALS_PATH="$2";  shift 2 ;;
            *)
                echo "Unknown argument: $1" >&2
                exit 1
                ;;
        esac
    done

    local missing=()
    [[ -z "$PROXY_IMAGE" ]]      && missing+=("--proxy-image")
    [[ -z "$PROD_IMAGE" ]]       && missing+=("--prod-image")
    [[ -z "$ECHO_IMAGE" ]]       && missing+=("--echo-image")
    [[ -z "$CONFIG_PATH" ]]      && missing+=("--config")
    [[ -z "$CREDENTIALS_PATH" ]] && missing+=("--credentials")

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Missing required arguments: ${missing[*]}" >&2
        exit 1
    fi
}

# =============================================================================
# Prerequisites
# =============================================================================

check_prerequisites() {
    local missing=()
    for cmd in docker curl jq; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Missing required tools: ${missing[*]}" >&2
        echo "Install them before running this script." >&2
        exit 1
    fi
}

# =============================================================================
# Helpers
# =============================================================================

pass() {
    echo "   ✓ $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
    echo "   ❌ $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

# Redact sensitive fields from JSON output before printing.
# Prevents credential values from appearing in CI logs.
redact_sensitive() {
    sed -E 's/"(Authorization|Proxy-Authorization|X-API-Key|X-Auth-Token|Cookie)":"[^"]*"/"\1":"[REDACTED]"/g'
}

# Wait for an HTTP endpoint to become available.
# Usage: wait_for_health <url> <label> <max_attempts>
wait_for_health() {
    local url="$1"
    local label="$2"
    local max_attempts="${3:-30}"

    for i in $(seq 1 "$max_attempts"); do
        if curl -sf "$url" > /dev/null 2>&1; then
            return 0
        fi
        if [[ "$i" == "$max_attempts" ]]; then
            echo "   ❌ $label failed to start within $((max_attempts / 2))s" >&2
            return 1
        fi
        sleep 0.5
    done
}

# =============================================================================
# Setup & Teardown
# =============================================================================

cleanup() {
    echo ""
    echo "Cleaning up..."
    docker stop "$PROXY_CONTAINER"    2>/dev/null || true
    docker stop "$ECHO_CONTAINER"     2>/dev/null || true
    docker stop "$PROD_CONTAINER"     2>/dev/null || true
    docker stop "$SHUTDOWN_CONTAINER" 2>/dev/null || true
    docker rm   "$SHUTDOWN_CONTAINER" 2>/dev/null || true
    docker network rm "$DOCKER_NET"   2>/dev/null || true
}

setup_network() {
    echo "1. Creating test network..."
    docker network create "$DOCKER_NET" > /dev/null
    pass "Network $DOCKER_NET created"
}

start_echo_server() {
    echo "2. Starting echo server..."
    docker run -d --rm \
        --name "$ECHO_CONTAINER" \
        --network "$DOCKER_NET" \
        --network-alias echoserver \
        "$ECHO_IMAGE" > /dev/null
    pass "Echo server running as echoserver:8080"
}

start_proxy() {
    echo "3. Starting Chaperone proxy..."
    docker run -d --rm \
        --name "$PROXY_CONTAINER" \
        --network "$DOCKER_NET" \
        -p "${PROXY_PORT}:8443" \
        -p "${ADMIN_PORT}:9090" \
        -v "${CONFIG_PATH}:/app/config.yaml:ro" \
        -v "${CREDENTIALS_PATH}:/app/credentials.json:ro" \
        "$PROXY_IMAGE" \
        -config /app/config.yaml -credentials /app/credentials.json > /dev/null

    echo "4. Waiting for proxy to be ready..."
    if ! wait_for_health "http://localhost:${PROXY_PORT}/_ops/health" "Proxy" 30; then
        docker logs "$PROXY_CONTAINER" 2>&1 || true
        return 1
    fi
    pass "Health endpoint returns 200"

    echo "5. Version check..."
    if ! curl -sf "http://localhost:${PROXY_PORT}/_ops/version" > /dev/null; then
        fail "Version check failed!"
        return 1
    fi
    pass "Version endpoint returns 200"
}

# =============================================================================
# Test Groups
# =============================================================================

check_proxy_roundtrip() {
    echo ""
    echo "--- Proxy Round-Trip Tests ---"

    # Test 6: Bearer credential injection
    echo "6. Sending proxy request (bearer credential injection)..."
    local response=""
    for i in $(seq 1 10); do
        response=$(curl -sf "http://localhost:${PROXY_PORT}/proxy" \
            -H "X-Connect-Target-URL: http://echoserver:8080/test-path" \
            -H "X-Connect-Vendor-ID: docker-test-vendor" \
            -H "X-Connect-Marketplace-ID: test-mp" \
        ) && break
        if [[ "$i" == "10" ]]; then
            fail "Proxy request failed after 10 retries!"
            docker logs "$PROXY_CONTAINER" 2>&1 || true
            return 1
        fi
        sleep 0.5
    done
    pass "Proxy returned 200"

    # Test 7: Credential injection validation
    echo "7. Validating credential injection..."
    local auth_header
    auth_header=$(echo "$response" | jq -r '.headers.Authorization // empty')
    if [[ "$auth_header" == "Bearer docker-test-token-42" ]]; then
        pass "Authorization: Bearer header injected correctly"
    else
        fail "Expected Authorization: Bearer docker-test-token-42"
        echo "   Got: $(echo "$response" | redact_sensitive)"
        return 1
    fi

    # Test 8: Request path forwarding
    echo "8. Validating request path forwarded..."
    local path
    path=$(echo "$response" | jq -r '.path // empty')
    if [[ "$path" == "/test-path" ]]; then
        pass "Request path /test-path forwarded correctly"
    else
        fail "Expected path /test-path in echo response"
        echo "   Got: $(echo "$response" | redact_sensitive)"
        return 1
    fi

    # Test 9: HTTP method passthrough
    echo "9. Validating HTTP method passthrough..."
    local method_response
    method_response=$(curl -sf -X POST "http://localhost:${PROXY_PORT}/proxy" \
        -H "X-Connect-Target-URL: http://echoserver:8080/method-check" \
        -H "X-Connect-Vendor-ID: docker-test-vendor" \
        -d '{"test": true}' \
    ) || {
        fail "POST proxy request failed!"
        return 1
    }
    local method
    method=$(echo "$method_response" | jq -r '.method // empty')
    if [[ "$method" == "POST" ]]; then
        pass "HTTP method POST forwarded correctly"
    else
        fail "Expected method POST in echo response"
        echo "   Got: $(echo "$method_response" | redact_sensitive)"
        return 1
    fi
}

check_security() {
    echo ""
    echo "--- Security & Compliance ---"

    # Test 10: Non-root user
    echo "10. Verifying non-root user..."
    local user
    user=$(docker inspect "$PROXY_IMAGE" --format '{{.Config.User}}')
    if [[ "$user" == "nonroot:nonroot" ]]; then
        pass "Running as nonroot:nonroot"
    else
        fail "Not running as non-root (found: $user)"
        return 1
    fi

    # Test 11: Distroless base (no shell)
    echo "11. Verifying distroless base (no shell)..."
    if ! docker run --rm --entrypoint /bin/sh "$PROXY_IMAGE" -c "exit 0" 2>/dev/null; then
        pass "No shell available (distroless confirmed)"
    else
        fail "Image has shell - not distroless!"
        return 1
    fi

    # Test 12: Image size
    echo "12. Verifying image size..."
    local size_raw size_num size_unit
    size_raw=$(docker images "$PROXY_IMAGE" --format '{{.Size}}')
    size_num=$(echo "$size_raw" | grep -oE '[0-9.]+')
    size_unit=$(echo "$size_raw" | grep -oE '[A-Za-z]+')
    if [[ "$size_unit" == "MB" ]] && [[ "${size_num%.*}" -lt 50 ]]; then
        pass "Image size: $size_raw (< 50MB target)"
    elif [[ "$size_unit" == "KB" ]] || [[ "$size_unit" == "kB" ]]; then
        pass "Image size: $size_raw (< 50MB target)"
    else
        fail "Image too large: $size_raw (target: < 50MB)"
        return 1
    fi
}

check_telemetry() {
    echo ""
    echo "--- Telemetry ---"

    # Test 13: Prometheus metrics endpoint
    echo "13. Verifying Prometheus metrics endpoint..."
    local metrics_ct
    metrics_ct=$(curl -sf -o /dev/null -w '%{content_type}' "http://localhost:${ADMIN_PORT}/metrics")
    if ! echo "$metrics_ct" | grep -q 'text/plain'; then
        fail "Expected text/plain content type, got: $metrics_ct"
        return 1
    fi

    local metrics_body
    metrics_body=$(curl -sf "http://localhost:${ADMIN_PORT}/metrics")

    if echo "$metrics_body" | grep -q '^# HELP chaperone_requests_total'; then
        pass "/metrics returns valid Prometheus format with chaperone_requests_total"
    else
        fail "chaperone_requests_total not found in /metrics output"
        echo "   Got (first 5 lines):"
        echo "$metrics_body" | head -5
        return 1
    fi

    if echo "$metrics_body" | grep -q '^chaperone_requests_total{'; then
        pass "chaperone_requests_total counter has observations (proxy traffic recorded)"
    else
        echo "   ⚠  chaperone_requests_total has no observations yet (metrics registered but no matching labels)"
    fi

    if echo "$metrics_body" | grep -q '^# HELP chaperone_request_duration_seconds'; then
        pass "chaperone_request_duration_seconds histogram registered"
    else
        fail "chaperone_request_duration_seconds not found in /metrics output"
        return 1
    fi
}

check_request_validation() {
    echo ""
    echo "--- Request Validation ---"

    # Test 14: Missing target URL returns 400
    echo "14. Verifying missing target URL returns 400..."
    local missing_status
    missing_status=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${PROXY_PORT}/proxy" \
        -H "X-Connect-Vendor-ID: docker-test-vendor" \
    )
    if [[ "$missing_status" == "400" ]]; then
        pass "Missing X-Connect-Target-URL returns 400 Bad Request"
    else
        fail "Expected 400, got $missing_status"
        return 1
    fi

    # Test 15: Blocked host returns 403
    echo "15. Verifying blocked host returns 403..."
    local blocked_status
    blocked_status=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${PROXY_PORT}/proxy" \
        -H "X-Connect-Target-URL: http://evil.example.com/steal-data" \
        -H "X-Connect-Vendor-ID: docker-test-vendor" \
    )
    if [[ "$blocked_status" == "403" ]]; then
        pass "Blocked host returns 403 Forbidden"
    else
        fail "Expected 403, got $blocked_status"
        return 1
    fi
}

check_production_defaults() {
    echo ""
    echo "--- Production Secure Defaults ---"

    # Test 16: Production image rejects insecure HTTP targets
    echo "16. Verifying production image rejects insecure HTTP targets..."

    local http_status
    http_status=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${PROXY_PORT}/proxy" \
        -H "X-Connect-Target-URL: http://echoserver:8080/should-fail" \
        -H "X-Connect-Vendor-ID: docker-test-vendor" \
    )
    echo "   Test image allows HTTP (status: $http_status) - expected for test build"

    # Stop test proxy, start production image
    docker stop "$PROXY_CONTAINER" > /dev/null 2>&1 || true

    echo "   Starting production image (ALLOW_INSECURE_TARGETS=false)..."
    docker run -d --rm \
        --name "$PROD_CONTAINER" \
        --network "$DOCKER_NET" \
        -p "${PROXY_PORT}:8443" \
        -v "${CONFIG_PATH}:/app/config.yaml:ro" \
        -v "${CREDENTIALS_PATH}:/app/credentials.json:ro" \
        "$PROD_IMAGE" \
        -config /app/config.yaml -credentials /app/credentials.json > /dev/null

    if ! wait_for_health "http://localhost:${PROXY_PORT}/_ops/health" "Production proxy" 30; then
        docker logs "$PROD_CONTAINER" 2>&1 || true
        docker stop "$PROD_CONTAINER" 2>/dev/null || true
        fail "Production proxy failed to start"
        return 1
    fi

    local prod_http_status
    prod_http_status=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${PROXY_PORT}/proxy" \
        -H "X-Connect-Target-URL: http://echoserver:8080/should-reject" \
        -H "X-Connect-Vendor-ID: docker-test-vendor" \
    )
    docker stop "$PROD_CONTAINER" > /dev/null 2>&1 || true

    if [[ "$prod_http_status" == "400" ]]; then
        pass "Production image rejects HTTP targets (400 Bad Request)"
    else
        fail "Expected 400, got $prod_http_status"
        echo "   Production image is NOT rejecting insecure targets!"
        return 1
    fi
}

check_operations() {
    echo ""
    echo "--- Operational Behavior ---"

    # Test 17: Graceful shutdown (SIGTERM → exit 0)
    echo "17. Verifying graceful shutdown (SIGTERM → exit 0)..."
    docker run -d \
        --name "$SHUTDOWN_CONTAINER" \
        --network "$DOCKER_NET" \
        -v "${CONFIG_PATH}:/app/config.yaml:ro" \
        -v "${CREDENTIALS_PATH}:/app/credentials.json:ro" \
        "$PROXY_IMAGE" \
        -config /app/config.yaml -credentials /app/credentials.json > /dev/null
    sleep 1

    docker stop --time 5 "$SHUTDOWN_CONTAINER" > /dev/null 2>&1

    local exit_code
    exit_code=$(docker inspect "$SHUTDOWN_CONTAINER" --format '{{.State.ExitCode}}' 2>/dev/null || echo "unknown")
    docker rm "$SHUTDOWN_CONTAINER" > /dev/null 2>&1 || true

    if [[ "$exit_code" == "0" ]]; then
        pass "Container exited cleanly after SIGTERM (exit 0)"
    else
        fail "Expected exit 0, got $exit_code"
        return 1
    fi

    # Test 18: Bad config rejection
    echo "18. Verifying bad config rejection..."
    local bad_config_path
    bad_config_path="$(cd "$(dirname "$CREDENTIALS_PATH")" && pwd)/bad-config.yaml"

    local bad_exit=0
    docker run --rm \
        -v "${bad_config_path}:/app/config.yaml:ro" \
        "$PROXY_IMAGE" \
        -config /app/config.yaml 2>&1 || bad_exit=$?

    if [[ "$bad_exit" != "0" ]]; then
        pass "Malformed config rejected (exit code $bad_exit)"
    else
        fail "Expected non-zero exit for bad config, got 0"
        return 1
    fi
}

# =============================================================================
# Main
# =============================================================================

main() {
    parse_args "$@"
    check_prerequisites

    trap cleanup EXIT

    echo "=== Docker Validation Suite ==="
    echo ""

    # Setup
    setup_network
    start_echo_server
    start_proxy

    # Test groups
    check_proxy_roundtrip
    check_security
    check_telemetry
    check_request_validation
    check_production_defaults
    check_operations

    # Summary
    echo ""
    echo "=== Docker Validation Complete ==="
    echo "   Passed: $TESTS_PASSED"
    if [[ "$TESTS_FAILED" -gt 0 ]]; then
        echo "   Failed: $TESTS_FAILED"
        exit 1
    fi
    echo "   All checks passed! ✓"
}

main "$@"
