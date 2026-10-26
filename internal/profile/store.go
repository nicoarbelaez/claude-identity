package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalidName is returned when a profile name fails validation.
	ErrInvalidName = errors.New("invalid profile name (use 1-32 chars of a-z 0-9 . _ -, starting with a letter or digit)")
	// ErrDuplicate is returned when a profile name already exists.
	ErrDuplicate = errors.New("profile already exists")
	// ErrNotFound is returned when no profile matches an index or name.
	ErrNotFound = errors.New("profile not found")
	// ErrNoActive is returned when no active profile has been recorded.
	ErrNoActive = errors.New("no active profile")
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// ConfigDirEnv overrides the cid config directory (used by tests and power users).
const ConfigDirEnv = "CID_CONFIG_DIR"

// DefaultPath returns the profiles.json location: $CID_CONFIG_DIR or
// os.UserConfigDir()/cid.
func DefaultPath() (string, error) {
	if dir := os.Getenv(ConfigDirEnv); dir != "" {
		return filepath.Join(dir, "profiles.json"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "cid", "profiles.json"), nil
}

// Store persists profile metadata (never secrets) as JSON.
// It is not safe for concurrent use across processes.
type Store struct {
	path string
}

// NewStore returns a Store backed by the JSON file at path.
func NewStore(path string) *Store { return &Store{path: path} }

type fileData struct {
	Profiles []Profile `json:"profiles"`
	Active   string    `json:"active,omitempty"`
}

// Add creates a profile with the next free index. Indexes are never reused.
func (s *Store) Add(name string) (Profile, error) {
	if !nameRE.MatchString(name) {
		return Profile{}, ErrInvalidName
	}
	d, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	next := 1
	for _, p := range d.Profiles {
		if strings.EqualFold(p.Name, name) {
			return Profile{}, fmt.Errorf("%w: %s", ErrDuplicate, name)
		}
		if p.Index >= next {
			next = p.Index + 1
		}
	}
	p := Profile{Name: name, Index: next, AddedAt: time.Now().UTC()}
	d.Profiles = append(d.Profiles, p)
	return p, s.save(d)
}

// Remove deletes a profile and clears the active marker if it pointed at it.
func (s *Store) Remove(name string) error {
	d, err := s.load()
	if err != nil {
		return err
	}
	i := find(d.Profiles, name)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if strings.EqualFold(d.Active, d.Profiles[i].Name) {
		d.Active = ""
	}
	d.Profiles = append(d.Profiles[:i], d.Profiles[i+1:]...)
	return s.save(d)
}

// List returns all profiles sorted by index.
func (s *Store) List() ([]Profile, error) {
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(d.Profiles, func(i, j int) bool { return d.Profiles[i].Index < d.Profiles[j].Index })
	return d.Profiles, nil
}

// Resolve finds a profile by index ("2") or case-insensitive name ("isa").
// A numeric argument is tried as an index first, then as a name.
func (s *Store) Resolve(arg string) (Profile, error) {
	d, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	if n, convErr := strconv.Atoi(arg); convErr == nil {
		for _, p := range d.Profiles {
			if p.Index == n {
				return p, nil
			}
		}
	}
	if i := find(d.Profiles, arg); i >= 0 {
		return d.Profiles[i], nil
	}
	return Profile{}, fmt.Errorf("%w: %q", ErrNotFound, arg)
}

// SetActive records the last active profile by name.
func (s *Store) SetActive(name string) error {
	d, err := s.load()
	if err != nil {
		return err
	}
	i := find(d.Profiles, name)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	d.Active = d.Profiles[i].Name
	return s.save(d)
}

// Active returns the last active profile, or ErrNoActive.
func (s *Store) Active() (Profile, error) {
	d, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	if i := find(d.Profiles, d.Active); d.Active != "" && i >= 0 {
		return d.Profiles[i], nil
	}
	return Profile{}, ErrNoActive
}

// SetEmail stores the display email for a profile.
func (s *Store) SetEmail(name, email string) error {
	d, err := s.load()
	if err != nil {
		return err
	}
	i := find(d.Profiles, name)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	d.Profiles[i].Email = email
	return s.save(d)
}

func find(ps []Profile, name string) int {
	for i, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return i
		}
	}
	return -1
}

func (s *Store) load() (fileData, error) {
	var d fileData
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, fmt.Errorf("read profiles: %w", err)
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, fmt.Errorf("parse profiles: %w", err)
	}
	return d, nil
}

// save writes atomically: temp file in the same dir, then rename.
func (s *Store) save(d fileData) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".profiles-*.tmp") // created 0600
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replace profiles: %w", err)
	}
	return nil
}
