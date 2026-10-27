// Package usage keeps the last plan-usage reading of each profile, as reported
// by Claude Code's status line. It holds percentages and reset times only:
// never tokens, and it makes no network call.
package usage

import (
	"context"
	_ "embed" // usage.schema.json
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

// Window names as they appear in Claude Code's rate_limits.
const (
	FiveHour = "five_hour"
	SevenDay = "seven_day"
)

// ErrLockTimeout means another process held usage.lock for the whole timeout.
var ErrLockTimeout = errors.New("usage cache is busy (lock timeout)")

// lockTimeout is short on purpose: recording must never slow the status line.
const lockTimeout = 2 * time.Second

// Window is one rate-limit window of a reading.
type Window struct {
	PercentUsed float64   `json:"percentUsed"`
	ResetsAt    time.Time `json:"resetsAt"`
}

// Entry is the last reading of one profile.
type Entry struct {
	FetchedAt time.Time         `json:"fetchedAt"`
	Windows   map[string]Window `json:"windows"`
}

type fileData struct {
	SchemaVersion int              `json:"schemaVersion"`
	Profiles      map[string]Entry `json:"profiles"`
}

// PathFor returns the usage.json that sits next to profilesPath.
func PathFor(profilesPath string) string {
	return filepath.Join(filepath.Dir(profilesPath), "usage.json")
}

// LockFor returns the lock file that guards the usage.json at path.
func LockFor(path string) string {
	return filepath.Join(filepath.Dir(path), "usage.lock")
}

// Store persists readings in usage.json. Reads never lock: writes are atomic
// renames. Writes take usage.lock so concurrent status lines do not lose each
// other's updates.
type Store struct {
	path    string
	timeout time.Duration
	notify  func(fsutil.Notice)
}

// NewStore returns a Store for the usage.json at path.
func NewStore(path string) *Store { return &Store{path: path, timeout: lockTimeout} }

// WithNotifier sets the callback that receives recovery notices. It returns s.
func (s *Store) WithNotifier(fn func(fsutil.Notice)) *Store {
	s.notify = fn
	return s
}

// Record replaces the reading of profile with windows taken at now.
func (s *Store) Record(profile string, windows map[string]Window, now time.Time) error {
	return s.update(func(d *fileData) bool {
		d.Profiles[profile] = Entry{FetchedAt: now.UTC(), Windows: windows}
		return true
	})
}

// Forget drops the reading of profile, if any.
func (s *Store) Forget(profile string) error {
	return s.update(func(d *fileData) bool {
		if _, ok := d.Profiles[profile]; !ok {
			return false
		}
		delete(d.Profiles, profile)
		return true
	})
}

// Get returns the reading of profile.
func (s *Store) Get(profile string) (Entry, bool, error) {
	all, err := s.All()
	if err != nil {
		return Entry{}, false, err
	}
	e, ok := all[profile]
	return e, ok, nil
}

// All returns every stored reading by profile name.
func (s *Store) All() (map[string]Entry, error) {
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	return d.Profiles, nil
}

// update loads the file under the lock, applies fn and saves when fn says so.
func (s *Store) update(fn func(*fileData) bool) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create usage dir: %w", err)
	}
	l := flock.New(LockFor(s.path))
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	ok, err := l.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("acquire usage lock: %w", err)
	}
	if !ok {
		return ErrLockTimeout
	}
	defer func() { _ = l.Unlock() }()

	d, err := s.load()
	if err != nil {
		return err
	}
	if !fn(&d) {
		return nil
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomic(s.path, append(b, '\n')); err != nil {
		return fmt.Errorf("save usage: %w", err)
	}
	return nil
}

// load reads usage.json. A file that fails validation is moved aside (see
// fsutil.LoadValidated) and an empty cache is used.
func (s *Store) load() (fileData, error) {
	d := fileData{SchemaVersion: 1, Profiles: map[string]Entry{}}
	raw, n, err := fsutil.LoadValidated(s.path, validate)
	if err != nil {
		return d, fmt.Errorf("read usage: %w", err)
	}
	if n != nil && s.notify != nil {
		s.notify(*n)
	}
	if raw == nil {
		return d, nil
	}
	if err := json.Unmarshal(raw, &d); err != nil { // unreachable after validate
		return fileData{}, fmt.Errorf("parse usage: %w", err)
	}
	if d.Profiles == nil {
		d.Profiles = map[string]Entry{}
	}
	return d, nil
}

//go:embed usage.schema.json
var schemaDoc []byte

var schema = fsutil.MustCompileSchema("usage.schema.json", schemaDoc)

// validate checks the schema plus what it cannot express: the timestamps must
// parse as times.
func validate(raw []byte) error {
	if err := schema.Validate(raw); err != nil {
		return err
	}
	var d fileData
	return json.Unmarshal(raw, &d)
}
