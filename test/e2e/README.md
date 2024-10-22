# Running integration tests
The integration tests are executed against deployed `planner-api`. The planner api can be deployed
as container or running as binary.

## Requiremets

```
dnf install -y libvirt-devel
sudo usermod -a -G libvirt $USER
```

Running planner api, either as container or binary:
```
bin/planner-api
```

## Executing tests
```
PLANNER_IP=1.2.3.4 make integration-tests
```
