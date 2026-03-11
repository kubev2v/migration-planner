# Complexity scoring in the duckdb_parser package

Score definitions and OS/disk heuristics are owned by the `pkg/estimations/complexity` package — see its README for the full reference.

This document covers only what is specific to how complexity is computed during ingestion.

---

## How OS classification works in the templates

OS scoring is not hardcoded in the SQL templates. Instead, the CASE clauses are **generated at query-build time** from `OSDifficultyScores` in `pkg/estimations/complexity/complexity.go`.

The template placeholder `{{.OSCaseClauses}}` in `populate_complexity.go.tmpl` is replaced with one `WHEN ... THEN '...'` clause per entry in that map, ensuring a single source of truth for OS scores across both the API response path and the ingestion path.

## Disk size tiers in SQL

The disk thresholds are expressed directly in SQL within `populate_complexity.go.tmpl`:

| SQL condition | Tier label | Score |
|---|---|---|
| `total_disk_tb < 10` | Easy | 1 |
| `total_disk_tb < 20` | Medium | 2 |
| `total_disk_tb < 50` | Hard | 3 |
| `else` | White Glove | 4 |

Note: these thresholds are currently hardcoded in the template separately from `DiskSizeScores` in the complexity package. They must be kept in sync manually if the tier boundaries change.

## Combined OS × disk matrix

The final per-VM score is computed in `populate_complexity.go.tmpl` by combining the OS level and disk level:

| OS level | Disk level | Final score |
|---|---|---|
| unknown | any | 0 |
| database | any | 4 |
| easy | easy / medium | 1 |
| easy | hard / white glove | 3 |
| medium | easy / medium | 2 |
| medium | hard / white glove | 3 |
| hard | easy / medium | 2 |
| hard | hard / white glove | 3 |

The result is stored in the `OsDiskComplexity` column of the `vinfo` table during ingestion.

## Relevant templates

| Template | Purpose |
|---|---|
| `populate_complexity.go.tmpl` | Computes and stores `OsDiskComplexity` for every VM row |
| `complexity_distribution_query.go.tmpl` | Aggregates VM counts by `OsDiskComplexity` for reporting |
