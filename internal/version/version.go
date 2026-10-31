// Package version exposes build metadata injected at link time.
package version

// These values are overridden with -ldflags by the Makefile.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
