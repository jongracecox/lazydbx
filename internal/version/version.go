// Package version holds build metadata injected at release time via ldflags.
package version

import "fmt"

// Populated by goreleaser:
//
//	-X github.com/jongracecox/lazydbx/internal/version.Version={{.Version}}
//
// Version must stay valid semver even for dev builds — the Databricks SDK's
// useragent.WithProduct panics on anything else.
var (
	Version = "0.0.0-dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human-readable version line.
func String() string {
	return fmt.Sprintf("lazydbx %s (commit %s, built %s)", Version, Commit, Date)
}
