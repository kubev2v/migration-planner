# Collector
To run the collector localy here are the steps.

## Prepare
Prepare the dependencies.

### Credentials
Create VMware credentials file.

```
cat <<EOF > /tmp/creds.json
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
go run cmd/collector/main.go /tmp/creds.json /tmp/inventory.json
```

Explore `/tmp/inventory.json`
