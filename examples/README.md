# Demo flow

## Agent API
On your laptop run agent instance:
```
$ make build
$ make deploy-db
$ bin/planner-api &
$ bin/planner create source mysource
```

## Ignition
Modify the ignition to have the Agent IP in config.yml.

## Create & Run ISO
```
# Download rhcos-live
$ curl -O https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live.x86_64.iso

# Generate RHCOS ISO with ignition
$ podman run --privileged --rm --env-host -v /dev:/dev -v /run/udev:/run/udev -v $PWD:/data -w /data quay.io/coreos/coreos-installer:release iso ignition embed -fi config.ign -o /data/coreos.iso rhcos-live.x86_64.iso

# Move the ISO to the path that will be used by virt-install
$ cp coreos.iso ~/Downloads/

$ sudo virt-install --name coreos-vm --memory 4096 --vcpus 2 --disk path=/home/omachace/coreos.qcow,size=20,format=qcow2 --cdrom /home/omachace/coreos.iso --os-variant fedora-coreos-stable --boot hd,cdrom --network network=default --graphics vnc,listen=0.0.0.0
```

## OVA
Rename `coreos.iso` to `AgentVM-1.iso` and build OVA archive, which you can later upload.

```
tar -cvf AgentVM.ova AgentVM-1.iso AgentVM.ovf
```

## Script to create OVA
There is a script called `createova.sh`, which does the job for you. It download the RHCOS ISO and bundle the ignition based on your SSH key and IP address of Agent service.
To execute it just run:
```
./createova.sh AGENT_SERVICE_IP PATH_TO_SSH_PUB_KEY
```
For example `./createova.sh 10.0.0.2 ~/.ssh/id_rsa.pub`. It will generate `AgentVM.ova` file in current this directory.

## Input the credentials
Open your browser put the VM IP `https://VM_IP:8443` put the crendentials of VMware environment.
Then wait for the script to finish. After script is finished you can see the inventory as follows:

```
$ bin/planner get source mysoruce -o yaml
```
