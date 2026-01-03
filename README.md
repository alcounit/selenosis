# selenosis
A stateless Selenium hub for Kubernetes that creates a browser pod per session and proxies WebDriver traffic to it.

## What it does
- Accepts standard WebDriver requests at `/wd/hub` (and root).
- Creates a `Browser` resource via `browser-service` based on requested capabilities.
- Waits for the browser pod to become ready, then proxies traffic to the sidecar `seleniferous` inside that pod.
- Encodes the pod IP into a UUID session id so routing stays stateless.

## Requirements
- Kubernetes cluster.
- `browser-controller` CRDs installed (for `Browser` resources).
- `browser-service` running and reachable at `BROWSER_SERVICE_URL`.
- Browser pod image includes `seleniferous` sidecar listening on `PROXY_PORT`.

## Configuration
Selenosis is configured via environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:4444` | HTTP listen address. |
| `BROWSER_SERVICE_URL` | `http://browser-service:8080` | `browser-service` API base URL. |
| `PROXY_PORT` | `4445` | Sidecar port inside the browser pod. |
| `NAMESPACE` | `default` | Kubernetes namespace where `Browser` resources are created. |
| `SESSION_CREATE_ATTEMPTS` | `5` | Reserved for retries (loaded but not currently used). |
| `SESSION_CREATE_TIMEOUT` | `3m` | Reserved for timeouts (loaded but not currently used). |

## Endpoints
Selenosis exposes Selenium-compatible endpoints on both `/` and `/wd/hub`.

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/session` or `/wd/hub/session` | Create a new WebDriver session. |
| `*` | `/wd/hub/session/{sessionId}/*` | Proxy all session traffic (HTTP and WebSocket). |
| `GET` | `/status` or `/wd/hub/status` | Simple service status response. |
| `*` | `/selenosis/v1/sessions/{sessionId}/proxy/http/*` | Internal HTTP-only proxy used by Seleniferous. |

## Request flow
1. Client calls `POST /wd/hub/session` with W3C capabilities.
2. Selenosis creates a `Browser` resource via `browser-service`.
3. When the browser pod is `Running`, Selenosis maps its IP to a UUID session id.
4. All session requests are proxied to the sidecar `seleniferous` in that pod.

## Example: create session
```bash
curl -sS -X POST http://localhost:4444/wd/hub/session \
  -H 'Content-Type: application/json' \
  -d '{
    "capabilities": {
      "alwaysMatch": {
        "browserName": "chrome",
        "browserVersion": "120.0"
      }
    }
  }'
```

The response is proxied from the browser and contains the `sessionId` used for subsequent requests.

## Example: proxy a command
```bash
curl -sS -X GET http://localhost:4444/wd/hub/session/<sessionId>/url
```

## Networking and headers
If you run behind a reverse proxy or ingress, set these headers so Selenosis can build correct external URLs for the sidecar:
- `X-Forwarded-Proto`
- `X-Forwarded-Host`

Selenosis also adds `Selenosis-Request-ID` to outgoing requests for tracing.

## Docker
Build and run locally:

```bash
docker build -t selenosis:local .

docker run --rm -p 4444:4444 \
  -e BROWSER_SERVICE_URL=http://browser-service:8080 \
  -e NAMESPACE=default \
  selenosis:local
```

In Kubernetes, point `BROWSER_SERVICE_URL` to the in-cluster service and expose `LISTEN_ADDR` as needed.
