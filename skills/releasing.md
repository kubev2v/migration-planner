# Skill: Openshift Migration Advisor Releasing

## Overview

Handles the process of releasing the Migration Planner service to production and stable environments, including tagging, 
building images, and updating downstream components.

## Important Note

**When uncertain about any step, environment setup, or user intent - ASK the user rather than making assumptions.**
This skill involves production releases, so clarity is critical.

## Use Cases

- Creating a new production release

## Requirements
- Active GitHub clone and a remote for the repos: **Kubev2v/migration-planner**, **Kubev2v/migration-planner-ui-app**
- Active GitLab clone and a remote for the repo: **app-interface**
- Authenticated Git CLI (SSH) for the repos above 
- GitHub CLI (gh) to be installed and authenticated
- `planner` CLI installed (in Kubev2v/migration-planner clone at: bin/planner)
- User must have push permissions

## Inputs

| Name      | Type   | Required   | Description                    |
|-----------|--------|------------|--------------------------------|
| version   | string | ✅          | Release version (e.g. v0.13.2) |

## Environment Validation
Before execution:

You need to know or infer the following:

1. OS paths for all the above-mentioned repos in the requirements. The default case is:
**Kubev2v/migration-planner** clone is in your current directory.
**Kubev2v/migration-planner-ui-app** located relatively to your path: ../migration-planner-ui-app
Same as **app-interface** which should be ../app-interface.

If that is not the case (and you don't find it without scanning the filesystem) you can ask the user if there is
an active clone of those repos and ask the user either to clone them, or give you the path. 

2. Once step 1 is completed you have all the paths for the cloned repos. Identify the correct remote for each repository that points to the upstream repos (not to forks/personal clones).

## Outputs

| Name      | Type   | Description                                           |
|-----------|--------|-------------------------------------------------------|
| success   | bool   | Whether release initiated and MR created successfully |
| tag       | string | Created Git tag                                       |
| error     | string | Error message if failed                               |
| gitlab MR | string | Open MR in app-interface                              |

## Flow / Logic

1. Validate version format (must follow semver, e.g. v0.13.2)
2. Check working directory status for **migration-planner** and **migration-planner-ui-app** repos:
   - Ignore git submodules (they may show as modified)
   - If there are uncommitted changes (tracked files), inform the user and ask them to either commit, stash, or confirm to proceed
   - Untracked files can be ignored
3. Ensure the content of migration-planner/.github/release-config.txt reflects the version provided:
   - Format should be: `release-X.Y` (branch name style, e.g., `release-0.13` for version `v0.13.3`)
   - Extract the minor version from the provided tag (e.g., `v0.13.3` → `release-0.13`)
   - If the file doesn't match, create a PR to update it and ask the user to review and merge it before continuing with the release process.
4. Create and push Git tags for both repositories using the `bin/planner release` command:
   - The command requires confirmation input, so pipe `echo "Y"` to auto-confirm
   - For migration-planner: `echo "Y" | bin/planner release --tag <version> --branch main --repo-path <planner-path> --remote <remote-name>`
   - For migration-planner-ui-app: `echo "Y" | bin/planner release --tag <version> --branch master --repo-path <ui-app-path> --remote <remote-name>`
   - The command creates the tag locally and pushes it to the remote repository
   - If `--sha` is not specified, the command uses the latest commit on the specified branch
   - **Expected output on success:**
     ```
     ✓ Created tag v0.13.3 at commit <sha>
     ✓ Pushed tag v0.13.3 to upstream
     ✓ Created GitHub release: Release v0.13.3
     
     HEAD SHA: <sha>
     ```
5. Prepare an MR for app-interface to update the stable environments with the new tag SHAs:
   - In the app-interface repository, locate the files named `saas-service.yaml` and `saas-ui.yaml` that are related to assisted-migration
   - Review the git log and last merged MRs for these files to understand the pattern used in previous releases
   - **Important**: The number of environments and their configuration may vary between releases, so always check the latest MRs to identify all environments that need updating
   - Update the SHA values for stable environments to match the HEAD sha of the newly created tags
   - Create a new branch, commit the changes with format: `NO-JIRA | release version <version>` and signed-off-by line
   - Push the branch to the user's fork (origin remote)
   - **GitLab MR Creation:**
     - Identify the upstream remote name and user's fork remote name from git remotes
     - Create MR using `glab` with the `--head` flag to specify the fork:
       ```bash
       glab mr create \
         --repo <upstream-repo-path> \
         --head <user-fork-path> \
         --source-branch <branch-name> \
         --target-branch master \
         --title "NO-JIRA | release version <version>" \
         --description "<description with SHAs and environments>" \
         --remove-source-branch \
         --yes
       ```
     - The `--head` flag is crucial for creating MRs from forks to upstream repositories

## Errors

| Code                | Description                            |
|---------------------|----------------------------------------|
| INVALID_VERSION     | Version format is incorrect            |
| TAG_ALREADY_EXISTS  | Tag already exists and force=false     |
| GIT_PUSH_FAILED     | Failed to push tag to remote           |
| BUILD_FAILED        | CI pipeline failed to build image      |
| IMAGE_NOT_AVAILABLE | Image not found after successful build |
| NOTIFICATION_FAILED | Failed to notify downstream services   |

## Example

### Input

```yaml
version: "v0.13.2"
```

### Output

```yaml
success: true
tag: "v0.13.2"
gitlab_mr: "https://.../-/merge_requests/12345"
```
