# CSI-based tool provider

This CSI driver implements the POC of a CSI-based tool provider.
It allows mounting in PODs, in read-only mode and as CSI volumes, the content of container images
gathered as software catalogs.

The idea is to allow "injecting" a Java, Maven or any other self-contained tool installation
into an existing container, thanks to a CSI volume.

#### More details

TBD

#### How to test it

Since it is designed to leverage Kubernetes clusters that use an OCI-compatible engine,
testing it on CRC is recommended.

##### To install it on CRC:

- First add the csi volumes to the lisyt of volumes allowed in the `restricted` SCC:

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
