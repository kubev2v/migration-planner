# Appliance host updates

The host configuration in `data/ignition.template` applies to newly generated
appliances. Existing appliances need a separate host configuration rollout;
updating the agent container does not update its host scripts or systemd units.

## Agent/host contract

The agent validates an OCI archive before moving it from
`/var/lib/data/agent/updates/.staging/` into `updates/<version>.tar`. The host
watches only `updates/*.tar`, loads the archive into the configured local image
tag, and immediately invokes Podman auto-update. The host requires `jq`,
`skopeo`, and Podman with Quadlet `Notify=healthy` support.
The filename is chosen by the agent; `<version>.tar` is a convention, not a
host-enforced name. The publisher must avoid replacing an archive still being
processed under the same name.

The shared file `/var/lib/data/agent/updates/status.json` is visible inside the
agent as `/var/lib/agent/updates/status.json`:

```json
{
  "file": "v2.tar",
  "result": "applied",
  "time": "2026-11-02T00:00:00Z",
  "error": "",
  "error_stage": "",
  "running_image": {"digest": "sha256:...", "version": "v2"},
  "last_successful_update": {
    "digest": "sha256:...",
    "version": "v2",
    "time": "2026-11-02T00:00:00Z"
  }
}
```

`result` is `applied`, `rolled-back`, `failed`, or `unchanged`. `file`, `result`,
`time`, and `error` describe the latest attempt. `running_image` comes from
inspecting the running container's immutable image ID, and is `null` if its
digest and version cannot be determined. Version comes from the image label
`org.opencontainers.image.version`.

`applied` means Podman completed the restart, the configured `/api/v2/version`
health check passed, and the running image metadata was recorded. This confirms
API startup, not the readability of every collection. The health check does not
scan for archives; the separate systemd path unit watches for them.

For failures, `error_stage` identifies `load`, `restart`, `report`, or `metadata`.
It is empty for non-failed outcomes. `error` includes a stage-specific message
and, when available, up to the last 4096 bytes of command stderr; those diagnostics
also go to the user journal. The explanatory prefix is additional to that limit.
Temporary diagnostic files are removed when the script exits.

`last_successful_update` changes only after a confirmed application with image
metadata available. Failures, rollbacks, and unchanged images preserve it;
it is `null` before the first confirmed update. Its timestamp is the completion
time, not the download or image-load time. A rollback's `running_image` can
therefore identify the restored image while `last_successful_update` remains
unchanged.

Status is written to a temporary file on the persistent data filesystem and
renamed only after encoding succeeds. Publication failures leave the previous
status and archive intact, fail the systemd service, and skip image pruning.
For example, a restart failure can record:

```json
{"result":"failed","error_stage":"restart","error":"podman auto-update failed: startup timed out"}
```

## Persistence and rollback boundary

The persistent mount retains databases, the credential encryption key, uploaded
VDDK, and update status across container replacement. Image rollback reuses
these files as they are; the host does not back them up, undo migrations, or
restore their pre-update contents. `last_successful_update` records the last
confirmed image application, not a database backup or a guarantee of data
compatibility with that image. Supported upgrade/downgrade paths need separate
database compatibility and recovery validation.

Normal startup policy and update validation are separate concerns. An already
corrupt collection should not prevent access to otherwise healthy collections.
This host change neither requires agent PR #565's proposed fail-fast policy nor
adds a requirement that every collection be healthy. With tolerant agent
startup, a collection failure can coexist with `applied`. Detecting a previously
working collection becoming unusable during an update requires agent-side
validation; `/version` alone cannot distinguish it from a pre-existing failure.

The host also does not wait for collection, inspection, forecasting, or uploads
to finish before restarting the container. Run these checks with the appliance
idle. Interrupting active work may discard unfinished results. Coordinating
updates with active jobs and implementing database backup/restore are separate
work spanning the agent and host. After a successful application, dangling image
pruning can remove the previous image, so later manual rollback may require
loading its archive again.

## Retrying after a status-publication failure

Run the following as `core` after a failed status write. First stop the watcher
so a retained archive does not cause repeated retries while resolving the fault:

```sh
systemctl --user stop planner-agent-image-update.path
journalctl --user -u planner-agent-image-update.service -n 100 --no-pager
df -h /var/lib/data
ls -ld /var/lib/data/agent/updates
```

Resolve the underlying problem, such as insufficient disk space or incorrect
directory ownership/permissions. Then reset any service/path rate-limit failure,
retry the retained archive once, and re-enable watching:

```sh
systemctl --user reset-failed planner-agent-image-update.service planner-agent-image-update.path
systemctl --user start planner-agent-image-update.service
systemctl --user start planner-agent-image-update.path
jq . /var/lib/data/agent/updates/status.json
```

Check the recorded result even if `systemctl start` succeeds: the script can
successfully record a failed update. If the new image already started before
publication failed, the retry can report `unchanged`. It records the actual
running image but preserves the previous `last_successful_update` (or `null`);
it does not invent a success timestamp for the unrecorded attempt. The archive
is removed once the retry publishes its outcome.

## Automated checks

Run `go test ./internal/image/`. The tests decode the real Butane output and
execute its scripts with isolated Podman/skopeo substitutes. They cover load
ordering, successful application, rollback, unchanged images, load and restart
failures, report selection, missing metadata, bounded failure diagnostics, status
publication failures and retries, JSON escaping, and preservation of the
successful record across later attempts.
The boot-loader test verifies that an existing persistent image skips the ISO
copy. These tests do not execute a real systemd restart or reboot.

## Appliance integration checks

Use a disposable appliance generated with the updated ignition, and run host
commands as `core`, whose rootless Podman storage and user units own the agent.
Keep access through SSH or the VM console while testing restart failures.
Record the old and new agent image versions/digests and whether their startup
policy tolerates individual collection failures. These checks do not establish
compatibility for other image pairs.

1. Confirm `jq --version`, `skopeo --version`, and `podman --version` work.
   Confirm `systemctl --user status planner-agent-image-update.path` is active,
   `planner-agent.service` is healthy, and the auto-update timer is disabled.
   Keep the appliance idle: no collection, inspection, forecasting, or uploads.
   Populate saved inventories, groups, VM labels/exclusions, inspection and
   rightsizing results, forecast reports, credentials, and VDDK. Export/read the
   logical values through the UI/API as a baseline. Record checksums of
   `credentials.key` and installed VDDK files without exposing their contents;
   verify that saved credentials can connect to vCenter.
2. Obtain a validated newer OCI archive with a version label and an image that
   answers `https://localhost:3333/api/v2/version`. Copy it to `updates/.staging/`;
   verify that neither the container nor `status.json` changes. Rename it to
   `updates/<version>.tar` to publish it.
3. Verify a prompt restart, `result == "applied"`, archive removal, and matching
   digest/version between `status.json` and `podman image inspect` of the image
   ID reported by `podman container inspect planner-agent`. Confirm
   `last_successful_update.time` advanced only after healthy startup.
   Read the saved information again, compare logical values with the baseline,
   verify credentials still decrypt and connect, and verify the key and VDDK
   checksums. Compare logical database contents, not raw database-file hashes,
   because legitimate writes/checkpointing can change those files.
4. Save the container image ID and `status.json`. Reboot the appliance, reconnect,
   and verify the same image is running, the status file is unchanged, and the
   boot loader did not restore the ISO image. Check
   `journalctl --user -u planner-agent-image-load.service -b`.
   Repeat the saved-data, credential, and VDDK checks after reboot.
5. Publish an archive for a test image that exits or never becomes healthy.
   Verify rollback restores the previous image, the result is `rolled-back`,
   and the last successful update is unchanged. Allow for the 300-second
   startup timeout and the subsequent rollback startup.
   Use a test image that fails without modifying persistent state to isolate
   image-rollback behavior. Verify the saved-data baseline after rollback and
   after a further reboot. This test does not prove recovery from incompatible
   schema changes; database restore is not implemented by the host.
6. Publish an invalid archive. Verify `failed`, an error reason, no container
   restart, archive removal, and an unchanged last successful update. Reboot
   again and verify the successful image and record are still retained.
7. Publish the already-running image. Verify `unchanged` and no new successful
   update timestamp.
8. On a disposable copy, introduce one corrupt collection before the update and
   record which collections are usable. Verify the chosen agent startup policy:
   with tolerant startup, other collections remain accessible and the affected
   collection should be identifiable through the API/UI. This host change does
   not implement that per-collection error reporting. If the chosen image fails
   startup instead, record that as an agent-policy issue, not proof that the
   update introduced the corruption. Separately test an image that makes a
   previously usable collection inaccessible; compare against the baseline even
   if `/version` succeeds and the host reports `applied`. Record this as an
   update regression requiring agent-side validation/recovery.

For an existing appliance, first apply the rendered host scripts, update path
and service units, and agent Quadlet from the new ignition, preserving that
appliance's configured image reference and environment. Create `.staging/` with
`core` ownership, verify host prerequisites, disable the old user auto-update
timer, reload the user systemd manager, and restart the watcher and agent during
a maintenance window. Then run the integration checks above. Replacing the
container image alone does not perform this host rollout.
