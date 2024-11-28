# Collector
To run the collector localy here are the steps.

## Prepare
Prepare the dependencies.

### Configuration 
Create the planner-agent configuration file:

```
$ mkdir /tmp/config
$ mkdir /tmp/data
$ cat <<EOF > ~/.planner-agent/config.yaml
config-dir: /tmp/config
data-dir: /tmp/data
log-level: debug
source-id: 9195e61d-e56d-407d-8b29-ff2fb7986928
update-interval: 5s
planner-service:
  service:
    server: http://127.0.0.1:7443
EOF
```

### Credentials
Create VMware credentials file.

```
cat <<EOF > /tmp/data/credentials.json
{
  "username": "user@example.com",
  "password": "userpassword",
  "url": "https://vmware.example.com/sdk"
}
EOF
```

### OPA
Run the OPA server for VM validations.

```
podman run -p 8181:8181 -d --name opa --entrypoint '/usr/bin/opa' quay.io/kubev2v/forklift-validation:release-v2.6.4 run --server /usr/share/opa/policies
```

## Run
Build & run the collector code specifying credentials file as first argument and as second path to invetory file, where data should be written.

```
go run cmd/planner-agent/main.go -config -config ~/.planner-agent/config.yaml
```

Explore `/tmp/data/inventory.json`
