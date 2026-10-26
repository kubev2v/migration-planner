package inventory

import "strings"

const (
	MTVStatusGreen = "green"
	MTVStatusBlue  = "blue"
)

// ClassifyOSMTVStatus returns the migration verification status from KCS 4234591
// (Updated September 24 2026). A blank status means the OS/architecture is not listed.
// VMware's "(64-bit)" guest names denote x86; ARM and s390x
// entries have no MTV status in the article and must not inherit the x86 status.
func ClassifyOSMTVStatus(osName string) string {
	name := strings.ToLower(strings.TrimSpace(osName))
	if strings.Contains(name, "arm") || strings.Contains(name, "aarch64") || strings.Contains(name, "s390") {
		return ""
	}
	is64 := strings.HasSuffix(name, " (64-bit)")
	name = strings.TrimSuffix(name, " (64-bit)")
	name = strings.TrimPrefix(name, "microsoft ")
	if is64 {
		switch name {
		case "red hat enterprise linux 7", "red hat enterprise linux 8", "red hat enterprise linux 9", "red hat enterprise linux 10":
			return MTVStatusGreen
		case "red hat enterprise linux 6", "windows 10", "windows 11", "windows server 2012 r2", "windows server 2016", "windows server 2019", "windows server 2022", "windows server 2025":
			return MTVStatusBlue
		}
	}
	return ""
}
