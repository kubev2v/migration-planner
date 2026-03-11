# complexity package

Pure, stateless functions for scoring VM migration complexity. No dependency on the HTTP layer or database — designed to be unit-tested in isolation.

## Score scale

Scores express how much effort a migration is expected to require. Two independent dimensions are scored: OS type and disk size.

| Score | Label | Meaning |
|---|---|---|
| 0 | Unknown | OS not recognised; complexity cannot be assessed |
| 1 | Easy | Straightforward migration, minimal intervention expected |
| 2 | Medium | Standard migration, some attention required |
| 3 | Hard | Complex migration, significant effort expected |
| 4 | White Glove | Requires manual intervention and special handling |

OS scores run 0–4. Disk scores run 1–4 (unknown does not apply to disk size).

---

## OS classification

`ClassifyOS(osName string) Score` performs case-insensitive substring matching against `OSDifficultyScores`. If no keyword matches, the OS receives score **0 (unknown)**.

The full keyword-to-score mapping:

### Score 1 — Easy

| OS family | Matched versions |
|---|---|
| Red Hat Enterprise Linux | 6, 7, 8, 9, 10 |
| Red Hat Fedora | all |
| CentOS | 6, 7, 8, 9 |
| Oracle Linux | 6, 7, 8, 9 |
| Rocky Linux | 8, 9 |

### Score 2 — Medium

| OS family | Matched versions |
|---|---|
| Red Hat Enterprise Linux | 4, 5 |
| CentOS | 4, 5 |
| Oracle Linux | 4, 5 |
| SUSE Linux Enterprise | 12, 15 |
| Microsoft Windows Server | 2016, 2019, 2022, 2025 |
| Microsoft Windows | 10, 11 |

### Score 3 — Hard

| OS family | Matched versions |
|---|---|
| SUSE Linux Enterprise | 8, 9, 10, 11 |
| SUSE openSUSE | all |
| AlmaLinux | 8, 9 |
| Ubuntu Linux | 18.04, 20.04, 22.04, 24.04, generic |
| Debian GNU/Linux | 5–12 |
| Microsoft Windows Server | 2000, 2003, 2008, 2008 R2, 2012, 2012 R2 |
| Microsoft Windows | XP, Vista, 7, 8 |
| Oracle Solaris | 10, 11 |
| FreeBSD | all |
| VMware Photon OS | all |
| Amazon Linux 2 | all |
| CoreOS Linux | all |
| Apple macOS | all |

### Score 4 — White Glove

| OS family | Matched versions |
|---|---|
| Microsoft SQL | all (database workload) |

### Score 0 — Unknown

Any OS name that does not match a keyword in the table above. Examples: "Other (64-bit)", "Other Linux (64-bit)", unrecognised custom distributions.

---

## Disk size classification

`ScoreDiskTierLabel(label string) Score` maps the pre-computed tier labels produced by the agent (stored in `vms.diskSizeTier`) to numeric scores.

| Tier label | Size range | Score |
|---|---|---|
| Easy (0-10TB) | ≤ 10 TB provisioned | 1 |
| Medium (10-20TB) | ≤ 20 TB provisioned | 2 |
| Hard (20-50TB) | ≤ 50 TB provisioned | 3 |
| White Glove (>50TB) | > 50 TB provisioned | 4 |

`DiskSizeRangeRatings()` returns the same map with keys trimmed to the numeric range only (e.g. `"0-10TB"`) for use in API responses.

---

## Functions

| Function | Returns | Description |
|---|---|---|
| `ClassifyOS(osName)` | `Score` | Score for a single OS name string |
| `OSBreakdown(entries)` | `[]OSDifficultyEntry` | VM counts aggregated by score (0–4), always 5 entries |
| `OSNameBreakdown(entries)` | `[]OSNameEntry` | One entry per distinct OS name with its score and VM count |
| `OSRatings(entries)` | `map[string]Score` | OS name → score map (no VM counts) |
| `ScoreDiskTierLabel(label)` | `Score` | Score for a single disk tier label |
| `DiskBreakdown(tiers)` | `[]DiskComplexityEntry` | VM counts and total TB aggregated by score (1–4), always 4 entries |
| `DiskSizeRangeRatings()` | `map[string]Score` | Static tier label → score lookup with range-only keys |
