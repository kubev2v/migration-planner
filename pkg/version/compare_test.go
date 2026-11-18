package version

import "testing"

func TestSameChannel(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"prod", "v0.31.0", "v0.32.0", true},
		{"identical", "v0.31.0", "v0.31.0", true},
		{"dev", "main-release-0.31-abcdef0", "main-release-0.32-1234567", true},
		{"stage", "stable-release-0.31-abcdef0", "stable-release-0.32-1234567", true},
		{"pr", "on-pr-abcdef0", "on-pr-1234567", true},
		{"prod and dev", "v0.31.0", "main-release-0.31-abcdef0", false},
		{"dev and stage", "main-release-0.31-abcdef0", "stable-release-0.31-abcdef0", false},
		{"prod and pr", "v0.31.0", "on-pr-abcdef0", false},
		{"empty", "", "v0.31.0", false},
		{"unknown", "unknown", "v0.31.0", false},
		{"legacy", "v0.0.0", "v0.31.0", false},
		{"malformed", "0.31.0", "v0.31.0", false},
		{"both invalid", "junk", "junk", false},
		{"overflow", "v9223372036854775808.0.0", "v0.31.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameChannel(tc.a, tc.b); got != tc.want {
				t.Errorf("SameChannel(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := SameChannel(tc.b, tc.a); got != tc.want {
				t.Errorf("SameChannel(%q, %q) = %v, want %v", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

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
		{"served major overflow", "v9223372036854775808.0.0", "v0.31.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateAvailable(tc.served, tc.current); got != tc.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tc.served, tc.current, got, tc.want)
			}
		})
	}
}

func TestIsUpdateSupported(t *testing.T) {
	cases := []struct {
		name         string
		running      string
		minSupported string
		want         bool
	}{
		{"prod below min", "v0.30.5", "v0.31.0", false},
		{"prod at min", "v0.31.0", "v0.31.0", true},
		{"prod above min", "v0.32.1", "v0.31.0", true},
		{"prod major below", "v0.9.9", "v0.31.0", false},
		{"dev below min", "main-release-0.30-1da41c5", "main-release-0.31-25fcb31", false},
		{"dev at min numbers", "main-release-0.31-1da41c5", "main-release-0.31-25fcb31", true},
		{"dev above min", "main-release-0.32-1da41c5", "main-release-0.31-25fcb31", true},
		{"stage vs dev min fails open", "stable-release-0.30-1da41c5", "main-release-0.31-25fcb31", true},
		{"prod vs dev min fails open", "v0.30.5", "main-release-0.31-25fcb31", true},
		{"pr fails open", "on-pr-383f77d", "main-release-0.31-25fcb31", true},
		{"empty min no gating", "v0.30.5", "", true},
		{"running invalid fails open", "v0.0.0", "v0.31.0", true},
		{"min invalid fails open", "v0.30.5", "junk", true},
		{"running major overflow fails open", "v9223372036854775808.0.0", "v9.0.0", true},
		{"min major overflow fails open", "v9.0.0", "v9223372036854775808.0.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUpdateSupported(tc.running, tc.minSupported); got != tc.want {
				t.Errorf("IsUpdateSupported(%q, %q) = %v, want %v", tc.running, tc.minSupported, got, tc.want)
			}
		})
	}
}
