//go:build !windows

package fsutil

// isTransientRename is always false: POSIX rename does not fail on sharing.
func isTransientRename(error) bool { return false }
