//go:build windows

package fsutil

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestTransientRenameErrorsOnWindows(t *testing.T) {
	for _, no := range []syscall.Errno{syscall.ERROR_ACCESS_DENIED, errSharingViolation} {
		err := fmt.Errorf("wrapped: %w", &os.LinkError{Op: "rename", Err: no})
		if !isTransientRename(err) {
			t.Errorf("errno %d must be transient", no)
		}
	}
	if isTransientRename(&os.LinkError{Op: "rename", Err: syscall.ERROR_FILE_NOT_FOUND}) {
		t.Error("file-not-found must not be retried")
	}
}
