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
    # Try to construct tag from release config file if TARGET_BRANCH is set
    if [ -n "$TARGET_BRANCH" ] && [ -f ".github/release-config.txt" ]; then
        RELEASE_BRANCH=$(cat .github/release-config.txt | tr -d '\n\r ')
        export SOURCE_GIT_TAG="${TARGET_BRANCH}-${RELEASE_BRANCH}-${SOURCE_GIT_COMMIT_SHORT}"
    else
        # Fallback to git describe
        export SOURCE_GIT_TAG=$(git describe --always --tags --abbrev=7 \
            --match '[0-9]*.[0-9]*.[0-9]*' \
            --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null \
            || echo "v0.0.0-${SOURCE_GIT_COMMIT_SHORT}")
    fi
fi

# The appliance version is the tag of the appliance image the ISO embeds, taken
# from the same config file the ISO build uses, so both report the same release.
# It is never derived from this repo's git: the appliance is built elsewhere.
if [ -z "${APPLIANCE_VERSION:-}" ] && [ -f "build/migration-planner-iso/config" ]; then
    APPLIANCE_VERSION=$(sed -n 's/^APPLIANCE_VERSION=//p' build/migration-planner-iso/config)
fi
export APPLIANCE_VERSION

# Release tags (v0.31.0) carry no commit; branch builds end in the agent's short SHA.
export APPLIANCE_COMMIT=$(echo "$APPLIANCE_VERSION" | sed -n 's/.*-\([0-9a-f]\{7\}\)$/\1/p')

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
-X github.com/kubev2v/migration-planner/pkg/version.buildDate=${BIN_TIMESTAMP} \
-X github.com/kubev2v/migration-planner/pkg/version.applianceCommit=${APPLIANCE_COMMIT} \
-X github.com/kubev2v/migration-planner/pkg/version.applianceVersion=${APPLIANCE_VERSION}"
