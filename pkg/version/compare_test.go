package version

import "testing"

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		name    string
		served  string
		current string
		want    bool
	}{
		{"same version", "v0.31.0", "v0.31.0", false},
		{"newer prod served", "v0.32.0", "v0.31.0", true},
		{"dev sha differs", "main-release-0.31-25fcb31", "main-release-0.31-1da41c5", true},
		{"dev numbers differ", "main-release-0.32-25fcb31", "main-release-0.31-1da41c5", true},
		{"stage sha differs", "stable-release-0.31-25fcb31", "stable-release-0.31-1da41c5", true},
		{"pr sha differs", "on-pr-383f77d", "on-pr-1da41c5", true},
		{"pr same sha", "on-pr-383f77d", "on-pr-383f77d", false},
		{"cross channel", "v0.31.0", "main-release-0.31-25fcb31", false},
		{"current invalid v0.0.0", "v0.31.0", "v0.0.0", false},
		{"current empty", "v0.31.0", "", false},
		{"current unknown", "v0.31.0", "unknown", false},
		{"current DEV", "v0.31.0", "DEV", false},
		{"current unparseable", "v0.31.0", "somejunk", false},
		{"served invalid", "DEV", "v0.31.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateAvailable(tc.served, tc.current); got != tc.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tc.served, tc.current, got, tc.want)
			}
		})
	}
}
