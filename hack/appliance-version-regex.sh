#!/bin/bash

# Single source for the appliance version format validation used by the
# GitHub workflows. Keep in sync with the Go regexes in pkg/version/compare.go.
export APPLIANCE_VERSION_RE='^(v[0-9]+\.[0-9]+\.[0-9]+|main-release-[0-9]+\.[0-9]+-[0-9a-f]{7}|stable-release-[0-9]+\.[0-9]+-[0-9a-f]{7}|on-pr-[0-9a-f]{7})$'
