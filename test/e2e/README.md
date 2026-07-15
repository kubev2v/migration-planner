# Running E2E tests

The E2E tests spin up a Kind cluster, deploy the planner stack, and run the test suite.

## Requirements

- Docker
- Kind
- A private key for agent authentication at `/etc/planner/e2e` (or pass `--private-key-path`)

## Executing tests

```
go run ./test/e2e/
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--api-image` | `quay.io/.../migration-planner-api:latest` | API image ref |
| `--iso-image` | `quay.io/.../migration-planner-rhcos-iso:latest` | ISO image ref |
| `--cluster-name` | `kind-e2e` | Kind cluster name |
| `--private-key-path` | `/etc/planner/e2e` | Path to private key directory |
| `--keep-env` | `false` | Keep the Kind cluster after tests complete |

The host IP is auto-detected. Images are pulled automatically if not present locally.
