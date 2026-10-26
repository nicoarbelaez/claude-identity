//go:build !windows

package pathenv

type noop struct{}

// New returns an Editor that does nothing: only Windows keeps cid on a
// registry-backed user PATH.
func New() Editor { return noop{} }

func (noop) RemoveDir(string) (bool, error) { return false, nil }
