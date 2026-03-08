package version

import (
	"fmt"
	"regexp"
	"runtime"
)

var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+`)

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
		AgentVersionName: agentVersionFromGit,
	}
}

// IsValidAgentVersion checks if the version string is valid (not empty, not "unknown", and matches semver pattern).
// Valid versions must start with 'v' followed by major.minor.patch (e.g., v0.5.1, v0.5.1-25-g45c60d3).
func IsValidAgentVersion(ver string) bool {
	if ver == "" || ver == "unknown" {
		return false
	}
	return versionPattern.MatchString(ver)
}
