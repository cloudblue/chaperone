# Copyright 2026 CloudBlue LLC
# SPDX-License-Identifier: Apache-2.0

# Chaperone Egress Proxy - Multi-Stage Dockerfile
# See: docs/DESIGN-SPECIFICATION.md Section 6.2.1

# =============================================================================
# Stage 1: Builder
# =============================================================================
FROM golang:1.25-alpine AS builder

# Install git for version info (optional) and ca-certificates
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependencies first (for better layer caching)
# Copy both modules' dependency files
# Note: go.sum may not exist if no external dependencies
COPY go.mod ./
COPY sdk/go.mod ./sdk/

# Download dependencies (SDK has no external deps currently)
RUN go mod download

# Copy source code
COPY . .

# Build static binary
# CGO_ENABLED=0 ensures static linking (required for distroless)
# -ldflags "-s -w" strips debug info for smaller binary
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
        -X main.Version=${VERSION} \
        -X main.GitCommit=${GIT_COMMIT} \
        -X main.BuildDate=${BUILD_DATE}" \
    -o chaperone ./cmd/chaperone

# =============================================================================
# Stage 2: Runtime (Distroless)
# =============================================================================
# Using distroless/static for minimal attack surface:
# - No shell (prevents container escape techniques)
# - No package manager
# - Only the binary and CA certificates
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.title="Chaperone"
LABEL org.opencontainers.image.description="Secure egress proxy for credential injection"
LABEL org.opencontainers.image.source="https://github.com/cloudblue/chaperone"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /app

# Copy the statically-linked binary
COPY --from=builder /build/chaperone /app/chaperone

# Distroless nonroot image runs as UID 65532 by default
# This is a security best practice
USER nonroot:nonroot

# Expose ports:
# - 8443: Traffic port (mTLS)
# - 9090: Admin/metrics port (future)
EXPOSE 8443 9090

# Note: HEALTHCHECK directive not used because distroless has no shell/curl/wget.
# Health checking options:
#   - Kubernetes: livenessProbe/readinessProbe with httpGet to /_ops/health
#   - Docker Compose: healthcheck with curl from host or sidecar container
#   - Manual: curl http://localhost:8443/_ops/health (from host, not container)

# Run the proxy
# Default flags can be overridden via docker run args
ENTRYPOINT ["/app/chaperone"]

# Default arguments (can be overridden)
# Note: TLS disabled by default in container since certs must be mounted
CMD ["-tls=false", "-addr=:8443"]
