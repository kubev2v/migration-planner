package inventory

import "strings"

const (
	MTVStatusGreen = "green"
	MTVStatusBlue  = "blue"
)

// ClassifyOSMTVStatus returns the migration verification status for the given OS name.
// A blank status means the OS/architecture is not listed. The article lists MTV status
// only for x86_64, so 32-bit, ARM and s390x guests must not inherit it.
func ClassifyOSMTVStatus(osName string) string {
	name := strings.ToLower(osName)
	for _, arch := range []string{"32-bit", "arm", "aarch64", "s390"} {
		if strings.Contains(name, arch) {
			return ""
		}
	}
	return classifyOS(osName).mtvStatus
}
