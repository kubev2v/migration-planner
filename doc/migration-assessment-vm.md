# Migration Assessment virtual machine
The Migration Assessment virtual machine(VM), based on Red Hat CoreOS (RHCOS), communicates with the Assisted Migration Service and reports its status.
The Migration Assessment VM is initialized using ignition, which configures multiple containers that run as systemd services. Each of these services is dedicated to a specific function.

## Systemd services
The following are a list of systemd services that can be found on Migration Assessment virtual machines. All of the services
are defined as quadlets. Quadlet configuration can be found in the [ignition template file](../data/ignition.template).
The `planner-agent` containerfile can be found [here](../Containerfile.agent).

### planner-agent
Planner-agent is a service that reports the status to the Assisted Migration service. The URL of the Assisted Migration service is configured in the file `$HOME/.migration-planner/config/config.yaml`, which is injected via ignition.

The `planner-agent` contains a web application that is exposed via tcp port 3333. Once the user accesses the web application and enters the credentials of their vCenter, the `credentials.json` file is created on the shared volume and the `collector` goroutine is spawned, which fetches the vCenter data. The data is stored in `$HOME/.migration-planner/data/inventory.json`. Once `inventory.json` is created, the `planner-agent` service sends the data over to Assisted Migration service.

### planner-agent-opa
Planner-agent-opa is a service that re-uses the [forklift validation](https://github.com/kubev2v/forklift/blob/main/validation/README.adoc) container. The forklift validation container is responsible for vCenter data validation. When the `planner-agent-collector` fetches vCenter data, it's validated against the OPA server and the report is shared back to the Assisted Migration Service.

### podman-auto-update
Podman auto update is responsible for updating the image of the containers in case there is a new image release. The default `podman-auto-update.timer` is used, which executes `podman-auto-update` every 24 hours.

## Troubleshooting Migration Assessment VM services
Useful commands to troubleshoot the Migration Assessment VM. Note that all the containers are running under the `core` user.

### Listing the running podman containers
```
$ podman ps
```

### Checking the status of planner-agent service
```
$ systemctl --user status planner-agent
```

### Inspecting the host directory with data
When the virtual machine boots, ignition creates the `.migration-planner` directory in the `core` user's home directory.
This directory contains two subdirectories: `data` and `config`.
The `data` directory contains a `credentials.json` file, which is created when the user enters their vCenter credentials.
It also contains an `inventory.json` file, which is created by the `planner-agent` service when vCenter data is fetched.
The `config` directory contains a `config.yaml` file, which is created by ignition and contains configuration for the
`planner-agent` service.

To explore the data and configuration created and used by `planner-agent`:
```
$ ls -l /home/core/.migration-planner/data
$ cat /home/core/.migration-planner/config/config.yaml
```

### Check logs of the services
```
$ journalctl --user -f -u planner-agent
```

### Status is `Not connected` after VM is booted.
This usually indicates that the `planner-agent` service can't communicate with the Assisted Migration service.
Check the logs of the `planner-agent` service:
```
journalctl --user -f -u planner-agent
```
And search for the error in the log:
```
level=error msg="failed connecting to migration planner: dial tcp: http://non-working-ip:7443
```
Make sure `non-working-ip` has a properly setup Assisted Migration service and is listening on port `7443`.
