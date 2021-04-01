![GitHub release (latest by date)](https://img.shields.io/github/v/release/alcounit/selenosis)
![Docker Pulls](https://img.shields.io/docker/pulls/alcounit/selenosis)
![GitHub](https://img.shields.io/github/license/alcounit/selenosis)
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

## Features
### Scalability
By default selenosis starts with 2 replica sets. To change it, edit selenosis deployment file: <b>[03-selenosis.yaml](https://github.com/alcounit/selenosis-deploy/blob/main/03-selenosis.yaml)</b>
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
When a new session request is received, selenosis creates a pod with 2 containers, one is a browser and the second is a lightweight sidecar called [seleniferous](https://github.com/alcounit/seleniferous). 
Seleniferous proxies all requests to the browser and replaces original sessionId returned by the browser with pod hostname. All other requests received by selenosis just proxied to the existing pod by using sessionId and [headless service](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) as a hostname.

### Hot config reload
Selenosis supports hot config reload, to do so update you configMap
```bash
kubectl edit configmap -n selenosis selenosis-config -o yaml
```

### UI for debug
Selenosis itself doesn't have ui. If you need such functionality you can use [selenoid-ui](https://github.com/aerokube/selenoid-ui) with special [adapter container](https://github.com/alcounit/adaptee). 
Deployment steps and minifests you can find in [selenosis-deploy](https://github.com/alcounit/selenosis-deploy) repository.


## Configuration
Selenosis can run any docker image with browser but best work with images debeloped by Aerokube:
<br>
* [Android](https://aerokube.com/images/latest/#_android)
* [Chrome](https://aerokube.com/images/latest/#_chrome)
* [Firefox](https://aerokube.com/images/latest/#_firefox)
* [Microsoft Edge](https://aerokube.com/images/latest/#_microsoft_edge)
* [Opera](https://aerokube.com/images/latest/#_opera)

### Basic config
To start browsers in kubernetes cluster you will need config, config can be JSON or YAML file.<br/>
Browser name and browser version shoud be passed via selenium desired capabilities.<br/>
Basic configuration is be like (all fields in this example are mandatory):

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

Each browser type can have default spec/meta sections, same is applied to individual browser versions. Some properties like annotations and labels will be merged to specific browser version, others like resources stay unchanged(can't be overriden). Specific browser version properties have a higher priority on merge.
### Managing Resources
[CPU and Memory limits](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) can be set globally to specific browser type or individually to specific browser version. </br>
In the example below chrome browser v86.0 pod will be launched with resource limits that are set globally and browser v85.0 pod will be launched with individual resource limits that are set in browser version spec section.

``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "spec": {
      "resources": {
        "requests": {
          "memory": "500Mi",
          "cpu": "0.5"
        },
        "limits": {
          "memory": "1Gi",
          "cpu": "1"
        }
      }
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
              "memory": "1.5Gi",
              "cpu": "1"
            }
          }
        }
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
  spec:
    resources:
      requests:
        memory: 500Mi
        cpu: '0.5'
      limits:
        memory: 1Gi
        cpu: '1'
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
      spec:
        resources:
          requests:
            memory: 750Mi
            cpu: '0.5'
          limits:
            memory: 1.5Gi
            cpu: '1'
    '86.0':
      image: selenoid/vnc:chrome_86.0

```

### Labels and annotations
[Labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/) and [annotations](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/) are supported by config and can be added globally or individually depends on your requirements.
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
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0"
      },
      "86.0": {
        "image": "selenoid/vnc:chrome_86.0",
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
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    '86.0':
      image: selenoid/vnc:chrome_86.0
      meta:
        labels:
          environment: dev
          app: veryCoolApp

```

### Adding Host Aliases
You can add the [host name and aliases](https://kubernetes.io/docs/concepts/services-networking/add-entries-to-pod-etc-hosts-with-host-aliases/) to /etc/hosts file by using hostAliases property.
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "spec": {
      "hostAliases": [
        {
          "ip": "127.0.0.1",
          "hostnames": [
            "foo.local",
            "bar.local"
          ]
        }
      ]
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0",
      "spec": {
        "hostAliases": [
          {
            "ip": "10.1.2.3",
            "hostnames": [
            "foo.remote",
            "bar.remote"
            ]
          }
        ]
      },
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
  spec:
    hostAliases:
    - ip: 127.0.0.1
      hostnames:
      - foo.local
      - bar.local
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    spec:
      - ip: 10.1.2.3
        hostnames:
        - foo.remote
        - bar.remote
    '86.0':
      image: selenoid/vnc:chrome_86.0
```

### Environment Variables
You can set [environment variables](https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/) for browser pods. To set environment variables, include the env in the configuration file.
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "spec": {
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
          "env": [
            {
              "name": "SCREEN_RESOLUTION",
              "value": "1024x768x24"
            }
          ]
        }
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
  spec:
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
      env:
      - name: SCREEN_RESOLUTION
        value: 1024x768x24
    '86.0':
      image: selenoid/vnc:chrome_86.0
```

### Mounting volumes to a browser pod
If you need a [directory](https://kubernetes.io/docs/concepts/storage/volumes/) with a data that is accessible to the browser use volume and volumeMount properties in your config
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "volumes": [
      {
        "name": "simple-vol",
        "emptyDir": {}
      }
    ],
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0",
        "spec": {
          "volumeMounts": [
            {
              "name": "simple-vol",
              "mountPath": "/var/simple"
            }
          ]
        }
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
  defaultVersion: '85.0'
  path: /
  volumes:
    - name: simple-vol
      emptyDir: {}
  versions:
    '85.0':
      image: 'selenoid/vnc:chrome_85.0'
    spec:
      volumeMounts:
        - name: simple-vol
          mountPath: /var/simple
    '86.0':
      image: 'selenoid/vnc:chrome_86.0'
```



### Assigning Browsers to Nodes
You can constrain a browser pods to only be able [to run on particular node(s)](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/), or to prefer to run on particular nodes. To do so add a nodeSelector property to your configuration.
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "spec": {
      "nodeSelector": {
        "disktype": "ssd"
      }
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0",
        "spec": {
          "nodeSelector": {
            "nodeType": "N2D"
          }
        }
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
  spec:
    nodeSelector:
      disktype: N2D
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    spec:
      nodeSelector:
        nodeType: ssd
    '86.0':
      image: selenoid/vnc:chrome_86.0
```

### Node affinity
To attract browser pods to [set of nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) use tolerations property
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "spec": {
      "tolerations": [
        {
          "key": "example-key",
          "operator": "Exists",
          "effect": "NoSchedule"
        }
      ]
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0"
      },
      "spec": {
        "tolerations": [
          {
            "key": "key1",
            "operator": "Equal",
            "value": "value1",
            "effect": "NoSchedule"
          }
        ]
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
  spec:
    tolerations:
    - key: "example-key"
      operator: "Exists"
      effect: "NoSchedule"
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    spec:
      tolerations:
      - key: "key1"
        operator: "Equal"
        value: "value1"
        effect: "NoSchedule"
    '86.0':
      image: selenoid/vnc:chrome_86.0
```

## Custom UID and GID for browser pod
Browser pod can be run with custom UID and GID. To do so set runAs property for specific browser globally or per each browser version.
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "runAs": {
      "uid": 1000,
      "gid": 2000
    },
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0"
      },
      "runAs": {
        "uid": 1001,
        "gid": 2002
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
  defaultVersion: '85.0'
  path: "/"
  runAs:
    uid: 1000
    gid: 2000
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    runAs:
      uid: 1001
      gid: 2002
    '86.0':
      image: selenoid/vnc:chrome_86.0
```

## Custom Kernel Capabilities
In some cases you may need to run browser container with custom Linux capabilities. To do so set kernelCaps property for specific browser globally or per each browser version.
``` json
{
  "chrome": {
    "defaultVersion": "85.0",
    "path": "/",
    "kernelCaps": ["SYS_ADMIN"],
    "versions": {
      "85.0": {
        "image": "selenoid/vnc:chrome_85.0"
      },
      "kernelCaps": ["SYS_ADMIN"],
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
  defaultVersion: '85.0'
  path: "/"
  kernelCaps:
  - SYS_ADMIN
  versions:
    '85.0':
      image: selenoid/vnc:chrome_85.0
    kernelCaps:
    - SYS_ADMIN
    '86.0':
      image: selenoid/vnc:chrome_86.0
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

List of capabilities required for selenoid-ui compatibility:
| key              | type    | description              |
|----------------- |-------- |------------------------- |
| enableVNC        | boolean | enables VNC support      |
| name             | string  | name of test             |
| screenResolution | string  | custom screen resolution |

</br>
 Note: you can omit browser version in your desired capabilities, make sure you set defaultVersion property in the config file.
</br></br>

## Known issues
### Browser pods are deleted right after start
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
Currently this project is under development and can be unstable, in case of any bugs or ideas please report