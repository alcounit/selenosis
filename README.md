[![GitHub release](https://img.shields.io/github/v/release/alcounit/selenosis)](https://github.com/alcounit/selenosis/releases) [![Go Reference](https://pkg.go.dev/badge/github.com/alcounit/selenosis.svg)](https://pkg.go.dev/github.com/alcounit/selenosis/v2) [![Docker Pulls](https://img.shields.io/docker/pulls/alcounit/selenosis.svg)](https://hub.docker.com/r/alcounit/selenosis) [![codecov](https://codecov.io/gh/alcounit/selenosis/branch/main/graph/badge.svg)](https://codecov.io/gh/alcounit/selenosis)

# selenosis
A stateless Selenium hub for Kubernetes that creates a browser resource per session and proxies traffic to it.

## Table of Contents

- [What it does](#what-it-does)
- [Ecosystem](#ecosystem)
- [Requirements](#requirements)
- [Configuration](#configuration)
- [Endpoints](#endpoints)
- [Request flow](#request-flow)
- [Examples](#examples)
  - [Create a session](#example-create-a-session)
  - [Proxy a command](#example-proxy-a-command)
- [WebDriver BiDi support](#webdriver-bidi-support)
- [CDP support](#cdp-support)
- [Networking and headers](#networking-and-headers)
- [selenosis:options](#selenosisoptions)
  - [Supported scope](#supported-scope)
  - [Example payload](#example-payload)
  - [Behavioral notes](#behavioral-notes)
- [Playwright (experimental)](#playwright-experimental)
- [MCP (experimental)](#mcp-experimental)
  - [Create an MCP session](#create-an-mcp-session)
  - [Proxying MCP traffic](#proxying-mcp-traffic)
  - [Terminating an MCP session](#terminating-an-mcp-session)
  - [MCP Playwright example](#mcp-playwright-example)
  - [MCP Selenium example](#mcp-selenium-example)
- [Basic authentication](#basic-authentication)
- [Build and image workflow](#build-and-image-workflow)
- [Deployment](#deployment)

## What it does
- Selenosis exposes Selenium-compatible endpoints on both `/` and `/wd/hub`.
- Creates a `Browser` resource via `browser-service` based on requested capabilities.
- Waits for the browser pod to become ready, then proxies traffic to the sidecar `seleniferous` inside that pod.

## Ecosystem

selenosis is the stateless hub of a larger Kubernetes-native platform for running
ephemeral browser sessions. It is one of several components — to deploy the whole
stack, use the Helm chart.

- **[selenosis-deploy](https://github.com/alcounit/selenosis-deploy)** — Helm chart that deploys the full stack (CRDs, RBAC, all services, ingress). Start here.
- **[selenosis](https://github.com/alcounit/selenosis)** (this repo) — stateless Selenium/Playwright/MCP hub; creates `Browser` resources and proxies session traffic.
- **[seleniferous](https://github.com/alcounit/seleniferous)** — sidecar proxy inside each browser pod; manages session lifecycle and routing.
- **[browser-controller](https://github.com/alcounit/browser-controller)** — Kubernetes operator that reconciles `Browser` and `BrowserConfig` CRDs into pods.
- **[browser-service](https://github.com/alcounit/browser-service)** — REST + SSE facade over `Browser` resources.
- **[browser-ui](https://github.com/alcounit/browser-ui)** — web dashboard with live session view and VNC viewer.

## Requirements
- Kubernetes cluster.
- [browser-controller](https://github.com/alcounit/browser-controller) CRDs installed (for `Browser` and `BrowserConfig` resources).
- [browser-service](https://github.com/alcounit/browser-service) running and reachable at `BROWSER_SERVICE_URL`.
- Browser pod image includes [seleniferous](https://github.com/alcounit/seleniferous) sidecar listening on `PROXY_PORT`.

## Configuration
Selenosis is configured via environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:4444` | HTTP listen address. |
| `BROWSER_SERVICE_URL` | `http://browser-service:8080` | `browser-service` API base URL. |
| `PROXY_PORT` | `4445` | Sidecar port inside the browser pod. |
| `NAMESPACE` | `selenosis` | Kubernetes namespace where `Browser` resources are created. |
| `BROWSER_STARTUP_TIMEOUT` | `3m` | Maximum allowed time for a Browser resource to be created. |
| `BASIC_AUTH_FILE` | | Points to a file containing a JSON list of users.  |

## Endpoints
Selenosis exposes Selenium-compatible endpoints on both `/` and `/wd/hub`.

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/session` or `/wd/hub/session` | Create a new WebDriver session. |
| `*` | `/session/{sessionId}/*` or `/wd/hub/session/{sessionId}/*` | Proxy all session traffic (HTTP and WebSocket). |
| `GET` | `/status` or `/wd/hub/status` | Simple service status response. |
| `WS` | `/playwright/{name}/{version}` | Creates and proxies WS traffic. |
| `POST` | `/mcp` | MCP Streamable HTTP transport. With no `Mcp-Session-Id` header and `?browser=<name>&version=<version>` it creates a browser and initializes a session; otherwise routed by `Mcp-Session-Id`. |
| `GET` | `/mcp` | MCP Streamable HTTP transport — server-initiated stream (routed by `Mcp-Session-Id`). |
| `DELETE` | `/mcp` | Terminate an MCP session and tear down its browser (routed by `Mcp-Session-Id`). |
| `*` | `/selenosis/v1/sessions/{sessionId}/proxy/http/*` | Internal HTTP-only proxy used by Seleniferous. |

## Request flow
1. Client calls `POST /wd/hub/session` with W3C capabilities, `WS /playwright/{name}/{version}`, or `POST /mcp?browser=<name>&version=<version>}` (MCP `initialize`).
2. Selenosis creates a `Browser` resource via `browser-service`.
3. When the browser pod is `Running`, Selenosis maps its IP to a UUID session id.
4. All session requests are proxied to the sidecar `seleniferous` in that pod.

## Examples

### Example: create session
```bash
curl -sS -X POST http://{selenosis_host:port}/wd/hub/session \
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

### Example: proxy a command
```bash
curl -sS -X GET http://{selenosis_host:port}/wd/hub/session/<sessionId>/url
```

## WebDriver BiDi support

Selenosis supports WebDriver BiDi by proxying WebSocket connections per session.

To enable BiDi, request `webSocketUrl: true` in capabilities.

```bash
curl -X POST http://{selenosis_host:port}/wd/hub/session \
  -H 'Content-Type: application/json' \
  -d '{
    "capabilities": {
      "alwaysMatch": {
        "browserName": "chrome",
        "browserVersion": "120.0"
        "webSocketUrl": true
      }
    }
  }'
```

The response will include `capabilities.webSocketUrl` for the BiDi connection.

## CDP support

Chromium-based browsers can expose Chrome DevTools Protocol (CDP).
Selenosis transparently proxies CDP traffic through the `seleniferous` sidecar.

## Playwright (experimental)
`WS /playwright/{name}/{version}`

The `/playwright/{name}/{version}` endpoint is responsible for provisioning a Browser resource and proxying the current WebSocket connection to the seleniferous sidecar.

The request flow is as follows:
- The endpoint extracts the browser name and version from the URL path.
- Query parameters are parsed and interpreted as Selenosis options, allowing dynamic configuration of the underlying Kubernetes resources (for example, labels or environment variables).
- A new Browser custom resource is created in Kubernetes using the resolved configuration.
- A WebSocket reverse proxy is established, and the current WebSocket connection is transparently proxied to the seleniferous sidecar.

## Example:
```javascript
import { chromium } from 'playwright';

const browser = await chromium.connect({
  wsEndpoint: 'ws://http://{selenosis_host:port}/playwright/playwright-chrome/1.58.0'
});

const context = await browser.newContext();
const page = await context.newPage();

await page.goto('https://example.com');
await page.screenshot({ path: 'example.png' });

await browser.close();

```

## Networking and headers
If you run behind a reverse proxy or ingress, set these headers so Selenosis can build correct external URLs for the sidecar:
- `X-Forwarded-Proto`
- `X-Forwarded-Host`

Selenosis also adds `Selenosis-Request-ID` to outgoing requests for tracing.

# selenosis options

`selenosis options` is a vendor-namespaced WebDriver capability that allows users to pass session-specific overrides to Selenosis without changing CRDs or cluster-level configuration.

The options are attached to the `Browser` resource (via annotations) and applied by the controller at Pod creation time.


## Supported scope

### Global level

- **labels**: Kubernetes labels applied to the Browser Pod

### Per-container level

- **containers.<containerName>.env**: environment variables injected into matching containers

The controller does not rely on hard-coded container names. It iterates over the Pod containers and applies overrides only when names match.



## JSON Schema (conceptual)

```json
selenosis:options = {
  "labels": { "<string>": "<string>" },
  "containers": {
    "<containerName>": {
      "env": { "<ENV_NAME>": "<ENV_VALUE>" }
    }
  }
}
```

## Query parameters (playwright endpoint)

Pod Labels:
```
labels.<key>=<value>
```
Container environment variables:
```
containers.<container>.env.<ENV_NAME>=<value>
```

### Notes

- `labels` are optional.
- `containers` is optional.
- Each section is applied independently.

## Example Payloads
```json
{
  "capabilities": {
    "alwaysMatch": {
      "browserName": "chrome",
      "version": "139.0",
      "selenosis:options": {
        "labels": {
          "team": "qa"
        },
        "containers": {
          "browser": {
            "env": {
              "LOG_LEVEL": "debug"
            }
          },
          "seleniferous": {
            "env": {
              "SESSION_IDLE_TIMEOUT": "3m"
            }
          }
        }
      }
    }
  }
}
```

**Result**

Pod labels:

```
team=qa
```

Container **browser** receives:

```
LOG_LEVEL=debug
```

Container **seleniferous** receives:

```
SESSION_IDLE_TIMEOUT=3m
```

If a container name does not exist in the Pod, its configuration is ignored.

## Java Example (Selenium 4)

### Using `DesiredCapabilities`

```java
import org.openqa.selenium.remote.DesiredCapabilities;
import org.openqa.selenium.remote.RemoteWebDriver;

import java.net.URL;
import java.util.HashMap;
import java.util.Map;

public class SelenosisOptionsExample {

  public static void main(String[] args) throws Exception {

    DesiredCapabilities caps = new DesiredCapabilities();
    caps.setCapability("browserName", "chrome");

    // Global labels
    Map<String, String> labels = new HashMap<>();
    labels.put("project", "payments");

    // Container env vars
    Map<String, String> browserEnv = new HashMap<>();
    browserEnv.put("LOG_LEVEL", "debug");

    Map<String, Object> browserContainer = new HashMap<>();
    browserContainer.put("env", browserEnv);

    Map<String, Object> containers = new HashMap<>();
    containers.put("browser", browserContainer);

    Map<String, Object> selenosisOptions = new HashMap<>();
    selenosisOptions.put("labels", labels);
    selenosisOptions.put("containers", containers);

    // Vendor-namespaced capability
    caps.setCapability("selenosis:options", selenosisOptions);

    RemoteWebDriver driver = new RemoteWebDriver(
        new URL("http://{selenosis_host:port}/wd/hub"),
        caps
    );

    try {
      driver.get("https://example.com");
    } finally {
      driver.quit();
    }
  }
}
```

## Playwright Example

### Using `query prameters`
```javascript
import { chromium } from 'playwright';

const browser = await chromium.connect({
  wsEndpoint: 'ws://http://{selenosis_host:port}/playwright/playwright-chrome/1.58.0?labels.team=qa&labels.project=selenosis&containers.seleniferous.env.SESSION_IDLE_TIMEOUT=5m&containers.seleniferous.env.SESSION_CREATE_TIMEOUT=5m'
});

const context = await browser.newContext();
const page = await context.newPage();

await page.goto('https://example.com');
await page.screenshot({ path: 'example.png' });

await browser.close();

```

## Behavioral Notes

- `selenosis:options` is validated and parsed by the controller.
- Invalid JSON results in the Browser being marked as **Failed**.
- Options are applied when the Pod is created.

## Error responses

When proxying to a browser pod fails, Selenosis maps the failure to a status code that lets clients react correctly. A pod that is **unreachable** (for example, torn down after the idle timeout) is detected as a connection/dial failure to the sidecar:

| Endpoint | Pod unreachable | Other proxy failure |
| --- | --- | --- |
| `POST /session` | `500` Selenium `session not created` | `500` Selenium `session not created` |
| `* /session/{sessionId}/*` (HTTP) | `404` Selenium `invalid session id` | `500` Selenium `unknown error` |
| `/selenosis/v1/.../proxy/http/*` | `404` `session not found` | `500` |
| `POST /mcp` (initialize) | `500` | `500` |
| `POST`/`GET`/`DELETE /mcp` (routed by `Mcp-Session-Id`) | `404` `session not found` | `500` |

For session-scoped endpoints an unreachable pod returns `404` so the client can tell the session no longer exists and start a new one. WebSocket endpoints (BiDi/CDP/Playwright) are not covered by this mapping — a failed upstream dial closes the connection.

## MCP (experimental)

Selenosis supports the [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) for Playwright and Selenium MCP servers running inside browser pods.

The MCP server runs inside the browser container (not in seleniferous). Selenosis creates the browser pod and proxies MCP traffic through the seleniferous sidecar to the MCP server.

Only the **Streamable HTTP** transport is supported, exposed on a single endpoint `/mcp` (`POST`, `GET`, `DELETE`). Sessions are identified by the `Mcp-Session-Id` header — Selenosis is stateless and derives the target pod from it.

### Create an MCP session

A session is created by the standard MCP `initialize` call: a `POST /mcp` **without** an `Mcp-Session-Id` header. Selenosis requires `browser` and `version` query parameters to know which browser to start:

`POST /mcp?browser=<name>&version=<version>`

Selenosis creates a `Browser` resource, waits for the pod to become ready, and forwards the `initialize` request to the MCP server. The pod-derived session id is returned in the `Mcp-Session-Id` **response header**; clients send it back on every subsequent request.

Additional query parameters are parsed as [`selenosis:options`](#selenosisoptions) (same as the Playwright endpoint).

```bash
curl -isS -X POST 'http://{selenosis_host:port}/mcp?browser=playwright-mcp&version=0.0.75' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"example","version":"1.0.0"}}}'
# the response includes header: Mcp-Session-Id: <pod-uuid>
```

### Proxying MCP traffic

Once initialized, send the returned `Mcp-Session-Id` header on every request. Selenosis routes by that header to the correct pod and proxies to the sidecar `/mcp`.

```bash
# Client request
curl -sS -X POST http://{selenosis_host:port}/mcp \
  -H 'Content-Type: application/json' \
  -H 'Mcp-Session-Id: <pod-uuid>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# Server-initiated message stream
curl -sS http://{selenosis_host:port}/mcp \
  -H 'Mcp-Session-Id: <pod-uuid>'
```

### Terminating an MCP session

```bash
curl -sS -X DELETE http://{selenosis_host:port}/mcp \
  -H 'Mcp-Session-Id: <pod-uuid>'
```

This ends the MCP session and tears down the browser pod.

### Session expiry and reconnection

If a browser pod is torn down (for example after the idle timeout), a later `POST`/`GET`/`DELETE /mcp` carrying the now-stale `Mcp-Session-Id` returns **`404`**. Per the MCP spec, a `404` on a request that includes a session id signals that the session no longer exists, so a spec-compliant client drops the id and performs a fresh `initialize` — which transparently starts a new browser pod. A missing `Mcp-Session-Id` header on a non-initialize request returns `400`, as does a malformed (non-UUID) id.

### MCP Playwright example

```javascript
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';

// Point the transport at /mcp with the desired browser/version.
// The SDK performs `initialize`, captures the Mcp-Session-Id response header,
// and replays it on every subsequent request automatically.
const url = new URL('http://{selenosis_host:port}/mcp?browser=playwright-mcp&version=0.0.75');
const transport = new StreamableHTTPClientTransport(url);
const client = new Client({ name: 'example', version: '1.0.0' });
await client.connect(transport);

const tools = await client.listTools();
await client.callTool({
  name: 'browser_navigate',
  arguments: { url: 'https://example.com' }
});

await client.close(); // sends DELETE /mcp and tears down the pod
```

### MCP Selenium example

```javascript
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';

const url = new URL('http://{selenosis_host:port}/mcp?browser=playwright-mcp&version=0.0.75');
const transport = new StreamableHTTPClientTransport(url);
const client = new Client({ name: 'example', version: '1.0.0' });
await client.connect(transport);

const tools = await client.listTools();
await client.callTool({
  name: 'navigate',
  arguments: { url: 'https://example.com' }
});

await client.close();
```

## Basic Authentication in Selenosis

Selenosis supports optional HTTP Basic Authentication for protecting its public API endpoints.
Authentication is enabled by providing a users file via a Kubernetes Secret and referencing it through environment variables.
When Basic Auth is enabled, all incoming HTTP requests must include valid credentials.

### Configuration Overview

Basic Auth is disabled by default.

It becomes active when the following environment variable is set:

```
BASIC_AUTH_FILE
```
This variable must point to a file containing a JSON list of users.

```json
[
  { "user": "alice", "pass": "secret1" },
  { "user": "bob",   "pass": "secret2" }
]
```
- `user` — username
- `pass` — password (plain text in this example)

### Kubernetes Secret Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: basic-auth
  namespace: selenosis
type: Opaque
stringData:
  users.json: |
    [
      { "user": "alice", "pass": "secret1" },
      { "user": "bob",   "pass": "secret2" }
    ]

```

### Deployment Configuration

```yaml
env:
  - name: BASIC_AUTH_FILE
    value: /etc/auth/users.json

volumeMounts:
  - name: basic-auth
    mountPath: /etc/auth
    readOnly: true

volumes:
  - name: basic-auth
    secret:
      secretName: basic-auth
```

### Client Usage

Clients must send credentials using `http://{username}:{password}@{selenosis_host:port}/*`

## Build and image workflow

The project is built and packaged entirely via Docker. Local Go installation is not required for producing the final artifact.

## Build variables

The build process is controlled via the following Makefile variables:

| Variable         | Description                                                  |
|------------------|--------------------------------------------------------------|
| `BINARY_NAME`    | Name of the produced binary (fixed: `selenosis`)            |
| `REGISTRY`       | Docker registry prefix (default: `localhost:5000`)           |
| `IMAGE_NAME`     | Full image name, derived as `$(REGISTRY)/$(BINARY_NAME)`     |
| `VERSION`        | Image version/tag (default: `develop`)                       |
| `EXTRA_TAGS`     | Additional `-t` tags passed to `docker-push` (default: none) |
| `PLATFORM`       | Target platform (default: `linux/amd64`)                     |
| `CONTAINER_TOOL` | Container build tool (default: `docker`)                     |

`REGISTRY` and `VERSION` are expected to be provided externally, which allows the same Makefile to be used locally and in CI.

## Deployment

The full stack — CRDs, RBAC, all services, and ingress — is deployed with the
[selenosis-deploy](https://github.com/alcounit/selenosis-deploy) Helm chart:

```bash
helm install selenosis .
```

See the chart's `values.yaml` for image versions, service types, ingress, session
timeouts, and authentication settings.
