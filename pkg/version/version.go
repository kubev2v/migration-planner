package version

import (
	"fmt"
	"runtime"
)

var (
	// commitFromGit is a constant representing the source version that
	// generated this build. It should be set during build via -ldflags.
	commitFromGit string
	// versionFromGit is a constant representing the version tag that
	// generated this build. It should be set during build via -ldflags.
	versionFromGit = "unknown"
	// major version
	majorFromGit string
	// minor version
	minorFromGit string
	// patch version
	patchFromGit string
	// build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	buildDate string
	// state of git tree, either "clean" or "dirty"
	gitTreeState string
	// applianceCommit is the agent commit the embedded appliance image was built
	// from, empty for release tags. It should be set during build via -ldflags.
	applianceCommit string
	// applianceVersion is the version of the appliance image embedded in the ISO
	// this build serves. It should be set during build via -ldflags.
	applianceVersion string
	// minApplianceVersion is the oldest appliance version the served image
	// supports in-place updates from (io.kubev2v.oma.min-upgrade-version label
	// of the appliance image, written to the ISO build config on release).
	// Empty means no gating. It should be set during build via -ldflags.
	minApplianceVersion string
)

type Info struct {
	Major            string `json:"major"`
	Minor            string `json:"minor"`
	GitVersion       string `json:"gitVersion"`
	GitCommit        string `json:"gitCommit"`
	GitTreeState     string `json:"gitTreeState"`
	BuildDate        string `json:"buildDate"`
	GoVersion        string `json:"goVersion"`
	Compiler         string `json:"compiler"`
	Platform         string `json:"platform"`
	Patch            string `json:"patch"`
	AgentGitCommit   string `json:"agentGitCommit"`
	AgentVersionName string `json:"agentVersionName"`
}

func (info Info) String() string {
	return info.GitVersion
}

func Get() Info {
	return Info{
		Major:            majorFromGit,
		Minor:            minorFromGit,
		GitCommit:        commitFromGit,
		GitVersion:       versionFromGit,
		GitTreeState:     gitTreeState,
		BuildDate:        buildDate,
		GoVersion:        runtime.Version(),
		Compiler:         runtime.Compiler,
		Platform:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Patch:            patchFromGit,
		AgentGitCommit:   applianceCommit,
		AgentVersionName: applianceVersion,
	}
}

// SetAgentVersionName overrides the served appliance version. The ldflag is
// unset under go test, so tests set it explicitly instead of skipping.
func SetAgentVersionName(v string) {
	applianceVersion = v
}

// MinApplianceVersion returns the oldest appliance version the served image
// supports in-place updates from; empty means no gating.
func MinApplianceVersion() string {
	return minApplianceVersion
}

// SetMinApplianceVersion overrides the in-place update minimum for tests.
func SetMinApplianceVersion(v string) {
	minApplianceVersion = v
}

// IsValidAgentVersion reports whether ver is usable for agent version comparison and persistence.
// Empty string and "unknown" are treated as missing.
func IsValidAgentVersion(ver string) bool {
	return ver != "" && ver != "unknown"
}
