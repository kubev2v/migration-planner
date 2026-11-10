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
}

// parseApplianceVersion returns the parsed version and whether it is a known
// format. "v0.0.0" is permanently invalid: appliances deployed before agent
// PR #571 keep reporting it until updated.
func parseApplianceVersion(v string) (parsedVersion, bool) {
	switch {
	case v == "" || v == "unknown" || v == "DEV" || v == "v0.0.0":
		return parsedVersion{}, false
	}
	if m := prodVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{
			channel: "prod",
			major:   atoi(m[1]),
			minor:   atoi(m[2]),
			patch:   atoi(m[3]),
		}, true
	}
	if m := devVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "dev", major: atoi(m[1]), minor: atoi(m[2]), sha: m[3]}, true
	}
	if m := stageVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "stage", major: atoi(m[1]), minor: atoi(m[2]), sha: m[3]}, true
	}
	if m := prVersionRe.FindStringSubmatch(v); m != nil {
		return parsedVersion{channel: "pr", sha: m[1]}, true
	}
	return parsedVersion{}, false
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

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
