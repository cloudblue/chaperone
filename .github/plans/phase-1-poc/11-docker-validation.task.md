# Task: Docker Validation

**Status:** [x] Completed  
**Priority:** P0  
**Estimated Effort:** M (Medium)

## Objective

Create a Dockerfile and verify the PoC compiles and runs successfully inside a container.

## Design Spec Reference

- **Primary:** Section 6.2.1 - Containerized (Docker) - Primary Support
- **Primary:** Section 3.4 - System Constraints (Statelessness)
- **Related:** Section 6.4 - Deployment Matrix

## Dependencies

- [x] All previous tasks - This is the final integration validation

## Acceptance Criteria

- [x] Multi-stage Dockerfile exists
- [x] `docker build -t chaperone:poc .` succeeds
- [x] `docker run` starts the container
- [x] Health check returns 200
- [x] Image uses minimal base (distroless or alpine)
- [x] Image runs as non-root user
- [x] No secrets baked into image
- [x] Image size is reasonable (< 50MB for distroless) - **14MB achieved**

## Implementation Hints

### Dockerfile (Multi-Stage)

```dockerfile
# Stage 1: Builder
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
COPY sdk/go.mod sdk/go.sum ./sdk/
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o chaperone ./cmd/chaperone

# Stage 2: Runtime
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Copy binary
COPY --from=builder /build/chaperone /app/chaperone

# Copy default config (optional for PoC)
# COPY configs/default.yaml /app/config.yaml

# Non-root user (distroless/static:nonroot handles this)
USER nonroot:nonroot

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD ["/app/chaperone", "health"] || exit 1

# Run
ENTRYPOINT ["/app/chaperone"]
```

### Alternative: Alpine (if distroless issues)

```dockerfile
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /build/chaperone /app/chaperone

RUN adduser -D -u 1000 chaperone
USER chaperone

EXPOSE 8080 9090
ENTRYPOINT ["/app/chaperone"]
```

### Build and Test Script

```bash
#!/bin/bash
set -e

# Build
docker build -t chaperone:poc .

# Run (detached)
docker run -d --name chaperone-test -p 8080:8080 chaperone:poc

# Wait for startup
sleep 2

# Health check
curl -f http://localhost:8080/_ops/health || {
    docker logs chaperone-test
    docker rm -f chaperone-test
    exit 1
}

# Cleanup
docker rm -f chaperone-test

echo "Docker validation passed!"
```

### Gotchas

- `CGO_ENABLED=0` required for static binary (distroless compatibility)
- `go.mod` replace directives won't work in Docker; ensure proper module paths
- Need to copy `sdk/` directory structure for multi-module
- Health check command needs to exist in binary (or use curl in alpine)

## Files to Create/Modify

- [ ] `Dockerfile` - Multi-stage build
- [ ] `.dockerignore` - Exclude unnecessary files
- [ ] `scripts/docker-test.sh` - Validation script (optional)
- [ ] `Makefile` - Add `docker-build` and `docker-test` targets

### .dockerignore

```
.git
.github
*.md
!README.md
bin/
tmp/
*.test
coverage.*
```

### Makefile Targets

```makefile
.PHONY: docker-build docker-test

docker-build:
	docker build -t chaperone:poc .

docker-test: docker-build
	docker run --rm -d --name chaperone-test -p 8080:8080 chaperone:poc
	sleep 2
	curl -f http://localhost:8080/_ops/health
	docker rm -f chaperone-test
```

## Testing Strategy

### Build Verification

```bash
docker build -t chaperone:poc .
# Should complete without errors
```

### Runtime Verification

```bash
# Start container
docker run -d --name test -p 8080:8080 chaperone:poc

# Check health
curl http://localhost:8080/_ops/health
# Expected: {"status": "alive"}

# Check version
curl http://localhost:8080/_ops/version
# Expected: {"version": "...", "sdk_version": "..."}

# Cleanup
docker rm -f test
```

### Image Inspection

```bash
# Check image size
docker images chaperone:poc

# Check for secrets (should be empty)
docker history chaperone:poc

# Verify non-root
docker run --rm chaperone:poc whoami
# Expected: nonroot (or numeric UID)
```

## Security Considerations

- Non-root user in container
- No secrets in image layers
- Minimal base image (reduced attack surface)
- Read-only filesystem possible (for future hardening)
- No shell in distroless (prevents container escape techniques)

## Notes

This task is the final validation for Phase 1 PoC.
Success here means the architecture is proven and ready for Phase 2 features.

## Implementation Notes

**Decisions made during implementation (2026-02-02):**

### 1. No go.sum File Handling

The SDK has no external dependencies, so no `go.sum` file exists. The Dockerfile handles this by only copying `go.mod` files:
```dockerfile
COPY go.mod ./
COPY sdk/go.mod ./sdk/
```
When external dependencies are added later, update to include `go.sum` files.

### 2. Default CMD Uses Config File

Container starts with `/app/config.yaml` which has TLS disabled by default since certificates must be mounted at runtime (not baked into image). Users can override for mTLS:
```bash
# Mount custom config
docker run -v /path/to/config.yaml:/app/config.yaml chaperone:poc

# Or use environment variables
docker run -e CHAPERONE_SERVER_TLS_ENABLED=true -v /certs:/certs chaperone:poc
```

### 3. No HEALTHCHECK Directive in Dockerfile

Distroless images have no shell or curl, so Docker's `HEALTHCHECK` instruction cannot be used. Health checking must be done externally:
- **Kubernetes:** Use `livenessProbe` / `readinessProbe` with `httpGet`
- **Docker Compose:** Use `healthcheck` with a sidecar or external curl
- **Manual:** `curl http://localhost:8443/_ops/health`

### 4. Build Args for Version Injection

Version information is injected at build time via `--build-arg`:
```bash
docker build \
  --build-arg VERSION=$(git describe --tags) \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t chaperone:latest .
```
The Makefile `docker-build` target does this automatically.

### 5. Validation Suite

All acceptance criteria are validated automatically via `make docker-test`:
- Container starts and responds
- Health endpoint returns 200
- Version endpoint returns 200
- User is `nonroot:nonroot`
- No shell available (proves distroless base)
- Image size < 50MB
