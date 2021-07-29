# CSI-based tool provider

This CSI driver implements the POC of a CSI-based tool provider. It is the first component of the **Katalogos** project,
whose goal is to provide Composable Software Catalogs on Kubernetes.

It allows injecting into PODs, as read-only mounted CSI volumes, the content of container images
gathered as software catalogs.

The idea is to allow "injecting" a Java, Maven or any other self-contained tool installation
into an existing container, thanks to an inline ephemeral CSI volume.

## What does it solve ?

#### A problem

Instead of having to bundle every possible tool inside a container **at image build time**...

![Katalogos - The problem](https://user-images.githubusercontent.com/686586/127527553-1ba0c00f-e70a-4ade-8c08-1e532fec8b83.png)

#### A solution

... why not allow injecting each containerized tool seperately **at runtime**, thus allowing **composition**, **better image reuse**, and **reducing image-build burden**:

![Katalogos - The solution](https://user-images.githubusercontent.com/686586/127528197-054ec139-b341-4902-8119-cc8d15f86a8c.png)

## How to test it

#### Install the CSI driver

- Ensure you are logged to your Kubernetes cluster as `cluster-admin` and use the `default` namespace 

- On OpenShift (or CRC), as well as any Kubernetes distribution that doesn't allow CSI inline volumes in PODs by default, you should first add the `csi` volumes to the list of volumes allowed in the `restricted` SCC:

```bash
kubecly patch scc restricted --type='json' -p='[{"op": "add", "path": "/volumes/-", "value": "csi"}]'
```
- Apply the deployment YAML files:

```bash
kubectl apply -f deploy/pull-direct
```
This will deploy the `katalogos-csi-toolprovider` daemonset.

#### The Quarkus Getting Started example

This example injects the JDK and Maven, through the CSI driver, inside a raw Linux container, and
runs the standard Quarkus getting started example on top of this.
This allows switching the underlying container linux distribution without having to rebuild anything.

*Note*: This sample will be easier to test on OpenShift since, for the sake of simplicity, it defines a route to provide access to the Quarkus web application.

- Apply the following example YAML files:

```bash
kubectl apply -f examples/quarkus-quickstart/
```
- You can go to the POD logs, and see that maven started downloading the dependencies
- After a while, the eb application is available at the related route URL

- To change the linux distribution to ubuntu, and see that the POD works the same, just do:

```bash
kubectl patch deployment/csi-based-tool-provider-test --patch '{ "spec": { "template": { "spec" : { "containers": [ { "name": "main", "image": "ubuntu" } ] } } } }'
```

#### The Bash - Coreutils example

This example showcases how to inject statically-linked `bash` and `coreutils` binaries
inside a `from scratch` container that contains precisely nothing.

- Apply the following example YAML files:

```bash
kubectl apply -f examples/bash-coreutils-on-scratch-image.yaml
```

When the example is run, you can exec one of those injected binaries, ike `uname` for example:

```bash
kubectl exec <example pod name> -- uname -a
```
