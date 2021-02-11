[![Docker Pulls](https://img.shields.io/docker/pulls/alcounit/selenosis)](https://hub.docker.com/r/alcounit/selenosis/tags?page=1&ordering=last_updated)
# selenosis
Scalable, stateless selenium hub for Kubernetes cluster.

## Overview
### Available flags
```
[user@host]$ ./selenosis --help
Scallable, stateless selenium grid for Kubernetes cluster

Usage:
  selenosis [flags]

Flags:
      --port string                          port for selenosis (default ":4444")
      --proxy-port string                    proxy continer port (default "4445")
      --browsers-config string               browsers config (default "./config/browsers.yaml")
      --browser-limit int                    active sessions max limit (default 10)
      --namespace string                     kubernetes namespace (default "selenosis")
      --service-name string                  kubernetes service name for browsers (default "seleniferous")
      --browser-wait-timeout duration        time in seconds that a browser will be ready (default 30s)
      --session-wait-timeout duration        time in seconds that a session will be ready (default 1m0s)
      --session-idle-timeout duration        time in seconds that a session will idle (default 5m0s)
      --session-retry-count int              session retry count (default 3)
      --graceful-shutdown-timeout duration   time in seconds  gracefull shutdown timeout (default 30s)
      --image-pull-secret-name string        secret name to private registry
      --proxy-image string                   in case you use private registry replace with image from private registry (default "alcounit/seleniferous:latest")
  -h, --help                                 help for selenosis

```

### Available endpoints
| Protocol | Endpoint                    |
|--------- |---------------------------- |
| HTTP    | /wd/hub/session              |
| HTTP    | /wd/hub/session/{sessionId}/ |
| HTTP    | /wd/hub/status               |
| WS      | /vnc/{sessionId}             |
| WS/HTTP | /devtools/{sessionId}        |
| HTTP    | /download/{sessionId}        |
| HTTP    | /clipboard/{sessionId}       |
| HTTP    | /status                      |
| HTTP    | /healthz                     |
<br/>

## Configuration
Selenosis requires config to start browsers in K8 cluster. Config can be JSON or YAML file.<br/>
Basic configuration be like (all fields in this example are mandatory):

```json
{
    "chrome": {
        "defaultVersion": "85.0",
        "path": "/",
        "versions": {
            "85.0": {
                "image": "selenoid/vnc:chrome_85.0"
            },
            "86.0": {
                "image": "selenoid/vnc:chrome_86.0"
            }
        }
    },
    "firefox": {
        "defaultVersion": "82.0",
        "path": "/wd/hub",
        "versions": {
            "81.0": {
                "image": "selenoid/vnc:firefox_81.0"
            },
            "82.0": {
                "image": "selenoid/vnc:firefox_82.0"
            }
        }
    },

    "opera" : {
        "defaultVersion": "70.0",
        "path": "/",
        "versions": {
            "70.0": {
                "image": "selenoid/vnc:opera_70.0"
            },
            "71.0": {
                "image": "selenoid/vnc:opera_71.0"
            }
        }
    }
}
```
``` yaml
---
chrome:
  defaultVersion: "85.0"
  path: "/"
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    '86.0':
      image: selenoid/vnc:chrome_86.0
firefox:
  defaultVersion: "82.0"
  path: "/wd/hub"
  versions:
    '81.0':
      image: selenoid/vnc:firefox_81.0
    '82.0':
      image: selenoid/vnc:firefox_82.0
opera:
  defaultVersion: "70.0"
  path: "/"
  versions:
    '70.0':
      image: selenoid/vnc:opera_70.0
    '71.0':
      image: selenoid/vnc:opera_71.0
```
Browser name and browser version are taken from Selenium desired capabilities.<br/>

Each browser can have default <b>spec/annotations/labels</b>, they will merged to all browsers listed in the <b>versions</b> section.

``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "meta": {
      "labels": {
        "environment": "aqa",
        "app": "myCoolApp"
      },
      "annotations": {
        "build": "dev-v1.11.2",
        "builder": "jenkins"
      }
    },
    "spec": {
      "resources": {
        "requests": {
          "memory": "500Mi",
          "cpu": "0.5"
        },
        "limits": {
          "memory": "1000Gi",
          "cpu": "1"
        }
      },
      "hostAliases": [
        {
          "ip": "127.0.0.1",
          "hostnames": [
            "foo.local",
            "bar.local"
          ]
        },
        {
          "ip": "10.1.2.3",
          "hostnames": [
            "foo.remote",
            "bar.remote"
          ]
        }
      ],
      "env": [
        {
          "name": "TZ",
          "value": "Europe/Kiev"
        },
        {
          "name": "SCREEN_RESOLUTION",
          "value": "1920x1080x24"
        },
        {
          "name": "ENABLE_VNC",
          "value": "true"
        }
      ]
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0"
      },
      "86.0": {
        "image": "selenoid/vnc:chrome_86.0"
      }
    }
  }
}
```

``` yaml
---
chrome:
  defaultVersion: "85.0"
  path: "/"
  meta:
    labels:
      environment: aqa
      app: myCoolApp
    annotations:
      build: dev-v1.11.2
      builder: jenkins
  spec:
    resources:
      requests:
        memory: 500Mi
        cpu: '0.5'
      limits:
        memory: 1000Gi
        cpu: '1'
    hostAliases:
    - ip: 127.0.0.1
      hostnames:
      - foo.local
      - bar.local
    - ip: 10.1.2.3
      hostnames:
      - foo.remote
      - bar.remote
    env:
    - name: TZ
      value: Europe/Kiev
    - name: SCREEN_RESOLUTION
      value: 1920x1080x24
    - name: ENABLE_VNC
      value: 'true'
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    '86.0':
      image: selenoid/vnc:chrome_86.0
```
You can override default browser <b>spec/annotation/labels</b> by providing individual <b>spec/annotation/labels</b> to browser version
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "meta": {
      "labels": {
        "environment": "aqa",
        "app": "myCoolApp"
      },
      "annotations": {
        "build": "dev-v1.11.2",
        "builder": "jenkins"
      }
    },
    "spec": {
      "resources": {
        "requests": {
          "memory": "500Mi",
          "cpu": "0.5"
        },
        "limits": {
          "memory": "1000Gi",
          "cpu": "1"
        }
      },
      "hostAliases": [
        {
          "ip": "127.0.0.1",
          "hostnames": [
            "foo.local",
            "bar.local"
          ]
        },
        {
          "ip": "10.1.2.3",
          "hostnames": [
            "foo.remote",
            "bar.remote"
          ]
        }
      ],
      "env": [
        {
          "name": "TZ",
          "value": "Europe/Kiev"
        },
        {
          "name": "SCREEN_RESOLUTION",
          "value": "1920x1080x24"
        },
        {
          "name": "ENABLE_VNC",
          "value": "true"
        }
      ]
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0",
        "spec": {
          "resources": {
            "requests": {
              "memory": "750Mi",
              "cpu": "0.5"
            },
            "limits": {
              "memory": "1500Gi",
              "cpu": "1"
            }
          }
        }
      },
      "86.0": {
        "image": "selenoid/vnc:chrome_86.0",
        "spec": {
          "hostAliases": [
            {
              "ip": "127.0.0.1",
              "hostnames": [
                "bla-bla.com"
              ]
            }
          ]
        },
        "meta": {
          "labels": {
            "environment": "dev",
            "app": "veryCoolApp"
          }
        }
      }
    }
  }
}
```
``` yaml
---
chrome:
  defaultVersion: "85.0"
  path: "/"
  meta:
    labels:
      environment: aqa
      app: myCoolApp
    annotations:
      build: dev-v1.11.2
      builder: jenkins
  spec:
    resources:
      requests:
        memory: 500Mi
        cpu: '0.5'
      limits:
        memory: 1000Gi
        cpu: '1'
    hostAliases:
    - ip: 127.0.0.1
      hostnames:
      - foo.local
      - bar.local
    - ip: 10.1.2.3
      hostnames:
      - foo.remote
      - bar.remote
    env:
    - name: TZ
      value: Europe/Kiev
    - name: SCREEN_RESOLUTION
      value: 1920x1080x24
    - name: ENABLE_VNC
      value: 'true'
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
      spec:
        resources:
          requests:
            memory: 750Mi
            cpu: '0.5'
          limits:
            memory: 1500Gi
            cpu: '1'
    '86.0':
      image: selenoid/vnc:chrome_86.0
      spec:
        hostAliases:
        - ip: 127.0.0.1
          hostnames:
          - bla-bla.com
      meta:
        labels:
          environment: dev
          app: veryCoolApp

```
## Deployment
Files and steps required for selenosis deployment available in [selenosis-deploy](https://github.com/alcounit/selenosis-deploy) repository

 ## Run yout tests
 ``` java
 DesiredCapabilities capabilities = new DesiredCapabilities();
capabilities.setBrowserName("chrome");
capabilities.setVersion("85.0");

RemoteWebDriver driver = new RemoteWebDriver(
    URI.create("http://<loadBalancerIP|nodeIP>:<port>/wd/hub").toURL(), 
    capabilities
);
 ```
  ``` python
from selenium import webdriver
        
capabilities = {
    "browserName": "chrome",
    "version": "85.0",
}

driver = webdriver.Remote(
    command_executor="http://<loadBalancerIP|nodeIP>:<port>/wd/hub",
    desired_capabilities=capabilities)
 ```

 Note: you can omit browser version in your desired capabilities, make sure you set defaultVersion property in the config file.


## Browser pods are deleted right after start
Depends on you cluster version in some cases you can face with issue when some browser pods are deleted right after their start and selenosis log will contains lines like this:
```log
time="2020-12-21T10:28:20Z" level=error msg="session failed: Post \"http://selenoid-vnc-chrome-87-0-af3177a0-5052-45be-b4e4-9462146e4633.seleniferous:4445/wd/hub/session\": dial tcp: lookup selenoid-vnc-chrome-87-0-af3177a0-5052-45be-b4e4-9462146e4633.seleniferous on 10.96.0.10:53: no such host" request="POST /wd/hub/session" request_id=fa150040-86c1-4224-9e5c-21416b1d9f5c time_elapsed=5.73s
```
To fix this issue do the following:
```bash
kubectl edit cm coredns -n kube-system
```
add following record to your coredns config
```config
    selenosis.svc.cluster.local:53 {
        errors
        kubernetes cluster.local {
          namespaces selenosis
        }
    }
```
this option will turn off dns caching for selenosis namespace, resulting config update should be as following:
```yaml
apiVersion: v1
data:
  Corefile: |
    selenosis.svc.cluster.local:53 {
        errors
        kubernetes cluster.local {
          namespaces selenosis
        }
    }
    .:53 {
        errors
        health
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods insecure
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf
        cache 30
        loop
        reload
        loadbalance
        import custom/*.override
    }
    import custom/*.server
kind: ConfigMap
...
```

## Features
### Scalability
By default selenosis starts with 2 replica sets. To change it, edit selenosis deployment file: <b>03-selenosis.yaml</b>
``` yaml

apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: selenosis
  namespace: selenosis
spec:
  replicas: 2
  selector:
...
```
<br/>
by using kubectl

```bash
kubectl scale deployment selenosis -n selenosis --replicas=3
```

### Stateless
Selenosis doesn't store any session info. All connections to the browsers are automatically assigned via headless service.

### Hot config reload
Selenosis supports hot config reload, to do so update you configMap
```bash
kubectl edit configmap -n selenosis selenosis-config -o yaml
```

### UI for debug
Selenosis itself doesn't have ui. If you need such functionality you can use [selenoid-ui](https://github.com/aerokube/selenoid-ui) with special [adapter container](https://github.com/alcounit/adaptee). 
Deployment steps and minifests you can find in [selenosis-deploy](https://github.com/alcounit/selenosis-deploy) repository.

Currently this project is under development and can be unstable, in case of any bugs or ideas please report
