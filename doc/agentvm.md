# Agent virtual machine
The agent virtual machine, based on Red Hat CoreOS (RHCOS), communicates with the Agent Service and reports its status.
The VM is initialized using ignition, which configures multiple containers that run as systemd services. Each of these services is dedicated to a specific function.

## Systemd services
The following are a list of systemd services that can be found on agent virtual machines. All of the services
are defined as quadlets. Quadlet configuration can be found in the [ignition template file](../data/ignition.template).
The Agent containerfile can be found [here](../Containerfile.agent).

### planner-agent
Planner-agent is a service that reports the status to the Agent service. The URL of the Agent service is configured in the file `$HOME/.migration-planner/config/config.yaml`, which is injected via ignition.

The Planner-agent contains a web application that is exposed via tcp port 3333. Once the user accesses the web application and enters the credentials of their vCenter, the `credentials.json` file is created on the shared volume and the `collector` goroutine is spawned, which fetches the vCenter data. The data is stored in `$HOME/.migration-planner/data/inventory.json`. Once `inventory.json` is created, the `planner-agent` service sends the data over to Agent service.

### planner-agent-opa
Planner-agent-opa is a service that re-uses the [forklift validation](https://github.com/kubev2v/forklift/blob/main/validation/README.adoc) container. The forklift validation container is responsible for vCenter data validation. When the `planner-agent-collector` fetches vCenter data, it's validated against the OPA server and the report is shared back to the Agent Service.

### podman-auto-update
Podman auto update is responsible for updating the image of the containers in case there is a new image release. The default `podman-auto-update.timer` is used, which executes `podman-auto-update` every 24 hours.

## Troubleshooting Agent VM services
Useful commands to troubleshoot the Agent VM. Note that all the containers are running under the `core` user.

### Listing the running podman containers
```
$ podman ps
```

### Checking the status of planner-agent service
```
$ systemctl --user status planner-agent
```

### Inspecting the host directory with data
The ignition create a `.migration-planner` directory in `core` user home directory.
This directory should contain all relevant data, so in order to find misconfiguration please search in this directory.
```
$ ls -l .migration-planner
```

### Check logs of the services
```
$ journalctl --user -f -u planner-agent
```

### Status is `Not connected` after VM is booted.
This usually indicates that the `planner-agent` service can't communicate with the Agent service.
Check the logs of the `planner-agent` service:
```
journalctl --user -f -u planner-agent
```
And search for the error in the log:
```
level=error msg="failed connecting to migration planner: dial tcp: http://non-working-ip:7443
```
Make sure `non-working-ip` has a properly setup Agent service and is listening on port `7443`.
