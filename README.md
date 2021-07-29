# CSI-based tool provider

This CSI driver implements the POC of a CSI-based tool provider.
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

Since it is designed to leverage Kubernetes clusters that use an OCI-compatible engine,
testing it on CRC is recommended.

##### To install it on CRC:

- First add the csi volumes to the list of volumes allowed in the `restricted` SCC:

```bash
oc patch scc restricted --type='json' -p='[{"op": "add", "path": "/volumes/-", "value": "csi"}]'
```

- Apply the deployment YAML files:

```bash
oc project default
oc apply -f deploy/
```

This will deploy the `csi-toolprovider` daemonset.

###### Deploy the Software Catalog pull Job

- Apply the following Cron Job YAML definition:

```bash
oc apply -f examples/catalog/catalog-pull-cron-job.yaml
```

Note: You may want to customize the Cron Job schedule, which for the sake of the tests is every minute.

###### Deploy the Quarkys getting started example:

- Apply the following example YAML files:

```bash
oc apply -f examples/quarkus-quickstart/
```
