---
name: release-notes
description: Use when drafting customer-facing release notes for OpenShift Migration Advisor releases, comparing GitHub release tags across migration-planner and related repos
---

# Skill: Release Notes Drafting

## Overview

Draft customer-facing release notes from GitHub changes between two release tags. Everything is GitHub-based — no Jira CLI or MCP tools needed. Jira ticket IDs are extracted from commit messages only.

## Inputs

- **Target release tag** — the release to generate notes for (e.g., `v0.13.6`)
- **Previous release tag** — the tag to compare against (e.g., `v0.13.5`)

## Repositories

The release spans 4 repositories, all under `kubev2v`:

| Repository | Role | How Included in Release |
|---|---|---|
| `migration-planner` | Backend, API, orchestration | Main repo — release tagged here |
| `assisted-migration-agent` | Discovery/assessment agent | Git submodule at `agent-v2/` |
| `migration-planner-agent-ui` | Agent UI (credential collection, status) | SHA pinned in `build/migration-planner-iso/config` as `AGENT_UI_IMAGE_TAG` |
| `migration-planner-ui-app` | SaaS web UI | Tagged with the same version as migration-planner |

### Source of Truth: migration-planner

The `migration-planner` repo is the **source of truth** for what's included in a release:

- **Agent changes**: Only included if the submodule SHA at `agent-v2/` was updated between the two release tags. To find the range of agent commits in a release:
  ```
  git ls-tree <previous-tag> agent-v2   # → old SHA
  git ls-tree <target-tag> agent-v2     # → new SHA
  ```
  Then compare those SHAs in the `assisted-migration-agent` repo.

- **Agent UI changes**: Only included if `AGENT_UI_IMAGE_TAG` in `build/migration-planner-iso/config` changed between tags:
  ```
  git show <previous-tag>:build/migration-planner-iso/config
  git show <target-tag>:build/migration-planner-iso/config
  ```
  Then compare those SHAs in the `migration-planner-agent-ui` repo.

- **UI App changes**: Compare the same version tags on the `migration-planner-ui-app` repo:
  ```
  gh api repos/kubev2v/migration-planner-ui-app/compare/<previous-tag>...<target-tag>
  ```

## Flow

1. **Determine SHA ranges for each repo:**
   - `migration-planner`: `git log --oneline --no-merges <prev-tag>..<target-tag>`
   - `assisted-migration-agent`: Compare submodule SHAs (see above)
   - `migration-planner-agent-ui`: Compare `AGENT_UI_IMAGE_TAG` values (see above)
   - `migration-planner-ui-app`: Compare same version tags

2. **Fetch commits for each repo** using `gh` CLI or `git log`.

3. **Filter for user-facing changes only.** This is the critical step — see the filtering rules below.

4. **Classify each change** by release note type.

5. **Draft customer-facing text** following the writing guidelines.

## User-Facing Filtering Rules

**Only include changes that are visible to the user.** The release notes are for end users, not engineers.

### Include
- UI changes (new screens, modified workflows, new buttons, visual changes)
- Changes that affect what users see or interact with (new features, changed behavior)
- Fixes for bugs that users could encounter
- Agent UI changes that affect the credential collection or status display

### Exclude
- Backend-only changes (API refactors, internal error handling, database changes) **unless** they result in a visible behavior change
- Agent-only changes (data collection, internal processing) **unless** they affect what shows up in the UI
- Dependency updates, CI/CD changes, build system changes
- Internal refactors, code cleanup, test changes
- Submodule/SHA bump commits themselves (e.g., "Update submodule to reflect the SHA: ...")
- Commits with `NO-JIRA` prefix that are purely infrastructure
- Renovate/Dependabot automated commits

### Cross-Reference Rule
When an agent or backend change has a corresponding UI change in the same release, **describe it from the user's perspective** — what they see changed, not what was changed internally.

## Release Note Types

| Type | Use When | Commit Prefix |
|---|---|---|
| Enhancement | New feature or improvement visible to users | `feat` |
| Bug Fix | Fix for a user-visible defect | `fix` |
| Known Issue | Documented limitation users should know about | N/A |

## Writing Guidelines

### Structure
```
[What changed for the user] + [Why it matters]
```

### Do
- Use "you can now" / "this fixes" / "this adds"
- Focus on user benefits and outcomes
- Keep it 2-3 sentences max
- Use product names customers recognize (Migration Advisor, not "the planner")

### Don't
- Expose internal implementation details
- Use internal acronyms or code references
- Reference internal component names (chi router, envoy, cbor, etc.)
- Include Jira ticket IDs in the note text
- Mention backend/agent internals

### Never Include
- Internal URLs or paths
- Security vulnerability details
- Stack traces or debugging information
- Customer-specific details

## Example

### Wrong
"Refactored credentialUrl validator to reject javascript: protocol URLs in source configuration"

### Right
"Fixed a security issue where certain URL formats were incorrectly accepted when configuring migration sources."

## Output Format

Present the release notes grouped by type:

```markdown
## Release Notes for <version>

### Enhancements
- <note>

### Bug Fixes
- <note>

### Known Issues
- <note>
```

If a release has no user-facing changes, state that explicitly rather than inventing notes.

## Notes

- Engineers review and may edit the generated notes before publishing
- When in doubt about whether a change is user-facing, err on the side of excluding it
