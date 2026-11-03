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
- `AGENT_IMAGE` - Agent image repository (without a tag)
- `AGENT_IMAGE_TAG_MAIN` - Dev appliance tag (default: `main`)
- `AGENT_IMAGE_TAG_STABLE` - Stage appliance tag (default: `stable`)

Push builds select the configured tag using `AGENT_IMAGE_BRANCH` (`main` or
`stable`). Local `make migration-planner-iso-container` builds use the dev tag.
An explicit `AGENT_IMAGE_TAG` build argument overrides this selection; production
tag builds continue to use `AGENT_IMAGE_TAG=prod`.

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
