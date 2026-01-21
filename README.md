# selenosis
A stateless Selenium hub for Kubernetes that creates a browser resource per session and proxies WebDriver traffic to it.

## What it does
- Accepts standard WebDriver requests at `/wd/hub` (and root).
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

## Example: proxy a command
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


## Networking and headers
If you run behind a reverse proxy or ingress, set these headers so Selenosis can build correct external URLs for the sidecar:
- `X-Forwarded-Proto`
- `X-Forwarded-Host`

Selenosis also adds `Selenosis-Request-ID` to outgoing requests for tracing.

# selenosis:options

`selenosis:options` is a vendor-namespaced WebDriver capability that allows users to pass session-specific overrides to Selenosis without changing CRDs or cluster-level configuration.

The options are attached to the `Browser` resource (via annotations) and applied by the controller at Pod creation time.

---

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
              "SESSION_IDLE_TIMEOUT": "300"
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
        new URL("http://localhost:4444/wd/hub"),
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

## Behavioral Notes

- `selenosis:options` is validated and parsed by the controller.
- Invalid JSON results in the Browser being marked as **Failed**.
- Options are applied when the Pod is created.


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

Minimal configuration

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: selenosis
  namespace: default
```
```yaml
apiVersion: v1
kind: Service
metadata:
  name: selenosis
  labels:
    role: selenosis
spec:
  type: NodePort
  selector:
    role: selenosis
  ports:
  - name: http
    port: 4444   
    targetPort: 4444
```

``` yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: selenosis
  labels:
    role: selenosis
spec:
  replicas: 1
  selector:
    matchLabels:
      role: selenosis
  template:
    metadata:
      labels:
        role: selenosis
    spec:
      serviceAccountName: selenosis
      containers:
      - name: service
        image: alcounit/selenosis:latest
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 4444
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "500m"
            memory: "256Mi"
```
