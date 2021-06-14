# External builder for Fabric on Kubernetes

** **Work based on [hlfabric-k8scc](https://github.com/postfinance/hlfabric-k8scc)**

## Introduction
Running Hyperledger Fabric on Kubernetes doesn't allow observability on the chaincodes, which includes, building and executing them. To solve this, we can use external builders and launchers to adapt the way HLF launches the chaincodes.

To read more about this topic [please visit the official documentation](https://hyperledger-fabric.readthedocs.io/en/release-2.2/cc_launcher.html). 

## What is different from *hlfabric-k8scc*
This external builder doesn't relay on a `ReadWriteMany` persistent volume claim, rather, it depends on a shared file server where the code and the build output will be stored.

`ReadWriteMany` is not supported on many cloud providers, and of course not supported by test environments such as [KIND](https://github.com/kubernetes-sigs/kind/issues/1487)

In order to use it, you can either use one of the public images on [quay.io](https://quay.io/repository/kfsoftware/fabric-peer?tab=tags) or build the image by yourself.

## Features
- [x] Build chaincode for Golang
- [x] Build chaincode for NodeJS
- [x] Build chaincode for Java
- [x] Proxy support

## Roadmap
- [ ] Cache builds between peers in the same cluster
- [ ] Integration testing

## Components
External chaincode builder on Kubernetes needs a http server to store the chaincode inputs to compile them in a Pod, and to store the artifacts of the chaincode built. 

The contents of this server can be found [here](./cmd/fileserver/fileserver.go)

## Build

### For HLF 2.2.0
```bash
docker build -t quay.io/kfsoftware/fabric-peer:amd64-2.2.0 -f ./images/fabric-peer/2.2.0/Dockerfile ./
docker push quay.io/kfsoftware/fabric-peer:amd64-2.2.0
```

### For HLF 2.3.0
```bash
docker build -t quay.io/kfsoftware/fabric-peer:amd64-2.3.0 -f ./images/fabric-peer/2.3.0/Dockerfile ./
docker push quay.io/kfsoftware/fabric-peer:amd64-2.3.0
```

### File Server
```bash
docker build -t quay.io/kfsoftware/fabric-fs:amd64-2.2.0 -f ./Dockerfile ./
docker push quay.io/kfsoftware/fabric-fs:amd64-2.2.0
```

## Configure

Inside the **core.yaml** of the peer, under chaincode, there's a property called ``externalBuilders```.

If using the the image provided by this project, the contents of ```externalBuilders``` are the following:
```yaml
- name: k8s-builder
  path: /builders/golang
  propagateEnvironment:
    - CHAINCODE_SHARED_DIR
    - FILE_SERVER_BASE_IP
    - KUBERNETES_SERVICE_HOST
    - KUBERNETES_SERVICE_PORT
    - K8SCC_CFGFILE
    - TMPDIR
    - LD_LIBRARY_PATH
    - LIBPATH
    - PATH
    - EXTERNAL_BUILDER_HTTP_PROXY
    - EXTERNAL_BUILDER_HTTPS_PROXY
    - EXTERNAL_BUILDER_NO_PROXY
    - EXTERNAL_BUILDER_PEER_URL

```

### Behind a proxy

You have to build your own image with your own **k8scc.yaml**
```yaml

---
images:
  golang: "hyperledger/fabric-ccenv:2.2.0"
  java: "hyperledger/fabric-javaenv:2.2.0"
  node: "hyperledger/fabric-nodeenv:2.2.0"

builder:
  resources:
    memory_limit: "0.5G"
    cpu_limit: "0.2"

  env:
    - HTTP_PROXY=http://my-enterprise-proxy:1234
    - HTTPS_PROXY=http://my-enterprise-proxy:1234
    - NO_PROXY=home.com,example.com

launcher:
  resources:
    memory_limit: "0.5G"
    cpu_limit: "0.2"

```