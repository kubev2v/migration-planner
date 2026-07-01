package inventory

import "strings"

const (
	TierCertified          = "certified"
	TierVendorSupported    = "vendor_supported"
	TierCommunitySupported = "community_supported"
	TierSpecialHandling    = "special_handling"
)

// osTierMap maps OS name substrings to support tiers.
// Entries are checked via case-insensitive substring match, same as
// ClassifyOS in pkg/estimations/complexity/complexity.go.
//
// Tier membership follows the Red Hat KCS article:
// https://access.redhat.com/articles/4234591
var osTierMap = map[string]string{
	// --- Certified ---
	"Red Hat Enterprise Linux 7":  TierCertified,
	"Red Hat Enterprise Linux 8":  TierCertified,
	"Red Hat Enterprise Linux 9":  TierCertified,
	"Red Hat Enterprise Linux 10": TierCertified,
	"Windows 10":                  TierCertified,
	"Windows 11":                  TierCertified,
	"Windows Server 2016":         TierCertified,
	"Windows Server 2019":         TierCertified,
	"Windows Server 2022":         TierCertified,
	"Windows Server 2025":         TierCertified,

	// --- Commercial Vendor Supported ---
	"SUSE Linux Enterprise 15": TierVendorSupported,
	"Ubuntu Linux 18.04":       TierVendorSupported,
	"Ubuntu Linux 20.04":       TierVendorSupported,
	"Ubuntu Linux 22.04":       TierVendorSupported,
	"Ubuntu Linux 24.04":       TierVendorSupported,
	"Ubuntu Linux 25.04":       TierVendorSupported,
	"Oracle Linux 8":           TierVendorSupported,
	"Oracle Linux 9":           TierVendorSupported,

	// --- Community Supported ---
	"CentOS Stream 9":     TierCommunitySupported,
	"CentOS Stream 10":    TierCommunitySupported,
	"Debian GNU/Linux 11": TierCommunitySupported,
	"Debian GNU/Linux 12": TierCommunitySupported,
	"Debian GNU/Linux 13": TierCommunitySupported,
	"Fedora":              TierCommunitySupported,
	"openSUSE Leap 15":    TierCommunitySupported,
	"openSUSE Tumbleweed": TierCommunitySupported,
}

// ClassifyOSTier returns the support tier for the given OS name.
// Falls back to TierSpecialHandling for unrecognized OSes.
func ClassifyOSTier(osName string) string {
	normalized := strings.ToLower(osName)
	for keyword, tier := range osTierMap {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			return tier
		}
	}
	return TierSpecialHandling
}
