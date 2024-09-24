# Minimum permissions for the agent account

The **agent** only gathers information from vCenter, therefore a **read only** permission whould be enough. 

One such role is the [Read Only Role](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.security.doc/GUID-93B962A7-93FA-4E96-B68F-AE66D3D6C663.html).

This permission needs to be applied at the root object level with "Propagate to children" selected.

> Remark: The **Read Only** role is not sufficient for MTV operator.

## Credentials to log in to vCenter

To enable the **agent** to log in to vCenter, `username` and `password` must be supplied. 
Unfortunately, a `vCenter token` is not supported due to a Golang limitation [vSphere client](https://pkg.go.dev/github.com/vmware/govmomi#Client.Login).
