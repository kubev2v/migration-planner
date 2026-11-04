# OpenShift Migration Advisor ISO Builder

Builds a bootable RHCOS ISO with embedded OpenShift Migration Advisor agent image.

## Files

- `Containerfile` - Multi-stage container build definition
- `build-ove-image.sh` - Shell script that creates the OVE ISO
- `config` - Build configuration (ISO URL, checksum, agent image)

## Configuration

The `config` file defines build arguments that are provided to Konflux for building the image:

- `FINAL_ISO_PATH` - Path where ISO will be placed in final image (default: `/rhcos.iso`)
- `ISO_URL` - RHCOS ISO download URL
- `ISO_CHECKSUM` - SHA256 checksum for ISO verification
- `AGENT_IMAGE` - Agent image repository (without a tag, used by the pipeline's resolve task)
- `AGENT_IMAGE_TAG_MAIN` - Dev appliance tag (used by the pipeline to resolve the full pullspec for `main` branch builds)
- `AGENT_IMAGE_TAG_STABLE` - Stage appliance tag (used by the pipeline to resolve the full pullspec for `stable` branch builds)

### Containerfile ARGs

- `AGENT_IMAGE` - Full agent image pullspec including tag (e.g. `quay.io/.../oma-appliance:main-release-0.32-d689d7b`). The pipeline's `resolve-appliance-version` task constructs this from the config file values and passes it as a build arg. For local builds the Makefile constructs it from the config file.
- `APPLIANCE_VERSION` - Version extracted from the appliance image's `org.opencontainers.image.version` OCI label. Set by the pipeline's `resolve-appliance-version` task via `skopeo inspect`; defaults to `local-dev` for local Makefile builds. Applied as the `io.kubev2v.oma.appliance-version` LABEL on the final image.

### Pipeline image resolution

The `resolve-appliance-version` inline Tekton task runs before the container
build. It reads the config file, resolves the agent image tag from the branch
context (push: `main` or `stable` from `{{target_branch}}`; PR: `main`; tag:
`prod`), constructs the full `AGENT_IMAGE` pullspec, and uses `skopeo inspect`
to extract the `org.opencontainers.image.version` label as `APPLIANCE_VERSION`.

Local `make migration-planner-iso-container` builds construct the full pullspec
from `AGENT_IMAGE` and `AGENT_IMAGE_TAG_MAIN` in the config file.

Pushes to `main` and `stable` in `kubev2v/assisted-migration-agent` wait up to two
hours for the released `oma-appliance` image in Quay, then dispatch an
`appliance_image_updated` event to this repository, including the exact image tag
it waited for in `client_payload.image_tag`. The receiver validates the tag's
format, branch, and commit suffix and opens or updates a PR against the same
branch, changing only its corresponding config value to that supplied tag
(`<branch>-release-<major>.<minor>-<7-character-sha>`). Stale notifications are
rejected if the agent branch has moved on.

The existing PR approval workflow allows `migration-planner-sync[bot]` to update
this config file, approves the PR, and enables auto-merge with the branch's
required checks. Both repositories need the `MIGRATION_PLANNER_SYNC_APP_ID` and
`MIGRATION_PLANNER_SYNC_APP_PRIVATE_KEY` secrets, with the sync app installed on
both repositories. Planner also needs its existing reviewer app secrets and
repository auto-merge enabled. First merge the optional `image-tag` input support
in the shared `migration-planner-workflows` dispatch workflows. The new callers
use `@main` to consume those inputs; existing callers can keep their pinned
versions and omit the optional input. Install the receiver workflow on the default
branch and the ISO build changes on both `main` and `stable` before enabling the
source notification workflow on those branches.

## How It Works

1. **Builder stage**: Downloads RHCOS ISO, verifies checksum, extracts contents, embeds agent image, creates bootable ISO
2. **Final stage**: Minimal image containing only the final ISO at `${FINAL_ISO_PATH}`
