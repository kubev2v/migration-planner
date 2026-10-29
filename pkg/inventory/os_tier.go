package inventory

import "strings"

const (
	TierCertified          = "certified"
	TierVendorSupported    = "vendor_supported"
	TierCommunitySupported = "community_supported"
	TierSpecialHandling    = "special_handling"
)

type osClass struct{ tier, mtvStatus string }

// osClassMap maps OS name substrings to support tiers and MTV statuses.
// Entries are checked via case-insensitive substring match, same as
// ClassifyOS in pkg/estimations/complexity/complexity.go.
//
// Tier membership and MTV status follow the Red Hat KCS article:
// https://access.redhat.com/articles/4234591 (Updated September 24 2026)
var osClassMap = map[string]osClass{
	// --- Certified ---
	"Red Hat Enterprise Linux 7":  {TierCertified, MTVStatusGreen},
	"Red Hat Enterprise Linux 8":  {TierCertified, MTVStatusGreen},
	"Red Hat Enterprise Linux 9":  {TierCertified, MTVStatusGreen},
	"Red Hat Enterprise Linux 10": {TierCertified, MTVStatusGreen},
	"Windows 10":                  {TierCertified, MTVStatusBlue},
	"Windows 11":                  {TierCertified, MTVStatusBlue},
	"Windows Server 2016":         {TierCertified, MTVStatusBlue},
	"Windows Server 2019":         {TierCertified, MTVStatusBlue},
	"Windows Server 2022":         {TierCertified, MTVStatusBlue},
	"Windows Server 2025":         {TierCertified, MTVStatusBlue},

	// --- Commercial Vendor Supported ---
	"SUSE Linux Enterprise 15": {TierVendorSupported, ""},
	"SUSE Linux Enterprise 16": {TierVendorSupported, ""},
	"Ubuntu Linux 18.04":       {TierVendorSupported, ""},
	"Ubuntu Linux 20.04":       {TierVendorSupported, ""},
	"Ubuntu Linux 22.04":       {TierVendorSupported, ""},
	"Ubuntu Linux 24.04":       {TierVendorSupported, ""},
	"Ubuntu Linux 25.04":       {TierVendorSupported, ""},
	"Oracle Linux 8":           {TierVendorSupported, ""},
	"Oracle Linux 9":           {TierVendorSupported, ""},
	"Oracle Linux 10":          {TierVendorSupported, ""},

	// --- Community Supported ---
	"AlmaLinux 9":         {TierCommunitySupported, ""},
	"AlmaLinux 10":        {TierCommunitySupported, ""},
	"CentOS Stream 9":     {TierCommunitySupported, ""},
	"CentOS Stream 10":    {TierCommunitySupported, ""},
	"Debian GNU/Linux 11": {TierCommunitySupported, ""},
	"Debian GNU/Linux 12": {TierCommunitySupported, ""},
	"Debian GNU/Linux 13": {TierCommunitySupported, ""},
	"Fedora":              {TierCommunitySupported, ""},
	"openSUSE Leap 15.6":  {TierCommunitySupported, ""},
	"openSUSE Tumbleweed": {TierCommunitySupported, ""},
	"openSUSE Leap 16":    {TierCommunitySupported, ""},

	// --- Special Handling, but listed with an MTV status ---
	"Red Hat Enterprise Linux 6": {TierSpecialHandling, MTVStatusBlue},
	"Windows Server 2012 R2":     {TierSpecialHandling, MTVStatusBlue},
}

func classifyOS(osName string) osClass {
	normalized := strings.ToLower(osName)
	for keyword, class := range osClassMap {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			return class
		}
	}
	return osClass{tier: TierSpecialHandling}
}

// ClassifyOSTier returns the support tier for the given OS name.
// Falls back to TierSpecialHandling for unrecognized OSes.
func ClassifyOSTier(osName string) string {
	return classifyOS(osName).tier
}
