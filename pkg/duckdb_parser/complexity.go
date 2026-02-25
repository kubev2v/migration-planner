package duckdb_parser

// Migration complexity levels for OS+Disk combined assessment.
const (
	ComplexityUnsupported = 0
	ComplexityEasy        = 1
	ComplexityMedium      = 2
	ComplexityHard        = 3
	ComplexityWhiteGlove  = 4
)

// OSComplexity maps OS name substrings to their migration complexity level.
// An OS is matched if its name contains the substring (case-sensitive).
// OSes not matching any entry are considered Unsupported (0).
var OSComplexity = map[string]int{
	"Red Hat":               ComplexityEasy,
	"Rocky Linux":           ComplexityEasy,
	"CentOS":                ComplexityMedium,
	"Windows":               ComplexityMedium,
	"Ubuntu":                ComplexityHard,
	"SUSE Linux Enterprise": ComplexityHard,
	"Oracle":                ComplexityWhiteGlove,
	"Microsoft SQL":         ComplexityWhiteGlove,
}
