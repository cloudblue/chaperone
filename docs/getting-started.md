# Getting Started with Chaperone

This tutorial walks you through running Chaperone for the first time and
making a proxied API request. By the end, you'll have a working proxy
that injects credentials into outgoing requests.

**Time:** ~10 minutes

**What you'll learn:**
- How to build and run Chaperone with Docker
- How to verify the proxy is healthy
- How to send a proxied request through Chaperone
- How Chaperone injects credentials automatically

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Docker** | 20.10+ | Running Chaperone |
| **curl** | any | Sending test requests |

## Step 1: Build the Docker Image

Clone the repository and build:

```bash
git clone https://github.com/cloudblue/chaperone.git
cd chaperone
docker build -t chaperone:latest .
```

## Step 2: Start Chaperone

Chaperone requires a configuration file with an **allow-list** of permitted
target hosts — this is a security feature that prevents the proxy from being
used to reach arbitrary destinations. A tutorial config is included that
permits requests to `httpbin.org` (a public testing service):

> **What's in `configs/getting-started.yaml`?**
>
> ```yaml
> server:
>   addr: ":8443"
>   admin_addr: ":9090"
>   tls:
>     enabled: false            # No mTLS for the tutorial
>
> upstream:
>   allow_list:
>     "httpbin.org":
>       - "/**"                 # Allow all paths on httpbin.org
> ```
>
> In production, you'd replace `httpbin.org` with your real vendor API hosts
> and enable TLS. See the [Configuration Reference](reference/configuration.md).

Start the container in the foreground so you can see the startup logs:

```bash
docker run --rm --name chaperone-tutorial \
  -p 8443:8443 \
  -p 9090:9090 \
  -v $(pwd)/configs/getting-started.yaml:/app/config.yaml:ro \
  chaperone:latest
```

Chaperone logs are JSON-formatted (structured logging for production use).
Look for these key messages in the output:

- **"starting chaperone"** — Startup with version and config summary
- **"admin server started"** with `addr=:9090` — Admin endpoints are ready
- **"starting proxy server in HTTP mode"** with `addr=:8443` — Proxy is accepting traffic
- **"server listening"** — Ready for requests

> You may also see warnings about no credentials file (expected for this
> tutorial) and about config file permissions (safe to ignore in Docker).

Leave this terminal running and **open a new terminal** for the next steps.

Port mapping:
- **8443** — Proxy traffic port (where you send requests)
- **9090** — Admin port (health checks, metrics)

## Step 3: Verify Health

Check that the proxy is running:

```bash
# Health check (admin port)
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}

# Version check (admin port)
curl -s http://localhost:9090/_ops/version
# {"version": "..."}
```

Both endpoints are also available on the traffic port (8443).

## Step 4: Understand the Request Flow

When a platform sends a request through Chaperone, here's what happens:

```
Platform → Chaperone → Vendor API
              │
              ├─ 1. Parse context headers (vendor ID, target URL, etc.)
              ├─ 2. Validate target URL against allow-list
              ├─ 3. Call plugin to get credentials
              ├─ 4. Inject credentials into the request
              ├─ 5. Forward to vendor API
              └─ 6. Return response (with credentials stripped)
```

The platform sends context via headers (using the configurable prefix,
default `X-Connect-`):

| Header | Purpose |
|--------|---------|
| `X-Connect-Target-URL` | Where to forward the request |
| `X-Connect-Vendor-ID` | Which vendor's credentials to use |
| `X-Connect-Product-ID` | Product identifier |
| `X-Connect-Marketplace-ID` | Marketplace identifier |
| `X-Connect-Subscription-ID` | Subscription identifier |

## Step 5: Send a Proxied Request

Send a request through Chaperone to `httpbin.org/headers`, which echoes
back all the request headers it received:

```bash
curl -s http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://httpbin.org/headers" \
  -H "X-Connect-Vendor-ID: test-vendor"
```

You should get a JSON response with a `headers` object. Notice:

- **`Connect-Request-Id`** — Chaperone generates a unique trace ID for
  every request, useful for correlating logs across services.
- **`X-Connect-Target-Url`** and **`X-Connect-Vendor-Id`** — The context
  headers you sent, which Chaperone forwarded to the target.

In a real deployment with a credentials plugin configured, Chaperone
would also inject authentication headers (e.g., `Authorization`) into
the outgoing request — and then **strip them from the response** before
returning it, so credentials never leak back to the calling platform.
That's the core security value of the proxy. See the
[Plugin Development Guide](guides/plugin-development.md) to build
a plugin that provides credentials.

## Step 6: Clean Up

Press `Ctrl+C` in the terminal running Chaperone to stop it.

## Next Steps

Now that you've seen Chaperone in action:

- **Build your plugin:** Follow the [Plugin Development Guide](guides/plugin-development.md) to inject real credentials
- **Deploy for production:** Follow the [Deployment Guide](guides/deployment.md) for mTLS setup
- **Set up certificates:** See [Certificate Management](guides/certificate-management.md)
- **Configure routing:** Read the [Configuration Reference](reference/configuration.md)
