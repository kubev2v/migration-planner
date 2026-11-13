# Appliance host updates

`cmd/appliance-updater` runs as rootless `core` on the appliance host. It loads
uploaded images, stops the agent, runs offline database migrations, selects the
service image, and records the result. Migration and rollback rules belong to
the agent CLI from [agent #603](https://github.com/kubev2v/assisted-migration-agent/pull/603).
Ignition installs the host executable and systemd units independently of the
agent container. Podman operations use the official Go bindings through the
rootless `core` user's local API socket. Ignition enables `podman.socket` and
orders the updater and agent startup gate after it. `systemctl` manages services;
`skopeo` loads archives using temporary storage on the persistent disk.

Agent #616 is deferred. This flow does not use maintenance mode, job draining,
control tokens, or `agent upgrade validate`. Stopping the service can interrupt
ongoing work. A successful migration and healthy HTTP listener do not prove that
the new agent can use every existing record or retained asset. Schedule updates
when interruption is acceptable and test each image pair with populated data.

## Image compatibility

The installed `upgrade.json` starts with an empty `transitions` allowlist. Add
only source/candidate pairs tested on an appliance, including migration failure,
rollback with populated data, and reboot recovery. Each entry uses full immutable
Podman image IDs:

```json
{"previous_image":"sha256:<64 lowercase hex>","candidate_image":"sha256:<64 lowercase hex>"}
```

The source must support startup with `AGENT_AUTO_MIGRATE=false`; the candidate
must supply offline `migrate prepare`, `apply`, `rollback`, and `verify`.
A minimum-version image label alone does not establish compatibility.

Both images' bundled DuckDB extensions must match the installed files byte for
byte, including `sqlite_scanner.duckdb_extension`. Checks run before downtime and
again before selecting an image. The image probe has no persistent data mount;
the host reads installed files without modifying them. Engine or extension
changes and destructive migrations require a separate tested recovery path.

Unsupported archives are reported as failed with `error_stage: compatibility`
and retained in `updates/rejected/archive-*/`. The current agent keeps running.
An archive matching the confirmed running image reports `unchanged`.

## Update flow

The uploader streams to `updates/.staging/`, validates the complete archive, and
atomically renames it into `updates/*.tar`. Only published archives trigger the
host watcher. The host then:

1. Inspects the healthy running container, loads the candidate into a separate
   tag, checks the image pair and extensions, and pins both images for recovery.
2. Persists `preparing`, stops `planner-agent.service`, and confirms that its
   processes have exited and no other container uses the data directory.
3. Runs the candidate image's `agent migrate prepare` to record the baseline.
   Persists `prepared` and `applying`, then runs `agent migrate apply` offline.
4. If apply fails, stops the agent, runs the candidate's rollback command, and
   verifies the baseline. Only successful recovery permits the previous image
   to start. Recovery failure leaves startup blocked.
5. If apply succeeds and extensions still match, persists `committed`, selects
   the candidate, and starts the service with automatic migrations disabled.
6. Waits for systemd's `Notify=healthy` startup and confirms the container's
   health and immutable image metadata before publishing the result.

One-shot migration containers use the candidate's immutable ID and bypass its
normal entrypoint. They mount the data directory without console credentials or
application configuration. An interrupted CLI container is removed before any
new offline operation. The host repeats the stopped-container check before each
CLI invocation. Other data consumers are rejected, never killed automatically.

If preparation fails before apply, the previous image is restarted without down
SQL. After `committed` is persisted, startup or reporting failures retain that
candidate and retry it. They never automatically roll back migrations: normal
startup may already have allowed writes. In particular, this flow does not
provide automatic rollback when migrations succeed but application startup or
later data access fails.

## Boot and service wiring

The ISO includes a static Linux/amd64 executable at `/host/appliance-updater`.
Bootstrap installs it at `/usr/local/sbin/appliance-updater`. Build it locally
with `make build-appliance-updater`, producing
`bin/appliance-updater-linux-amd64`. Direct `build-ove-image.sh` invocations can
set `HOST_UPDATER=/absolute/path/to/appliance-updater`.

The rootless user units are:

- `planner-agent-image-load.service`: loads the ISO image only when the persistent
  service tag is absent, preserving an upgraded image across reboot.
- `planner-agent-image-recovery.service`: reconciles the active host journal on
  boot before the archive watcher starts.
- `planner-agent-image-update.path`: watches published `updates/*.tar` files.
- `planner-agent-image-update.service`: runs `appliance-updater update`.
- `planner-agent.service`: runs `appliance-updater gate` before every start,
  including manual starts and restarts. The Quadlet uses `Pull=never`,
  `Notify=healthy`, and an HTTP health check. Podman auto-update is disabled.

The agent service does not wait for the recovery service, because recovery may
itself need to start the agent. The startup gate checks the durable journal and
selected image instead. Only `committed`, `rolled-back`, and `aborted` permit
startup. Unfinished offline operations and `recovery-failed` block startup.

The gate writes `/home/core/.migration-planner/upgrade.env`. After a managed
attempt it sets `AGENT_AUTO_MIGRATE=false`, preventing normal startup from
applying additional migrations. Fresh appliances use the agent's default
migration behavior to initialize their databases. No lifecycle endpoint or
maintenance environment variables are required.

## Recovery records and status

Paths below are relative to `/var/lib/data/agent/updates`:

- `host/active.json`: active host-attempt pointer.
- `host/attempts/<id>.json`: host journal with pinned image identities, phase,
  commit time, and reporting state.
- `attempts/<id>/manifest.json`: migration baseline owned by the agent CLI.
- `status.json`: last result consumed by the agent's status endpoint and UI.

Writes use temporary files, fsync, atomic rename, and directory fsync. Malformed
journals, missing referenced records, and mismatched image selection block
startup. Journals, migration manifests, and pinned images are retained.

| Interrupted phase | Recovery |
| --- | --- |
| `preparing` | Abort without down SQL; resume the previous image. |
| `prepared`, `applying`, `rolling-back` | Stop the service, run/retry candidate rollback, and verify the baseline. |
| `restoring` | Recovery was verified; check extensions and restore the previous image. |
| `committed` | Start/retry the candidate; never reopen migration rollback. |
| `rolled-back`, `aborted` | Start/retry the previous image. |
| `recovery-failed` | Keep startup blocked until explicit operator retry. |

If container inspection fails, the host confirms exit before restarting. It
leaves a possibly running container alone and suspends the watcher if exit
cannot be established. A reporting retry does not restart a healthy agent.

Status outcomes are `applied`, `rolled-back`, `failed`, and `unchanged`. Records
include the archive name, timestamp, error and stage, running image, previous
successful update, attempt ID, and optional migration rollback details. Last
success advances only after confirmed candidate startup, using the original
commit time. A startup failure reports `failed` while retaining the committed
candidate; a successful retry reports `applied`.

## Operator retry and existing appliances

Run commands as `core`. Inspect the user journal, disk space, active attempt,
and pinned images before retrying. For interrupted work or a committed startup
or reporting failure:

```sh
systemctl --user stop planner-agent-image-update.path
/usr/local/sbin/appliance-updater recover
```

After correcting a `recovery-failed` attempt, name that exact attempt:

```sh
/usr/local/sbin/appliance-updater -attempt-id <id> retry-recovery
```

This rejects committed attempts. Never edit a journal phase or retag an image
to bypass recovery. After successful recovery:

```sh
systemctl --user reset-failed planner-agent-image-recovery.service planner-agent-image-update.service planner-agent-image-update.path
systemctl --user restart planner-agent-image-recovery.service
systemctl --user start planner-agent-image-update.path
```

Ignition affects new appliances. Existing appliances need a separate installation
of the host binary, rendered units, gated Quadlet, and configuration. Preserve
mount ownership, credentials, pinned images, and unresolved recovery records.
Stop the service before replacing host units and disable the legacy auto-update
timer. An agent image update alone cannot install these host assets. Leave the
transition allowlist empty until the exact image pair passes appliance testing.

## Validation

Run `go test -race ./internal/applianceupdater ./internal/image ./cmd/appliance-updater`
for migration ordering, failed-stop protection, rollback verification, reboot
recovery, committed startup/reporting retries, container health checks, and
rendered Ignition wiring. `make build-appliance-updater` checks the static host
binary. The Containerfile's `host-builder` target checks the Linux build stage
without building the entire ISO.

Real appliance testing must cover successful upgrades, interrupted active jobs,
prepare/apply failures, rollback and verification failures, reboot in each phase,
candidate startup failure, and retained data/credentials/assets. Unit tests with
substituted host commands do not establish release compatibility.

The updater builds with `remote,containers_image_openpgp` tags so the SDK uses
the remote client and pure Go signature implementation without native GPGME
dependencies. The Makefile and ISO container build set these tags. The Makefile
also excludes the unused local Btrfs driver with `exclude_graphdriver_btrfs`,
avoiding a native header dependency in Linux builds with CGO enabled. Use all
three tags when running updater tests directly:

```sh
go test -tags remote,containers_image_openpgp,exclude_graphdriver_btrfs ./internal/applianceupdater
```
