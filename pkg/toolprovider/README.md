# CSI tool provider driver

## Usage:

### Build toolproviderplugin
```
$ make push
```

### Start tool provider driver
```
$ sudo ./bin/toolproviderplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v=6
```

### Test using csc
Get ```csc``` tool from https://github.com/rexray/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"toolprovider.csi.davidfestal"  "0.1.0"
```

#### NodePublish a volume
```
$ csc node publish --endpoint tcp://127.0.0.1:10000 --cap 1,block --target-path /mnt/hostpath CSIVolumeID
CSIVolumeID
```

#### NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/hostpath CSIVolumeID
CSIVolumeID
```

#### Get NodeInfo
```
$ csc node get-info --endpoint tcp://127.0.0.1:10000
CSINode
```
