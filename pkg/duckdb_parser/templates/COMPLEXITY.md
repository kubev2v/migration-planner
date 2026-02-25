# VM Migration Complexity Scoring

This document describes how migration complexity is calculated for each VM.

## Overview

Each VM is assigned a complexity score (0-4) based on two factors:
- **OS type** - The operating system running on the VM
- **Total disk size** - Combined capacity of all attached disks

## Complexity Levels

| Level | Value | Description |
|-------|-------|-------------|
| Unsupported | 0 | OS not recognized or not supported for migration |
| Easy | 1 | Simple migration with minimal effort |
| Medium | 2 | Standard migration requiring some attention |
| Hard | 3 | Complex migration requiring significant effort |
| White Glove | 4 | Requires manual intervention and special handling |

## OS Classification

The OS is determined from VMware Tools or configuration file data.

**Source of truth:** `pkg/estimations/complexity/complexity.go` — see `OSDifficultyScores` map.

The SQL template (`populate_complexity.go.tmpl`) auto-generates OS CASE clauses from this map at query build time, ensuring a single source of truth.

| Score | Patterns |
|-------|----------|
| 1 (Easy) | Red Hat, Rocky Linux |
| 2 (Medium) | CentOS, Windows |
| 3 (Hard) | Ubuntu, SUSE Linux Enterprise |
| 4 (Database/White Glove) | Oracle, Microsoft SQL |
| 0 (Unsupported) | Any other OS |

## Disk Size Tiers

Total disk capacity (sum of all vdisks) determines the disk tier:

| Tier | Size Range |
|------|------------|
| Easy | 0 - 10 TB |
| Medium | 10 - 20 TB |
| Hard | 20 - 50 TB |
| White Glove | > 50 TB |

## Combined Complexity Matrix

The final complexity is calculated by combining OS and disk levels:

| OS Level | Disk Level | Final Complexity |
|----------|------------|------------------|
| Unsupported | Any | 0 (Unsupported) |
| Database | Any | 4 (White Glove) |
| Easy | Easy/Medium | 1 (Easy) |
| Easy | Hard/WG | 3 (Hard) |
| Medium | Easy/Medium | 2 (Medium) |
| Medium | Hard/WG | 3 (Hard) |
| Hard | Easy/Medium | 2 (Medium) |
| Hard | Hard/WG | 3 (Hard) |

## Implementation

The complexity is computed via SQL in `populate_complexity.go.tmpl` during data ingestion. The computed value is stored in the `OsDiskComplexity` column of the `vinfo` table.

The distribution query (`complexity_distribution_query.go.tmpl`) aggregates VMs by complexity level for reporting.
