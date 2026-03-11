package complexity

import (
	"regexp"
	"sort"
	"strings"
)

// Score is a numeric complexity level. Lower values indicate simpler migrations.
// OS scores run 0–4; disk scores run 1–4.
type Score = int

// OSScores lists all valid OS complexity scores in canonical order.
var OSScores = []Score{0, 1, 2, 3, 4}

// DiskScores lists all valid disk complexity scores in canonical order.
var DiskScores = []Score{1, 2, 3, 4}

// OSDifficultyScores maps OS keyword substrings to their numeric complexity score.
// ClassifyOS checks whether each key is a substring of the VM's OS name.
//
// The keyword set is designed so that no standard VMware OS name matches more than
// one keyword, making map-iteration order irrelevant in practice. If you add a
// keyword that could overlap with an existing one, prefer the ordered-slice approach
// instead to guarantee deterministic results.
//
// Edit this map to adjust OS complexity scoring. The mapping follows the
// migration difficulty table in complexity.md:
//
//	Score 0 — fallback when no keyword matches (unclassified)
//	Score 1 — Red Hat Enterprise Linux, Rocky Linux
//	Score 2 — CentOS, Windows
//	Score 3 — Ubuntu, SUSE Linux Enterprise
//	Score 4 — Oracle, Microsoft SQL (database workloads)
var OSDifficultyScores = map[string]Score{
	"Red Hat":               1,
	"Rocky Linux":           1,
	"CentOS":                2,
	"Windows":               2,
	"Ubuntu":                3,
	"SUSE Linux Enterprise": 3,
	"Oracle":                4,
	"Microsoft SQL":         4,
}

// DiskSizeScores maps the pre-computed inventory tier label strings (produced by
// the agent) to numeric complexity scores. The thresholds follow complexity.md:
//
//	Score 1 — provisioned disk ≤ 10 TB
//	Score 2 — provisioned disk ≤ 20 TB
//	Score 3 — provisioned disk ≤ 50 TB
//	Score 4 — provisioned disk  > 50 TB
//
// Edit this map to adjust disk complexity scoring.
var DiskSizeScores = map[string]Score{
	"Easy (0-10TB)":       1,
	"Medium (10-20TB)":    2,
	"Hard (20-50TB)":      3,
	"White Glove (>50TB)": 4,
}

// ClassifyOS returns the numeric complexity score for the given OS name by
// checking whether each keyword in OSDifficultyScores appears as a substring.
// Returns 0 if no keyword matches (unclassified).
func ClassifyOS(osName string) Score {
	normalized := strings.ToLower(osName)
	for keyword, score := range OSDifficultyScores {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			return score
		}
	}
	return 0
}

// ScoreDiskTierLabel returns the numeric score for an inventory disk tier label.
// Returns 0 if the label is not in DiskSizeScores.
func ScoreDiskTierLabel(label string) Score {
	return DiskSizeScores[label]
}

// extracts the actual ranges from the keys of the DiskSizeScores (this is just formatting for the API layer)
var diskTierRangeRe = regexp.MustCompile(`\(([^)]+)\)`)

// DiskSizeRangeRatings returns the DiskSizeScores map with keys reformatted to
// contain only the numeric range portion of each tier label (e.g. "Easy (0-10TB)"
// becomes "0-10TB"). Use this when exposing the lookup table in API responses so
// that UI-level label words ("Easy", "Hard", etc.) do not bleed into the data layer.
func DiskSizeRangeRatings() map[string]Score {
	result := make(map[string]Score, len(DiskSizeScores))
	for label, score := range DiskSizeScores {
		if m := diskTierRangeRe.FindStringSubmatch(label); m != nil {
			result[m[1]] = score
		} else {
			result[label] = score
		}
	}
	return result
}

// VMOsEntry represents a single OS in the inventory with its VM count.
type VMOsEntry struct {
	Name  string // OS name as reported by VMware (e.g., "Red Hat Enterprise Linux 8 (64-bit)")
	Count int    // Number of VMs running this OS
}

// DiskTierInput represents a pre-computed disk size tier from the inventory.
type DiskTierInput struct {
	Label       string // Tier label as stored in the inventory (key in vms.diskSizeTier)
	VMCount     int
	TotalSizeTB float64
}

// OSDifficultyEntry is one row in the OS complexity breakdown.
type OSDifficultyEntry struct {
	Score   Score
	VMCount int
}

// DiskComplexityEntry is one row in the disk complexity breakdown.
type DiskComplexityEntry struct {
	Score       Score
	VMCount     int
	TotalSizeTB float64
}

// OSBreakdown classifies each OS entry by numeric score and returns a slice
// of OSDifficultyEntry in canonical score order (0–4).
// All five scores are always present; absent scores carry VMCount == 0.
func OSBreakdown(osEntries []VMOsEntry) []OSDifficultyEntry {
	counts := make(map[Score]int)
	for _, entry := range osEntries {
		counts[ClassifyOS(entry.Name)] += entry.Count
	}
	result := make([]OSDifficultyEntry, len(OSScores))
	for i, s := range OSScores {
		result[i] = OSDifficultyEntry{Score: s, VMCount: counts[s]}
	}
	return result
}

// OSNameEntry is one row in the per-OS-name complexity breakdown.
type OSNameEntry struct {
	Name    string // OS name as reported by VMware
	Score   Score
	VMCount int
}

// OSNameBreakdown returns one OSNameEntry per distinct OS name in osEntries.
func OSNameBreakdown(osEntries []VMOsEntry) []OSNameEntry {
	result := make([]OSNameEntry, len(osEntries))
	for i, e := range osEntries {
		result[i] = OSNameEntry{
			Name:    e.Name,
			Score:   ClassifyOS(e.Name),
			VMCount: e.Count,
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score < result[j].Score
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// OSRatings returns a map from OS name to complexity score for every entry in
// the given slice. Each name is classified via ClassifyOS. Callers can use this
// map to show per-OS score annotations alongside the aggregate OSBreakdown result.
func OSRatings(osEntries []VMOsEntry) map[string]Score {
	result := make(map[string]Score, len(osEntries))
	for _, e := range osEntries {
		result[e.Name] = ClassifyOS(e.Name)
	}
	return result
}

// DiskBreakdown maps pre-computed inventory tier labels to numeric scores and
// returns a slice of DiskComplexityEntry in canonical score order (1–4).
// All four scores are always present; absent scores carry VMCount == 0 and TotalSizeTB == 0.
func DiskBreakdown(tiers []DiskTierInput) []DiskComplexityEntry {
	byScore := make(map[Score]DiskTierInput)
	for _, t := range tiers {
		if s := ScoreDiskTierLabel(t.Label); s != 0 {
			byScore[s] = t
		}
	}
	result := make([]DiskComplexityEntry, len(DiskScores))
	for i, s := range DiskScores {
		t := byScore[s]
		result[i] = DiskComplexityEntry{Score: s, VMCount: t.VMCount, TotalSizeTB: t.TotalSizeTB}
	}
	return result
}
