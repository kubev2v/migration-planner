package version_test

import (
	"testing"

	"github.com/kubev2v/migration-planner/pkg/version"
)

func TestParseApplianceVersion(t *testing.T) {
	cases := []struct {
		ver    string
		want   version.ApplianceVersion
		wantOK bool
	}{
		{ver: "main-release-0.31-25fcb31", want: version.ApplianceVersion{Channel: "main", Major: 0, Minor: 31, SHA: "25fcb31"}, wantOK: true},
		{ver: "stable-release-12.3-a1b2c3d", want: version.ApplianceVersion{Channel: "stable", Major: 12, Minor: 3, SHA: "a1b2c3d"}, wantOK: true},
		{ver: "v0.31.0", want: version.ApplianceVersion{Channel: "prod", Major: 0, Minor: 31, Patch: 0}, wantOK: true},
		{ver: "v12.3.45", want: version.ApplianceVersion{Channel: "prod", Major: 12, Minor: 3, Patch: 45}, wantOK: true},
		{ver: "on-pr-383f77d", want: version.ApplianceVersion{Channel: "pr", SHA: "383f77d"}, wantOK: true},
		{ver: "", wantOK: false},
		{ver: "unknown", wantOK: false},
		// v0.0.0 is syntactically a prod version but the pre-OMA-2411 unset
		// default; AgentUpdateAvailable rejects it via IsValidAgentVersion.
		{ver: "v0.0.0", want: version.ApplianceVersion{Channel: "prod"}, wantOK: true},
		{ver: "DEV", wantOK: false},
		{ver: "main-release-0.31", wantOK: false},    // missing sha
		{ver: "main-release-0.31-", wantOK: false},   // empty sha
		{ver: "v0.31", wantOK: false},                // missing patch
		{ver: "release-0.31-25fcb31", wantOK: false}, // missing channel prefix
		{ver: "stable-release-0.31-25fcb31-extra", wantOK: false},
	}
	for _, c := range cases {
		got, ok := version.ParseApplianceVersion(c.ver)
		if ok != c.wantOK {
			t.Errorf("ParseApplianceVersion(%q) ok = %v, want %v", c.ver, ok, c.wantOK)
			continue
		}
		if ok && got != c.want {
			t.Errorf("ParseApplianceVersion(%q) = %+v, want %+v", c.ver, got, c.want)
		}
	}
}

func TestIsValidAgentVersion(t *testing.T) {
	for _, ver := range []string{"", "unknown", "v0.0.0", "DEV"} {
		if version.IsValidAgentVersion(ver) {
			t.Errorf("IsValidAgentVersion(%q) = true, want false", ver)
		}
	}
	for _, ver := range []string{"v0.31.0", "main-release-0.31-25fcb31", "on-pr-383f77d"} {
		if !version.IsValidAgentVersion(ver) {
			t.Errorf("IsValidAgentVersion(%q) = false, want true", ver)
		}
	}
}

func TestAgentUpdateAvailable(t *testing.T) {
	cases := []struct {
		name     string
		latest   string
		reported string
		want     bool
	}{
		{"same dev build", "main-release-0.31-25fcb31", "main-release-0.31-25fcb31", false},
		{"older dev minor", "main-release-0.31-25fcb31", "main-release-0.30-1111111", true},
		{"same numbers different sha", "main-release-0.31-25fcb31", "main-release-0.31-1111111", true},
		{"older prod patch", "v0.31.2", "v0.31.1", true},
		{"same prod version", "v0.31.0", "v0.31.0", false},
		{"newer prod reported", "v0.31.0", "v0.32.0", true},
		{"cross channel dev vs prod", "main-release-0.31-25fcb31", "v0.31.0", false},
		{"cross channel dev vs stage", "main-release-0.31-25fcb31", "stable-release-0.31-25fcb31", false},
		{"latest invalid", "DEV", "v0.31.0", false},
		{"reported invalid", "v0.31.0", "v0.0.0", false},
		{"reported empty", "v0.31.0", "", false},
		{"pr build vs dev latest", "main-release-0.31-25fcb31", "on-pr-383f77d", false},
	}
	for _, c := range cases {
		if got := version.AgentUpdateAvailable(c.latest, c.reported); got != c.want {
			t.Errorf("%s: AgentUpdateAvailable(%q, %q) = %v, want %v", c.name, c.latest, c.reported, got, c.want)
		}
	}
}
