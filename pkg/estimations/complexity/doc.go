// Package complexity provides pure, stateless functions for scoring VM migration
// complexity. It has no dependency on the HTTP layer or database and is designed
// to be unit-tested in isolation.
//
// Two independent scoring dimensions are supported:
//
//   - OS complexity (scores 0–4): derived by substring-matching the VM's OS name
//     against OSDifficultyScores. Score 1 is the least complex; score 0 means the
//     OS is unknown and complexity can not be assessed.
//
//   - Disk complexity (scores 1–4): derived from the pre-computed disk-size tier
//     labels stored in the inventory by the agent. Score 1 is the least complex;
//     score 4 is the most complex.
package complexity
