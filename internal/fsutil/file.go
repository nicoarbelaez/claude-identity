package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Kind classifies a Notice.
type Kind int

// Notice kinds.
const (
	// KindReset: the file was unreadable, moved aside, and an empty state is used.
	KindReset Kind = iota + 1
	// KindRestored: the file was unreadable, moved aside, and restored from .bak.
	KindRestored
	// KindUnknownKey: config.json has a key cid does not know (Detail is the key).
	KindUnknownKey
	// KindInvalidValue: config.json has an invalid value (Detail is the key).
	KindInvalidValue
	// KindActiveCleared: profiles.json named a missing active profile (Detail is its name).
	KindActiveCleared
)

// Notice describes one recovery action the user should hear about once.
type Notice struct {
	Kind Kind
	// Path is the file the notice is about.
	Path string
	// Moved is where the broken copy was kept (KindReset, KindRestored).
	Moved string
	// Detail carries the key or profile name for the per-entry kinds.
	Detail string
}

// BackupPath returns the .bak sibling of path.
func BackupPath(path string) string { return path + ".bak" }

// WriteAtomic writes data to path through a temp file in the same directory
// (created 0600), fsync, and rename, so readers never see a partial file. The
// parent directory is created 0700.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		return errors.Join(err, os.Remove(tmpName))
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(errors.Join(fmt.Errorf("write temp file: %w", err), tmp.Close()))
	}
	if err := tmp.Sync(); err != nil {
		return fail(errors.Join(fmt.Errorf("sync temp file: %w", err), tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fail(fmt.Errorf("close temp file: %w", err))
	}
	if err := Rename(tmpName, path); err != nil {
		return fail(fmt.Errorf("replace %s: %w", filepath.Base(path), err))
	}
	syncDir(dir)
	return nil
}

// syncDir flushes the directory entry on a best-effort basis; some platforms
// (Windows) do not support syncing a directory.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// WriteAtomicBackup writes path atomically and then refreshes path.bak with
// the same bytes, so .bak always holds the last successfully written state.
func WriteAtomicBackup(path string, data []byte) error {
	if err := WriteAtomic(path, data); err != nil {
		return err
	}
	if err := WriteAtomic(BackupPath(path), data); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// LoadValidated reads path and checks it with validate.
//
//   - Missing file: (nil, nil, nil).
//   - Valid file: its bytes.
//   - Unreadable or invalid file: it is moved to <path>.corrupt-<timestamp>
//     and, if <path>.bak is valid, the backup is restored and returned
//     (KindRestored); otherwise (nil, KindReset notice) and the caller starts
//     from an empty state.
//
// Only an I/O failure that prevents recovery is returned as an error.
func LoadValidated(path string, validate func([]byte) error) ([]byte, *Notice, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if validate(data) == nil {
		return data, nil, nil
	}

	moved, err := moveAside(path)
	if err != nil {
		return nil, nil, err
	}
	n := &Notice{Kind: KindReset, Path: path, Moved: moved}
	bak, err := os.ReadFile(BackupPath(path))
	if err != nil || validate(bak) != nil {
		return nil, n, nil
	}
	if err := WriteAtomic(path, bak); err != nil {
		return nil, nil, err
	}
	n.Kind = KindRestored
	return bak, n, nil
}

// moveAside renames path to <path>.corrupt-<UTC timestamp>, adding a numeric
// suffix when that name is taken.
func moveAside(path string) (string, error) {
	base := path + ".corrupt-" + time.Now().UTC().Format("20060102T150405Z")
	dst := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(dst); errors.Is(err, os.ErrNotExist) {
			break
		}
		dst = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.Rename(path, dst); err != nil {
		return "", fmt.Errorf("move aside %s: %w", filepath.Base(path), err)
	}
	return dst, nil
}
