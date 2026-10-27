//go:build windows

package fsutil

import (
	"errors"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION, which package syscall omits.
const errSharingViolation = syscall.Errno(32)

// isTransientRename reports the sharing failures Windows returns for a few
// milliseconds when another process has the file open.
func isTransientRename(err error) bool {
	var no syscall.Errno
	if !errors.As(err, &no) {
		return false
	}
	return no == syscall.ERROR_ACCESS_DENIED || no == errSharingViolation
}
