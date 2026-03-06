[![Go Reference](https://pkg.go.dev/badge/github.com/alcounit/selenosis.svg)](https://pkg.go.dev/github.com/alcounit/selenosis/v2)

# selenosis
A stateless Selenium hub for Kubernetes that creates a browser resource per session and proxies traffic to it.

## Table of Contents

- [What it does](#what-it-does)
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
- [Basic authentication](#basic-authentication)
- [Build and image workflow](#build-and-image-workflow)
- [Deployment](#deployment)

## What it does
- Selenosis exposes Selenium-compatible endpoints on both `/` and `/wd/hub`.
- Creates a `Browser` resource via `browser-service` based on requested capabilities.
- Waits for the browser pod to become ready, then proxies traffic to the sidecar `seleniferous` inside that pod.

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
| `*` | `/selenosis/v1/sessions/{sessionId}/proxy/http/*` | Internal HTTP-only proxy used by Seleniferous. |

## Request flow
1. Client calls `POST /wd/hub/session` with W3C capabilities or `WS /playwright/{name}/{version}`.
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

Variable	Description
- BINARY_NAME	Name of the produced binary (selenosis).
- REGISTRY	Docker registry prefix (default: localhost:5000).
- IMAGE_NAME	Full image name (<registry>/selenosis).
- VERSION	Image version/tag (default: develop).
- PLATFORM	Target platform (default: linux/amd64).
- CONTAINER_TOOL docker cmd

REGISTRY, VERSION is expected to be provided externally, which allows the same Makefile to be used locally and in CI.

## Deployment

Helm chart [selenosis-deploy](https://github.com/alcounit/selenosis-deploy)
