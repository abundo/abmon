package abmon

// Author: Anders Lowinger, anders@abundo.se

import "fmt"

// Set at build time via -ldflags, see Makefile. Left at these defaults for
// plain "go build"/"go run" during development.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// VersionString is shown by each check's --version flag.
func VersionString() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, BuildDate)
}
