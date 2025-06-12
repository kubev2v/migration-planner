# Standalone Collector Container

A fully self-contained container image that:

1. Spins up an OPA server preloaded with the forklift migration policies
2. Invokes the vCenter inventory collector
3. Writes `inventory.json` into a host-mounted directory

---

## Prerequisites

- Podman or Docker installed
- Access to a vSphere endpoint
- **Credentials**: either
    - mount a directory containing `credentials.json`
    - *or* supply `VSPHERE_USER` / `VSPHERE_PASSWORD` / `VSPHERE_URL` as env-vars  
- **Output directory**: mount a folder where `inventory.json` will be written (defaults to your home `Downloads` if unset)

---

## Run

## Quick start via Makefile

```bash
make run-standalone-collector \
    DATA_DIR=/absolute/path/to/output-folder \
    CREDENTIALS_DIR=/absolute/path/to/credentials-folder
```

This will:

Build (if needed) the planner-standalone-collector image

Run it, mounting your directories and passing any env-vars you’ve exported

Clean up the image

You can also override vSphere credentials at runtime instead of using credential.json:

```bash
make run-standalone-collector \
    DATA_DIR=/absolute/path/to/output-folder \
    VSPHERE_USER=YOUR_VSPHERE_USERNAME \
    VSPHERE_PASSWORD=YOUR_VSPHERE_PASSWORD \
    VSPHERE_URL=YOUR_VSPHERE_URL \
```

### Manually with Podman/Docker

For your convenience you can first build the image and then provide manually the custom environment vars.

```bash
# 1) Build (if you haven't already)
podman build \
-f Containerfile.standalone-collector \
-t planner-standalone-collector .

# 2) Run
podman run --rm \
-v /host/output:/host/output:Z \
-v /host/credentials-dir:/host/credentials-dir:Z \
-e VSPHERE_USER='user' \
-e VSPHERE_PASSWORD='pass' \
-e VSPHERE_URL='https://vc.example.com' \
-e TIMEOUT='10m' \
-e DATA_DIR='/host/output' \
-e CREDENTIALS_DIR='/host/creds' \
planner-standalone-collector
```
