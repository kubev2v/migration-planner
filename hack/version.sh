#!/bin/bash

# This script calculates version information from git and exports them as environment variables
# Usage:
#   source hack/version.sh    - sources all version variables into current shell

set -e

# Get git commit
# If SOURCE_GIT_COMMIT is already set (from build args), use it
if [ -z "$SOURCE_GIT_COMMIT" ]; then
    export SOURCE_GIT_COMMIT=$(git rev-parse "HEAD^{commit}" 2>/dev/null || echo "")
fi

# Calculate short commit from full commit or from git
if [ -n "$SOURCE_GIT_COMMIT" ]; then
    export SOURCE_GIT_COMMIT_SHORT="${SOURCE_GIT_COMMIT:0:7}"
else
    export SOURCE_GIT_COMMIT_SHORT=$(git rev-parse --short "HEAD^{commit}" 2>/dev/null || echo "")
fi

# Get git tag
# If SOURCE_GIT_TAG is already set (from build args), use it
if [ -z "$SOURCE_GIT_TAG" ]; then
    export SOURCE_GIT_TAG=$(git describe --always --tags --abbrev=7 \
        --match '[0-9]*.[0-9]*.[0-9]*' \
        --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null \
        || echo "v0.0.0-${SOURCE_GIT_COMMIT_SHORT}")
fi

# Get git tree state
if [ ! -d ".git/" ] || git diff --quiet 2>/dev/null; then
    export SOURCE_GIT_TREE_STATE="clean"
else
    export SOURCE_GIT_TREE_STATE="dirty"
fi

# Get build timestamp
export BIN_TIMESTAMP=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# Parse version numbers from tag
export MAJOR=$(echo $SOURCE_GIT_TAG | sed 's/^v//' | awk -F'[._~-]' '{print $1}')
export MINOR=$(echo $SOURCE_GIT_TAG | sed 's/^v//' | awk -F'[._~-]' '{print $2}')
export PATCH=$(echo $SOURCE_GIT_TAG | sed 's/^v//' | awk -F'[._~-]' '{print $3}')

# Build ldflags
export GO_LDFLAGS="\
-X github.com/kubev2v/migration-planner/pkg/version.majorFromGit=${MAJOR} \
-X github.com/kubev2v/migration-planner/pkg/version.minorFromGit=${MINOR} \
-X github.com/kubev2v/migration-planner/pkg/version.patchFromGit=${PATCH} \
-X github.com/kubev2v/migration-planner/pkg/version.versionFromGit=${SOURCE_GIT_TAG} \
-X github.com/kubev2v/migration-planner/pkg/version.commitFromGit=${SOURCE_GIT_COMMIT} \
-X github.com/kubev2v/migration-planner/pkg/version.gitTreeState=${SOURCE_GIT_TREE_STATE} \
-X github.com/kubev2v/migration-planner/pkg/version.buildDate=${BIN_TIMESTAMP}"
