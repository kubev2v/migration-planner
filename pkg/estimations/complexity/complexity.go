package complexity

import (
	"regexp"
	"strings"
)

// Score is a numeric complexity level. Lower values indicate simpler migrations.
// OS scores run 0–4; disk scores run 1–4.
type Score = int

// OSScores lists all valid OS complexity scores in canonical order.
var OSScores = []Score{0, 1, 2, 3, 4}

// DiskScores lists all valid disk complexity scores in canonical order.
var DiskScores = []Score{1, 2, 3, 4}

// OSDifficultyScores maps OS name substrings to numeric complexity scores.
// ClassifyOS checks whether each key is a case-insensitive substring of the VM's OS name.
//
// When multiple keys match the same OS name, all matching keys carry the same score,
// so non-deterministic map iteration order does not affect the result.
//
// Score semantics:
//
//	0 = unknown (no key matched)
//	1 = score 1 easiest
//	2 = score 2
//	3 = score 3
//	4 = score 4 hardest
var OSDifficultyScores = map[string]Score{
	// --- Red Hat Enterprise Linux ---
	"Red Hat Enterprise Linux 4":  2,
	"Red Hat Enterprise Linux 5":  2,
	"Red Hat Enterprise Linux 6":  1,
	"Red Hat Enterprise Linux 7":  1,
	"Red Hat Enterprise Linux 8":  1,
	"Red Hat Enterprise Linux 9":  1,
	"Red Hat Enterprise Linux 10": 1,
	"Red Hat Fedora":              1,

	// --- CentOS ---
	"CentOS 4": 2,
	"CentOS 5": 2,
	"CentOS 6": 1,
	"CentOS 7": 1,
	"CentOS 8": 1,
	"CentOS 9": 1,

	// --- Oracle Linux ---
	"Oracle Linux 4": 2,
	"Oracle Linux 5": 2,
	"Oracle Linux 6": 1,
	"Oracle Linux 7": 1,
	"Oracle Linux 8": 1,
	"Oracle Linux 9": 1,

	// --- Rocky Linux ---
	"Rocky Linux 8": 1,
	"Rocky Linux 9": 1,

	// --- AlmaLinux ---
	"AlmaLinux 8": 3,
	"AlmaLinux 9": 3,

	// --- SUSE Linux Enterprise ---
	"SUSE Linux Enterprise 8":  3,
	"SUSE Linux Enterprise 9":  3,
	"SUSE Linux Enterprise 10": 3,
	"SUSE Linux Enterprise 11": 3,
	"SUSE Linux Enterprise 12": 2,
	"SUSE Linux Enterprise 15": 2,
	"SUSE openSUSE":            3,

	// --- Ubuntu ---
	"Ubuntu Linux 18.04": 3,
	"Ubuntu Linux 20.04": 3,
	"Ubuntu Linux 22.04": 3,
	"Ubuntu Linux 24.04": 3,
	"Ubuntu Linux":       3,

	// --- Debian ---
	"Debian GNU/Linux 5":  3,
	"Debian GNU/Linux 6":  3,
	"Debian GNU/Linux 7":  3,
	"Debian GNU/Linux 8":  3,
	"Debian GNU/Linux 9":  3,
	"Debian GNU/Linux 10": 3,
	"Debian GNU/Linux 11": 3,
	"Debian GNU/Linux 12": 3,

	// --- Windows Server ---
	"Microsoft Windows Server 2000":    3,
	"Microsoft Windows Server 2003":    3,
	"Microsoft Windows Server 2008 R2": 3,
	"Microsoft Windows Server 2008":    3,
	"Microsoft Windows Server 2012 R2": 3,
	"Microsoft Windows Server 2012":    3,
	"Microsoft Windows Server 2016":    2,
	"Microsoft Windows Server 2019":    2,
	"Microsoft Windows Server 2022":    2,
	"Microsoft Windows Server 2025":    2,

	// --- Windows Desktop ---
	"Microsoft Windows XP":    3,
	"Microsoft Windows Vista": 3,
	"Microsoft Windows 7":     3,
	"Microsoft Windows 8":     3,
	"Microsoft Windows 10":    2,
	"Microsoft Windows 11":    2,

	// --- Oracle Solaris ---
	"Oracle Solaris 10": 3,
	"Oracle Solaris 11": 3,

	// --- FreeBSD ---
	"FreeBSD": 3,

	// --- Other explicitly-rated OSes ---
	"VMware Photon OS": 3,
	"Amazon Linux 2":   3,
	"CoreOS Linux":     3,
	"Apple macOS":      3,

	"Microsoft SQL": 4,
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
// checking whether each key in OSDifficultyScores appears as a case-insensitive
// substring of osName. Returns 0 if no key matches (unknown).
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
