# selenosis
Scalable, stateless selenium hub for Kubernetes cluster

## Configuration

Selenosis requires config to start browsers in K8 cluster. Config can be JSON or YAML file.<br/>
Basic configuration be like (all fields in this example are mandatory):

```json
{
    "chrome": {
        "path": "/",
        "versions": {
            "85.0": {
                "image": "selenoid/vnc:chrome:85.0"
            },
            "86.0": {
                "image": "selenoid/vnc:chrome:86.0"
            }
        }
    },
    "firefox": {
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
  path: "/"
  versions:
    '85.0':
      image: selenoid/vnc:chrome:85.0
    '86.0':
      image: selenoid/vnc:chrome:86.0
firefox:
  path: "/wd/hub"
  versions:
    '81.0':
      image: selenoid/vnc:firefox_81.0
    '82.0':
      image: selenoid/vnc:firefox_82.0
opera:
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
    "path": "/",
    "labels": {
      "environment": "aqa",
      "app": "myCoolApp"
    },
    "annotations": {
      "build": "dev-v1.11.2",
      "builder": "jenkins"
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
        "image": "selenoid/vnc:chrome:85.0"
      },
      "86.0": {
        "image": "selenoid/vnc:chrome:86.0"
      }
    }
  }
}
```

``` yaml
---
chrome:
  path: "/"
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
      image: selenoid/vnc:chrome:85.0
    '86.0':
      image: selenoid/vnc:chrome:86.0
```
You can override default browser <b>spec/annotation/labels</b> by providing individual <b>spec/annotation/labels</b> to browser version
``` json
{
  "chrome": {
    "path": "/",
    "labels": {
      "environment": "aqa",
      "app": "myCoolApp"
    },
    "annotations": {
      "build": "dev-v1.11.2",
      "builder": "jenkins"
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
        "image": "selenoid/vnc:chrome:85.0",
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
        "image": "selenoid/vnc:chrome:86.0",
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
        "labels": {
          "environment": "dev",
          "app": "veryCoolApp"
        }
      }
    }
  }
}
```
``` yaml
---
chrome:
  path: "/"
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
      image: selenoid/vnc:chrome:85.0
      spec:
        resources:
          requests:
            memory: 750Mi
            cpu: '0.5'
          limits:
            memory: 1500Gi
            cpu: '1'
    '86.0':
      image: selenoid/vnc:chrome:86.0
      spec:
        hostAliases:
        - ip: 127.0.0.1
          hostnames:
          - bla-bla.com
      labels:
        environment: dev
        app: veryCoolApp

```
### Instalation

Clone deployment files
```
git clone https://github.com/alcounit/selenosis-deploy.git && cd selenosis-deploy
```

Create namespace
```
 kubectl apply -f 01-namespace.yaml
```

Create config map with config for browsers
```
 kubectl apply -f 02-configmap.yaml
```

Create kubernetes service
```
 kubectl apply -f 03-service.yaml
 ```

 Deploy selenosis
 ```
 kubectl apply -f 04-selenosis.yaml
 ```
Browser images prepull (optional)
```
kubectl apply -f 05-image-prepull.yaml
```

### Scalability
By default selenosis starts with 2 replica sets. To change it, edit selenosis deployment file: <b>04-selenosis.yaml</b>
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

### Stateless
selenosis doesn't not store session state. All connections to the browsers are automatically assigned via headless service.