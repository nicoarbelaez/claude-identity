// Package version exposes the build version of cid.
package version

// Version is overridden at build time via -ldflags "-X .../internal/version.Version=...".
var Version = "dev"

// String returns the current version string.
func String() string { return Version }
