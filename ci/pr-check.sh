#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARENT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PARENT_DIR"

export MIGRATION_PLANNER_API_IMAGE="${MIGRATION_PLANNER_API_IMAGE:-quay.io/app-sre/migration-planner-api}"
export MIGRATION_PLANNER_AGENT_IMAGE="${MIGRATION_PLANNER_AGENT_IMAGE:-quay.io/app-sre/migration-planner-agent}"
export REGISTRY_TAG="${REGISTRY_TAG:-pr-test}"

make build-containers
