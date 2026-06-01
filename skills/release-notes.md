# Skill: Release Notes Drafting

## Overview

Draft customer-facing release notes from Jira tickets for OpenShift Migration Advisor releases. Converts technical ticket content into clear, user-friendly text.

## Use Cases

- Drafting release note text from ECOPROJECT Jira tickets
- Converting technical changes into customer-facing language

## Requirements

- Access to ticket via Jira MCP tool, cli or user-provided details
- Clone of the repositories of assisted-migration-agent, migration-planner, migration planner-ui-app.

## Inputs

- GitHub release tag to generate the release notes for. 
- Previous GitHub release tag to compare with

## Data Sources to Analyze

1. **Ticket Description** - Primary context for what changed and why
2. **Linked PRs/Commits** - Look for conventional commit types (`feat:`, `fix:`, etc.)
3. **Technical Comments** - Implementation details to translate (not copy verbatim)

## Release Note Types

| Type               | Use When                   | Maps From Commit         |
|--------------------|----------------------------|--------------------------|
| Bug Fix            | Patches a defect           | `fix`                    |
| Enhancement        | New feature or improvement | `feat`, `perf`           |
| Technology Preview | Experimental feature       | `feat` + preview context |
| Deprecated         | Feature marked for removal | N/A                      |
| Removed            | Feature removed            | N/A                      |
| Known Issue        | Documented limitation      | N/A                      |

## Writing Guidelines

### Structure
```
[What changed] + [Why it matters] + [Optional: How to use]
```

### Do
- Use "you can now" / "this fixes" / "this adds"
- Focus on user benefits and outcomes
- Keep it 2-3 sentences max
- Use product names customers recognize

### Don't
- Expose internal implementation details
- Use undefined jargon or acronyms
- Include code snippets or API details
- Reference internal components users don't see

### Security - Never Include
- Internal URLs or paths
- Customer-specific details
- Security vulnerability details
- Debugging information or stack traces

## Flow / Logic

1. Fetch Jira ticket data (MCP tool, cli or ask user for details) compare and analyze the actual GitHub changes
2. Extract key information:
   - What problem was solved?
   - What capability was added?
   - User-facing impact (not implementation)
3. Identify Release Note Type from commit types/context
4. Draft customer-facing text following writing guidelines
5. Present draft with suggested Jira field values

## Outputs

| Name              | Type   | Description                                  |
|-------------------|--------|----------------------------------------------|
| release_note_type | string | Bug Fix, Enhancement, etc.                   |
| draft_text        | string | Customer-facing release note (2-3 sentences) |

## Common Mistakes to Avoid

| Wrong ❌                                         | Right ✅                                           |
|-------------------------------------------------|---------------------------------------------------|
| "Refactored the auth module to use new library" | "Authentication is now more reliable"             |
| "Fixed NPE in VMwareClient.java line 42"        | "Fixed crash when connecting to VMware source"    |
| "Updated to use v2 API endpoints"               | "Improved compatibility with latest OpenShift"    |
| "Implements RFC-9999 compliance"                | "Supports industry-standard configuration format" |

## Example

### Input
**Tag:** Release noted for V0.10.0

### Notes

- Engineers manually update Jira fields. Claude does not modify Jira tickets.
