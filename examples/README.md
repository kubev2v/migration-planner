# Demo flow

## Agent API
On your laptop run agent instance:
```
$ make build
$ make deploy-db
$ bin/planner-api &
$ bin/planner create source mysource
```

## Gather the OVA from API
Now you can create the OVA and gather it from the agent API:

```
SOURCE_ID=`bin/planner get source -o json | jq '.[].id'`
$ curl -v http://127.0.0.1:3443/api/v1/sources/$SOURCE_ID/image -o myova.ova
```

## Create VM from OVA in the VMware
Follow the guide from [documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vm_admin.doc/GUID-17BEDA21-43F6-41F4-8FB2-E01D275FE9B4.html).

## Input the credentials
Open your browser put the VM IP `https://VM_IP:8443` put the crendentials of VMware environment.

## See the results
Wait for the script to finish. After script is finished you can see the inventory as follows:

```
$ bin/planner get source mysoruce -o yaml
```
