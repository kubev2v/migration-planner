#!/bin/bash

# Single source for the appliance version format validation used by the
# GitHub workflows. Keep in sync with the Go regexes in pkg/version/compare.go.
export APPLIANCE_VERSION_RE='^(v[0-9]+\.[0-9]+\.[0-9]+|main-release-[0-9]+\.[0-9]+-[0-9a-f]{7}|stable-release-[0-9]+\.[0-9]+-[0-9a-f]{7}|on-pr-[0-9a-f]{7})$'

# Map a served appliance version to its per-environment minimum key in
# build/migration-planner-iso/config. Empty when the channel is unknown.
min_appliance_version_key() {
    case "$1" in
        v[0-9]*)          echo MIN_APPLIANCE_VERSION_PRODUCTION ;;
        main-release-*)   echo MIN_APPLIANCE_VERSION_DEV ;;
        stable-release-*) echo MIN_APPLIANCE_VERSION_STAGE ;;
        *)                echo "" ;;
    esac
}
