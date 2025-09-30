# Migration Planner ISO Builder

Builds a bootable RHCOS ISO with embedded migration planner agent image.

## Files

- `Containerfile` - Multi-stage container build definition
- `build-ove-image.sh` - Shell script that creates the OVE ISO
- `config` - Build configuration (ISO URL, checksum, agent image)

## Configuration

The `config` file defines build arguments that are provided to Konflux for building the image:

- `FINAL_ISO_PATH` - Path where ISO will be placed in final image (default: `/rhcos.iso`)
- `ISO_URL` - RHCOS ISO download URL
- `ISO_CHECKSUM` - SHA256 checksum for ISO verification
- `AGENT_IMAGE` - Full agent image path with tag (e.g., `registry/org/image:latest`)

## How It Works

1. **Builder stage**: Downloads RHCOS ISO, verifies checksum, extracts contents, embeds agent image, creates bootable ISO
2. **Final stage**: Minimal image containing only the final ISO at `${FINAL_ISO_PATH}`
