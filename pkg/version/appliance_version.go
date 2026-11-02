package version

import (
	"regexp"
	"strconv"
)

// ApplianceVersion is a parsed appliance image version, as stamped by the
// APPLIANCE_VERSION build arg (assisted-migration-agent) on both the agent
// binary and the OCI org.opencontainers.image.version label.
type ApplianceVersion struct {
	Channel string // release channel: main, stable, prod or pr
	Major   int
	Minor   int
	Patch   int    // prod only
	SHA     string // dev/stage/pr builds only
}

var (
	devStageVersionRe = regexp.MustCompile(`^(main|stable)-release-(\d+)\.(\d+)-([0-9a-f]+)$`)
	prodVersionRe     = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	prVersionRe       = regexp.MustCompile(`^on-pr-([0-9a-f]+)$`)
)

// ParseApplianceVersion parses an appliance version in any environment format:
// main-release-0.31-25fcb31 (dev), stable-release-0.31-25fcb31 (stage),
// v0.31.0 (prod) or on-pr-383f77d (pull-request builds).
func ParseApplianceVersion(ver string) (ApplianceVersion, bool) {
	if m := devStageVersionRe.FindStringSubmatch(ver); m != nil {
		major, _ := strconv.Atoi(m[2])
		minor, _ := strconv.Atoi(m[3])
		return ApplianceVersion{Channel: m[1], Major: major, Minor: minor, SHA: m[4]}, true
	}
	if m := prodVersionRe.FindStringSubmatch(ver); m != nil {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		patch, _ := strconv.Atoi(m[3])
		return ApplianceVersion{Channel: "prod", Major: major, Minor: minor, Patch: patch}, true
	}
	if m := prVersionRe.FindStringSubmatch(ver); m != nil {
		return ApplianceVersion{Channel: "pr", SHA: m[1]}, true
	}
	return ApplianceVersion{}, false
}

// AgentUpdateAvailable reports whether a reported appliance version differs
// from the latest version the service serves, in a way that means an update is
// available. Both versions must be usable (IsValidAgentVersion) and parse to
// the same release channel — versions from different channels are never
// compared. Within a channel any difference (numeric parts or sha) counts: the
// SaaS only serves images it caches, so the served generation is the newer one.
func AgentUpdateAvailable(latest, reported string) bool {
	if !IsValidAgentVersion(latest) || !IsValidAgentVersion(reported) {
		return false
	}
	l, ok := ParseApplianceVersion(latest)
	if !ok {
		return false
	}
	r, ok := ParseApplianceVersion(reported)
	if !ok {
		return false
	}
	if l.Channel != r.Channel {
		return false
	}
	return l != r
}
