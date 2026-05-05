# Admin Portal Guide

How to build, configure, and run the Chaperone Admin Portal (`chaperone-admin`) for fleet monitoring.

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Go** | 1.26+ | Building the binary |
| **Node.js** | 24 (CI-tested) | Building the Vue SPA |
| **pnpm** | 10 (CI-tested) | Frontend package manager |

`admin/ui/package.json` has no `engines` field, so other recent Node and pnpm versions will likely work locally — Node 24 and pnpm 10 are what CI runs against.

You can build, configure, and start the portal without a running proxy. A running Chaperone proxy is only needed once you reach step 3 of [First Run](#first-run), where you register and monitor instances.

## Build

```bash
make build-admin
```

This produces a single `chaperone-admin` binary at `./bin/chaperone-admin` with the Vue SPA embedded. No separate web server or static file serving needed.

The examples below assume `chaperone-admin` is on your `PATH`. If not, invoke it as `./bin/chaperone-admin` or add `./bin` to your `PATH`.

## Run for development

The dev backend (built with the `dev` build tag via `make run-admin`) serves the SPA from `admin/ui/dist` on disk. Populate that directory first, otherwise the binary exits with `UI dist directory not found`:

```bash
cd admin/ui && pnpm build
```

Once `dist/` exists, pick one of two dev modes:

- **Backend only** — run `make run-admin`. The Go server reads the SPA from disk. No hot reload; rebuild the SPA with `pnpm build` to pick up frontend changes.
- **Backend + Vite hot module replacement (HMR)** — run `make run-admin` in one terminal and `cd admin/ui && pnpm dev` in another. Open the Vite URL (default `http://localhost:5173`). Vite proxies API calls to the Go backend on `:8080` and reloads SPA changes instantly.

## Configuration

Create a `chaperone-admin.yaml` file (or pass `--config /path/to/config.yaml`):

```yaml
server:
  addr: "127.0.0.1:8080"
  secure_cookies: false   # Set to true when serving behind HTTPS

database:
  path: "./chaperone-admin.db"

scraper:
  interval: "10s"
  timeout: "5s"

session:
  max_age: "24h"
  idle_timeout: "2h"

audit:
  retention_days: 90

log:
  level: "info"       # debug, info, warn, error
  format: "json"      # json, text
```

The values above are the defaults; the portal starts with zero config for local testing.

> **Note:** `database.path` is resolved relative to the current working directory when no absolute path is given. Run `create-user`, `reset-password`, and `serve` from the same directory (or pass an absolute path / `--config`), otherwise each invocation will read or create a different SQLite file and you'll get a "user not found" failure at login.

### Environment variable overrides

Every config key can be overridden via environment variables using the `CHAPERONE_ADMIN_SECTION_KEY` convention:

| Config Key | Environment Variable |
|-----------|---------------------|
| `server.addr` | `CHAPERONE_ADMIN_SERVER_ADDR` |
| `server.secure_cookies` | `CHAPERONE_ADMIN_SERVER_SECURE_COOKIES` |
| `database.path` | `CHAPERONE_ADMIN_DATABASE_PATH` |
| `scraper.interval` | `CHAPERONE_ADMIN_SCRAPER_INTERVAL` |
| `scraper.timeout` | `CHAPERONE_ADMIN_SCRAPER_TIMEOUT` |
| `session.max_age` | `CHAPERONE_ADMIN_SESSION_MAX_AGE` |
| `session.idle_timeout` | `CHAPERONE_ADMIN_SESSION_IDLE_TIMEOUT` |
| `audit.retention_days` | `CHAPERONE_ADMIN_AUDIT_RETENTION_DAYS` |
| `log.level` | `CHAPERONE_ADMIN_LOG_LEVEL` |
| `log.format` | `CHAPERONE_ADMIN_LOG_FORMAT` |

Environment variables take precedence over the config file.

## First Run

### 1. Create an admin user

The portal requires authentication. No users exist on first start, so create one via CLI:

```bash
chaperone-admin create-user --username admin
```

The command prompts for a password and then asks you to confirm it. Constraints:

- Input is hidden as you type.
- Minimum length is 12 characters.
- A real TTY is required — the prompt cannot be piped via stdin or here-strings.

> **Note:** The portal returns 401 on all API routes until at least one user exists.

### 2. Start the server

```bash
chaperone-admin serve
# or simply:
chaperone-admin
```

The `serve` command is the default when no subcommand is given. Open `http://localhost:8080` in your browser and log in with the credentials you created.

### 3. Confirm network reachability

The portal polls each proxy's admin port (`/_ops/health`, `/_ops/version`, `GET /metrics`) every 10 seconds. Before registering instances, make sure the admin port is reachable from the portal host.

| Topology | Proxy Admin Port Config | When to Use |
|----------|------------------------|-------------|
| **Single-host** | Default (`127.0.0.1:9090`) | Portal and proxies on the same machine |
| **Multi-host** | Set `admin_addr` to a reachable interface (e.g., `0.0.0.0:9090`) | Proxies on separate hosts/containers |

> **Warning:** The admin port exposes health, version, and Prometheus metrics. Keep it within a trusted network (VPC, Kubernetes cluster network, firewall-restricted subnet). Never expose it to the public internet.

**Kubernetes**: Use `admin_addr: "0.0.0.0:9090"` to make the admin port reachable within the cluster. Do not create a `LoadBalancer` or `NodePort` Service for the admin port.

### 4. Register proxy instances

Log in and click "Add Your First Instance" on the welcome screen. Enter:

- **Name**: A human-readable label (e.g., `proxy-prod-01`)
- **Address**: The proxy's admin `host:port` (e.g., `10.0.0.1:9090`)

Use "Test Connection" to verify the portal can reach the proxy before saving. If the test fails, check:

- The proxy is running and its admin server is started
- The admin port is reachable from the portal host (see step 3 above)
- No firewall rules blocking the connection

## CLI Commands

**Global flag:** `--config <path>` works on every command and selects the config file. The `serve` command also accepts `--version` to print the version and exit.

| Command | Description |
|---------|-------------|
| `chaperone-admin serve [flags]` | Start the portal server (default) |
| `chaperone-admin create-user --username <name>` | Create a new admin user |
| `chaperone-admin reset-password --username <name>` | Reset a user's password and invalidate all their sessions |

## Manage Sessions

- To adjust session lifetime, set `session.max_age` (absolute TTL, default 24h) and `session.idle_timeout` (inactivity limit, default 2h) in the config file.
- To force a user to re-authenticate, run `chaperone-admin reset-password --username <name>` — this invalidates all their sessions.
- To end your own session, click "Logout" in the sidebar. The session is invalidated server-side immediately.

## Review the Audit Log

All portal actions (instance add/edit/remove, login, logout, password changes) are recorded in the audit log.

- To view the log, click "Audit Log" in the sidebar.
- To find specific events, use the full-text search bar or filter by action type and date range.
- To change retention, set `audit.retention_days` in the config file (default: 90 days, set to `0` to keep forever).
- To export audit data, query the SQLite database file at the path configured in `database.path`.

## Monitor Metrics and Health

- To view per-instance metrics, open the dashboard. It displays RPS, latency percentiles (p50, p95, p99), error rate, active connections, and panic count for each proxy, computed from each proxy's `/metrics` endpoint polled every 10 seconds.
- To interpret health badges, read them as: **unknown** (before first poll), **healthy** (last poll succeeded), or **unreachable** (3 consecutive failures). A single successful poll restores an unreachable instance to healthy.
- To wait through the post-restart placeholder, give the portal at least two scrape cycles (~20 seconds) after a restart — charts show "Collecting data points..." until two snapshots exist to compute rates from.
- To plan around history retention, note that metrics are kept in memory only. The portal retains 360 scrape snapshots per instance (`DefaultCapacity` in `admin/metrics/metrics.go`), which at 10s intervals is exactly 1 hour of history. A restart clears all metrics.
