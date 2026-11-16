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
//
// The bash-side validation lives in hack/appliance-version-regex.sh — keep the
// two in sync when a format changes.
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
		n, ok := atoi(m[1], m[2], m[3])
		if !ok {
			return parsedVersion{}, false
		}
		return parsedVersion{
			channel:     "prod",
			major:       n[0],
			minor:       n[1],
			patch:       n[2],
			hasNumerics: true,
		}, true
	}
	if m := devVersionRe.FindStringSubmatch(v); m != nil {
		n, ok := atoi(m[1], m[2])
		if !ok {
			return parsedVersion{}, false
		}
		return parsedVersion{channel: "dev", major: n[0], minor: n[1], sha: m[3], hasNumerics: true}, true
	}
	if m := stageVersionRe.FindStringSubmatch(v); m != nil {
		n, ok := atoi(m[1], m[2])
		if !ok {
			return parsedVersion{}, false
		}
		return parsedVersion{channel: "stage", major: n[0], minor: n[1], sha: m[3], hasNumerics: true}, true
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

// IsUpdateSupported reports whether the running appliance version can be
// updated in place to the served image: same channel as the minimum and not
// numerically older than it. Not comparable (different channel, missing
// numbers, invalid) gives no signal — the gate only blocks on a proven
// comparison, so it fails open.
func IsUpdateSupported(runningVersion, minVersionUpdateSupported string) bool {
	d, ok := parseApplianceVersion(runningVersion)
	if !ok {
		return true
	}
	m, ok := parseApplianceVersion(minVersionUpdateSupported)
	if !ok || m.channel != d.channel || !d.hasNumerics || !m.hasNumerics {
		return true
	}
	switch {
	case d.major != m.major:
		return d.major > m.major
	case d.minor != m.minor:
		return d.minor > m.minor
	default:
		return d.patch >= m.patch
	}
}

// atoi converts digit-only regex groups. The regexes guarantee digits, so the
// only failure is integer overflow — which makes the version unparseable so
// every comparison fails open instead of ordering saturated values.
func atoi(ss ...string) ([]int, bool) {
	out := make([]int, len(ss))
	for i, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}
