package version

import (
	"fmt"
	"runtime"
	"sync/atomic"
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
	// agentCommitFromGit is a constant representing the agent git commit SHA
	// that generated this build. It should be set during build via -ldflags.
	agentCommitFromGit string
	// agentVersionFromGit is a constant representing the agent version tag
	// that generated this build. It should be set during build via -ldflags.
	agentVersionFromGit string
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

// servedAgentVersion is the version label of the appliance image the API currently serves.
// The appliance is built from another repository, so it is the real latest agent version;
// the build-time agentVersionFromGit is only a fallback until the first image is cached.
var servedAgentVersion atomic.Pointer[string]

// SetAgentVersion records the version of the appliance image currently served.
func SetAgentVersion(v string) { servedAgentVersion.Store(&v) }

func agentVersion() string {
	if v := servedAgentVersion.Load(); v != nil && IsValidAgentVersion(*v) {
		return *v
	}
	return agentVersionFromGit
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
		AgentGitCommit:   agentCommitFromGit,
		AgentVersionName: agentVersion(),
	}
}

// IsValidAgentVersion reports whether ver is usable for agent version comparison and persistence.
// Empty string, "unknown" and the agent's unset build default "v0.0.0" are treated as missing.
func IsValidAgentVersion(ver string) bool {
	return ver != "" && ver != "unknown" && ver != "v0.0.0"
}
