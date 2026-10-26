//go:build !windows

package pathenv

// New returns nil: only Windows keeps cid on a registry-backed user PATH, so
// there is nothing to edit elsewhere and callers skip the PATH step.
func New() Editor { return nil }
