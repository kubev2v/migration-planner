# Standalone Collector Container

A fully self-contained container image that:

1. **Spins up** an OPA server preloaded with the forklift migration policies
2. **Exposes** the agent UI at `http://localhost:3333/login`, allowing you to enter credentials and download the assessment `inventory.json`
3. **Optionally accepts** credentials as command-line arguments

---

## Prerequisites

- Podman or Docker installed
- Authenticated access to a vSphere endpoint

---

## Quick start via Makefile

```bash
make run-collector
```

This will:
- Build (if needed) the `planner-standalone-collector` image.
- Run a container, passing `VSPHERE_USER`, `VSPHERE_PASSWORD`, and `VSPHERE_URL` as environment variables 
if they are set (you can also use the UI instead in order to provide credentials).
- Clean up the container on exit.
