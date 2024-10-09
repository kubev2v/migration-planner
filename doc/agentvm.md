# Agent virtual machine
The agent, based on Red Hat CoreOS (RHCOS), communicates with the Agent Service and reports its status.
The agent virtual machine is initialized using ignition, which configures multiple containers that run as systemd services. Each of these services is dedicated to a specific function.

## Systemd services
Follows the list of systemd services that can be found on agent virtual machine. All of the services
are defined as quadlets. Quadlet configuration can be found in the [ignition template file](../data/config.ign.template).
Agent dockerfile can be found [here](../Containerfile.agent), the collector containerfile is [here](../Containerfile.collector).

### planner-setup
Planner-setup service is responsible for inicializing the volume with data, that are shared between `planner-agent` and `planner-agent-collector`.

### planner-agent
Planner-agent is a service that reports the status to the Agent service. The URL of the Agent service is configured in `$HOME/vol/config.yaml` file, which is injected via ignition.

Planner-agent contains web application that is exposed via port 3333. Once user access the web app and enter the credentials of the vCenter, `credentials.json` file is created in the shared volume, and `planner-agent-collector` can be spawned.

### planner-agent-opa
Planner-agent-opa is a service that re-uses [forklift validation](https://github.com/kubev2v/forklift/blob/main/validation/README.adoc) container. The forklift validation container is responsible for vCenter data validation. When `planner-agent-collector` fetch vCenter data it's validated against the OPA server and report is shared back to Agent Service.

### planner-agent-collector
Planner-agent-collector service waits until user enter vCenter credentials, once credentials are entered the vCenter data are collected. The data are stored in `$HOME/vol/data/inventory.json`. Once `invetory.json` is created `planner-agent` service send the data over to Agent service.

### podman-auto-update
Podman auto update is responsible for updating the image of containers in case there is a new release of the image. We use default `podman-auto-update.timer`, which executes `podman-auto-update` every 24hours.

## Troubleshooting Agent VM services
Usefull commands to troubleshoot Agent VM. Note that all the containers are running under `core` user.

### Listing the running podman containers
```
$ podman ps
```

### Checking the status of all our services
```
$ systemctl --user status planner-*
```

### Inspecting the shared volume
We create a shared volume between containers, so we can share information between collector and agent container.
In order to expore the data stored in the volume find the mountpoint of the volume:
```
$ podman volume inspect planner.volume | jq .[0].Mountpoint
```

And then you can explore relevant data. Like `config.yaml`, `credentials.json`, `inventory.json`, etc.
```
$ ls /var/home/core/.local/share/containers/storage/volumes/planner.volume/_data
$ cat /var/home/core/.local/share/containers/storage/volumes/planner.volume/_data/config.yaml
$ cat /var/home/core/.local/share/containers/storage/volumes/planner.volume/_data/data/credentials.json
$ cat /var/home/core/.local/share/containers/storage/volumes/planner.volume/_data/data/inventory.json
```

### Inspecting the host directory with data
The ignition create a `vol` directory in `core` user home directory.
This directory should contain all relevant data, so in order to find misconfiguration please search in this directory.
```
$ ls -l vol
```

### Check logs of the services
```
$ journalctl --user -f -u planner-*
```

### Status is `Not connected` after VM is booted.
This isually indicates that `planner-agent` service can't communicate with the Agent service.
Check the logs of the `planner-agent` service:
```
journalctl --user -f -u planner-agent
```
And search for the error in the log:
```
level=error msg="failed connecting to migration planner: dial tcp: http://non-working-ip:7443
```
Make sure `non-working-ip` has properly setup Agent service and is listening on port `7443`.
