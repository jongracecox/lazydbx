// Package version holds build metadata injected at release time via ldflags.
package version

import "fmt"

// Populated by goreleaser:
//
//	-X github.com/jongracecox/lazydbx/internal/version.Version={{.Version}}
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human-readable version line.
func String() string {
	return fmt.Sprintf("lazydbx %s (commit %s, built %s)", Version, Commit, Date)
}
