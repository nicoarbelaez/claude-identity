package fsutil

import (
	"os"
	"time"
)

const (
	renameAttempts     = 10
	renameInitialDelay = 2 * time.Millisecond
	renameMaxDelay     = 250 * time.Millisecond
)

// Rename is os.Rename that retries the transient sharing violations Windows
// reports while antivirus or the search indexer holds a freshly written file
// (ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION). It makes at most 10 attempts
// with a doubling delay capped at 250 ms, under a second in total. On other
// platforms nothing is retried. Mirrors claude-swap's replace_with_retry.
func Rename(oldpath, newpath string) error {
	return renameWithRetry(os.Rename, isTransientRename, time.Sleep, oldpath, newpath)
}

// renameWithRetry is Rename with its collaborators injected for tests.
func renameWithRetry(rename func(string, string) error, transient func(error) bool, sleep func(time.Duration), oldpath, newpath string) error {
	delay := renameInitialDelay
	var err error
	for attempt := 0; attempt < renameAttempts; attempt++ {
		if err = rename(oldpath, newpath); err == nil || !transient(err) {
			return err
		}
		if attempt < renameAttempts-1 {
			sleep(delay)
			delay = min(delay*2, renameMaxDelay)
		}
	}
	return err
}
