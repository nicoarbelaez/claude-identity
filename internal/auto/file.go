package auto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

// ErrLockTimeout means another process held auto.lock for the whole timeout.
var ErrLockTimeout = errors.New("auto-switch files are busy (lock timeout)")

// lockTimeout is short on purpose: the status line must never wait long.
const lockTimeout = 2 * time.Second

// PathFor returns the auto.json that sits next to profilesPath.
func PathFor(profilesPath string) string {
	return filepath.Join(filepath.Dir(profilesPath), "auto.json")
}

// StatePathFor returns the auto-state.json that sits next to profilesPath.
func StatePathFor(profilesPath string) string {
	return filepath.Join(filepath.Dir(profilesPath), "auto-state.json")
}

// LockFor returns the lock file that guards the auto files next to path.
func LockFor(path string) string {
	return filepath.Join(filepath.Dir(path), "auto.lock")
}

// jsonFile is the shared plumbing of both stores: validated reads that never
// lock (writes are atomic renames) and writes under auto.lock.
type jsonFile struct {
	path    string
	timeout time.Duration
	notify  func(fsutil.Notice)
}

// read returns the validated bytes of the file, or nil when it is missing or
// was reset (see fsutil.LoadValidated).
func (f *jsonFile) read(validate func([]byte) error) ([]byte, error) {
	raw, n, err := fsutil.LoadValidated(f.path, validate)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(f.path), err)
	}
	if n != nil && f.notify != nil {
		f.notify(*n)
	}
	return raw, nil
}

// locked runs fn while holding auto.lock.
func (f *jsonFile) locked(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("create auto dir: %w", err)
	}
	l := flock.New(LockFor(f.path))
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	ok, err := l.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("acquire auto lock: %w", err)
	}
	if !ok {
		return ErrLockTimeout
	}
	defer func() { _ = l.Unlock() }()
	return fn()
}

// write saves v atomically.
func (f *jsonFile) write(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomic(f.path, append(b, '\n')); err != nil {
		return fmt.Errorf("save %s: %w", filepath.Base(f.path), err)
	}
	return nil
}
