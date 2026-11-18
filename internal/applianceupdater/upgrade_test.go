package applianceupdater

import (
	"context"
	"strings"
	"testing"
)

func TestCompatibilityUsesCandidateMinimumAndReleaseChannel(t *testing.T) {
	cases := []struct {
		name                        string
		running, candidate, minimum string
		wantError                   string
	}{
		{"prod at minimum", "v0.31.0", "v0.32.0", "v0.31.0", ""},
		{"prod above minimum", "v0.31.1", "v0.32.0", "v0.31.0", ""},
		{"prod below minimum", "v0.30.9", "v0.32.0", "v0.31.0", "below"},
		{"numeric ordering", "v0.9.0", "v0.32.0", "v0.10.0", "below"},
		{"no minimum", "v0.30.0", "v0.32.0", "", ""},
		{"same version different image", "v0.31.0", "v0.31.0", "v0.31.0", ""},
		{"different channels", "main-release-0.31-abcdef0", "v0.32.0", "", "same release channel"},
		{"dev and stage", "main-release-0.31-abcdef0", "stable-release-0.32-1234567", "", "same release channel"},
		{"invalid running", "unknown", "v0.32.0", "", "known formats"},
		{"invalid candidate", "v0.31.0", "junk", "", "known formats"},
		{"legacy running", "v0.0.0", "v0.32.0", "v0.31.0", "known formats"},
		{"invalid minimum", "v0.31.0", "v0.32.0", "junk", "candidate minimum"},
		{"minimum channel mismatch", "v0.31.0", "v0.32.0", "main-release-0.31-abcdef0", "candidate minimum"},
		{"minimum overflow", "v0.31.0", "v0.32.0", "v9223372036854775808.0.0", "candidate minimum"},
		{"dev at minimum", "main-release-0.31-abcdef0", "main-release-0.32-1234567", "main-release-0.31-7654321", ""},
		{"dev below minimum", "main-release-0.30-abcdef0", "main-release-0.32-1234567", "main-release-0.31-7654321", "below"},
		{"stage at minimum", "stable-release-0.31-abcdef0", "stable-release-0.32-1234567", "stable-release-0.31-7654321", ""},
		{"stage below minimum", "stable-release-0.30-abcdef0", "stable-release-0.32-1234567", "stable-release-0.31-7654321", "below"},
		{"pr without minimum", "on-pr-abcdef0", "on-pr-1234567", "", ""},
		// PR hashes have no numeric ordering; preserve IsUpdateSupported semantics.
		{"pr with minimum", "on-pr-abcdef0", "on-pr-1234567", "on-pr-7654321", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, h := fixture(t)
			previous, candidate := oldImage, newImage
			previous.Version = tc.running
			// Only the candidate's minimum controls this update.
			previous.MinUpgradeVersion = "v99.0.0"
			candidate.Version, candidate.MinUpgradeVersion = tc.candidate, tc.minimum
			err := c.checkCompatibility(context.Background(), previous, candidate)
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				if h.extensionChecks != 2 {
					t.Fatal("compatible versions bypassed extension validation")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %q error, got %v", tc.wantError, err)
				}
				if h.extensionChecks != 0 {
					t.Fatal("invalid versions reached extension probes")
				}
			}
		})
	}
}
