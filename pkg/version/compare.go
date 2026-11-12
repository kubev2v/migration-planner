package version

import (
	"regexp"
	"strconv"
)

// Appliance version formats, all stamped from the same build arg (ADR:
// appliance version reporting). Cross-channel pairs never occur in one
// environment and are not comparable.
//
//	prod:  v<major>.<minor>.<patch>          e.g. v0.31.0
//	dev:   main-release-<major>.<minor>-<sha> e.g. main-release-0.31-25fcb31
//	stage: stable-release-<major>.<minor>-<sha>
//	pr:    on-pr-<sha>
var (
	prodVersionRe  = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	devVersionRe   = regexp.MustCompile(`^main-release-(\d+)\.(\d+)-([0-9a-f]{7})$`)
	stageVersionRe = regexp.MustCompile(`^stable-release-(\d+)\.(\d+)-([0-9a-f]{7})$`)
	prVersionRe    = regexp.MustCompile(`^on-pr-([0-9a-f]{7})$`)
)

type parsedVersion struct {
	channel string
	major   int
	minor   int
	patch   int
	sha     string
	// hasNumerics is false for pr-channel versions (sha only), which cannot
	// be ordered against a minimum.
	hasNumerics bool
}

// parseApplianceVersion returns the parsed version and whether it is a known
// format. "v0.0.0" is permanently invalid: appliances deployed before agent
// PR #571 keep reporting it until updated.
func parseApplianceVersion(v string) (parsedVersion, bool) {
	switch v {
	case "", "unknown", "DEV", "v0.0.0":
		return parsedVersion{}, false
	}
	if m := prodVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{
			channel:     "prod",
			major:       atoi(m[1]),
			minor:       atoi(m[2]),
			patch:       atoi(m[3]),
			hasNumerics: true,
		}, true
	}
	if m := devVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "dev", major: atoi(m[1]), minor: atoi(m[2]), sha: m[3], hasNumerics: true}, true
	}
	if m := stageVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "stage", major: atoi(m[1]), minor: atoi(m[2]), sha: m[3], hasNumerics: true}, true
	}
	if m := prVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "pr", sha: m[1]}, true
	}
	return parsedVersion{}, false
}

// IsKnownFormat reports whether v parses as one of the four appliance version
// formats — the same validity rule UpdateAvailable uses, unlike the looser
// IsValidAgentVersion which accepts anything non-empty and non-"unknown".
func IsKnownFormat(v string) bool {
	_, ok := parseApplianceVersion(v)
	return ok
}

// UpdateAvailable reports whether the served appliance version differs from
// the deployed one per the channel-aware rule: any difference within a
// channel (numbers or sha) is an update; anything not comparable gives no
// signal. The served version is only ever what the SaaS distributes, so a
// differing sha means the served generation is newer.
func UpdateAvailable(served, deployed string) bool {
	s, ok := parseApplianceVersion(served)
	if !ok {
		return false
	}
	d, ok := parseApplianceVersion(deployed)
	if !ok || s.channel != d.channel {
		return false
	}
	return s != d
}

// BelowMin reports whether the deployed appliance version is in the same
// channel as min and numerically older than it — the in-place update path
// must not be offered. Not comparable (different channel, missing numbers,
// invalid) gives no signal: the gate only blocks on a proven comparison.
func BelowMin(deployed, min string) bool {
	d, ok := parseApplianceVersion(deployed)
	if !ok {
		return false
	}
	m, ok := parseApplianceVersion(min)
	if !ok || m.channel != d.channel || !d.hasNumerics || !m.hasNumerics {
		return false
	}
	switch {
	case d.major != m.major:
		return d.major < m.major
	case d.minor != m.minor:
		return d.minor < m.minor
	default:
		return d.patch < m.patch
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
