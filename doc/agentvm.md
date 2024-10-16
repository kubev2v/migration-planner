# Agent virtual machine
The agent, based on Red Hat CoreOS (RHCOS), communicates with the Agent Service and reports its status.
The agent virtual machine is initialized using ignition, which configures container that run as systemd service.

## Systemd services
Follows the list of systemd services that can be found on agent virtual machine. All of the services
are defined as quadlets. Quadlet configuration can be found in the [ignition template file](../data/ignition.template).
Agent dockerfile can be found [here](../Containerfile.agent).

### planner-agent
Planner-agent is a service that reports the status to the Agent service. The URL of the Agent service is configured in `$HOME/.migration-planner/config.yaml` file, which is injected via ignition.

Planner-agent contains web application that is exposed via port 3333. Once user access the web app and enter the credentials of the vCenter, `credentials.json` file is created, and goroutine is executed which fetch the data from the vCenter. The data are stored in `invetory.json` file. Once agent notice the file it will send them over to Agent service.

### planner-agent-opa
Planner-agent-opa is a service that re-uses [forklift validation](https://github.com/kubev2v/forklift/blob/main/validation/README.adoc) container. The forklift validation container is responsible for vCenter data validation. When `planner-agent` fetch vCenter data it's validated against the OPA server and report is shared back to Agent Service.

### podman-auto-update
Podman auto update is responsible for updating the image of containers in case there is a new release of the image. We use default `podman-auto-update.timer`, which executes `podman-auto-update` every 24hours.

## Troubleshooting Agent VM services
Usefull commands to troubleshoot Agent VM. Note that all the containers are running under `core` user.

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
