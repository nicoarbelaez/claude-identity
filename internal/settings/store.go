package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

// Source says where a resolved value came from.
type Source string

// Value sources, in precedence order.
const (
	SourceEnv     Source = "env"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
)

// Value is a resolved setting.
type Value struct {
	Key    string
	Value  string
	Source Source
}

// Store reads and writes config.json. Reads never fail because of the file's
// content: a broken file is moved aside and a bad entry falls back to the
// default, each reported once through the notifier. It is not safe for
// concurrent use.
type Store struct {
	path   string
	getenv func(string) string
	notify func(fsutil.Notice)
	seen   map[fsutil.Notice]bool
}

// NewStore returns a Store for the file at path. getenv reads the environment
// overrides (os.Getenv in production); nil means no overrides.
func NewStore(path string, getenv func(string) string) *Store {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	return &Store{path: path, getenv: getenv, seen: map[fsutil.Notice]bool{}}
}

// WithNotifier sets the callback that receives recovery notices. It returns s.
func (s *Store) WithNotifier(fn func(fsutil.Notice)) *Store {
	s.notify = fn
	return s
}

func (s *Store) emit(n fsutil.Notice) {
	if s.notify == nil || s.seen[n] {
		return
	}
	s.seen[n] = true
	s.notify(n)
}

// Resolve returns every registered setting, in registry order.
func (s *Store) Resolve() ([]Value, error) {
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	defs := Defs()
	out := make([]Value, len(defs))
	for i, d := range defs {
		out[i] = s.resolve(d, file)
	}
	return out, nil
}

// Get resolves one setting; an unregistered key returns ErrUnknownKey.
func (s *Store) Get(key string) (Value, error) {
	d, err := Lookup(key)
	if err != nil {
		return Value{}, err
	}
	file, err := s.load()
	if err != nil {
		return Value{}, err
	}
	return s.resolve(d, file), nil
}

// Set validates and stores value for key, returning its canonical form.
func (s *Store) Set(key, value string) (string, error) {
	d, err := Lookup(key)
	if err != nil {
		return "", err
	}
	norm, err := d.Normalize(value)
	if err != nil {
		return "", err
	}
	file, err := s.load()
	if err != nil {
		return "", err
	}
	if d.Kind == Bool {
		file[key] = json.RawMessage(norm)
	} else {
		b, err := json.Marshal(norm)
		if err != nil {
			return "", err
		}
		file[key] = b
	}
	return norm, s.save(file)
}

// Unset removes key from the file so the default applies again.
func (s *Store) Unset(key string) error {
	if _, err := Lookup(key); err != nil {
		return err
	}
	file, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := file[key]; !ok {
		return nil
	}
	delete(file, key)
	return s.save(file)
}

// resolve applies env > file > default for one setting.
func (s *Store) resolve(d Def, file map[string]json.RawMessage) Value {
	if d.Env != "" {
		if v, ok := d.fromEnv(s.getenv(d.Env)); ok {
			return Value{Key: d.Key, Value: v, Source: SourceEnv}
		}
	}
	if raw, ok := file[d.Key]; ok {
		if v, ok := s.fromFile(d, raw); ok {
			return Value{Key: d.Key, Value: v, Source: SourceFile}
		}
	}
	return Value{Key: d.Key, Value: d.Default, Source: SourceDefault}
}

// fromEnv accepts the canonical value, or a locale-style one such as
// "es_CO.UTF-8" (only the part before - _ . @ counts). Anything else is
// ignored so a stray variable never breaks cid.
func (d Def) fromEnv(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if v, err := d.Normalize(raw); err == nil {
		return v, true
	}
	if d.Kind == Enum {
		if i := strings.IndexAny(raw, "-_.@"); i > 0 {
			if v, err := d.Normalize(raw[:i]); err == nil {
				return v, true
			}
		}
	}
	return "", false
}

// fromFile validates one stored entry against the embedded schema and decodes
// it. An invalid entry is reported and skipped.
func (s *Store) fromFile(d Def, raw json.RawMessage) (string, bool) {
	if err := validateEntry(d.Key, raw); err != nil {
		s.emit(fsutil.Notice{Kind: fsutil.KindInvalidValue, Path: s.path, Detail: d.Key})
		return "", false
	}
	if d.Kind == Bool {
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return "", false
		}
		return strconv.FormatBool(b), true
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}

// validateObject is the file-level check: the content must be a JSON object.
func validateObject(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m == nil {
		return errors.New("config must be a JSON object")
	}
	return nil
}

// load returns the raw entries of config.json (empty when absent). Unknown
// keys are reported and kept so that a later write does not lose them.
func (s *Store) load() (map[string]json.RawMessage, error) {
	raw, n, err := fsutil.LoadValidated(s.path, validateObject)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if n != nil {
		s.emit(*n)
	}
	entries := map[string]json.RawMessage{}
	if raw == nil {
		return entries, nil
	}
	if err := json.Unmarshal(raw, &entries); err != nil { // unreachable after validateObject
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for key := range entries {
		if _, err := Lookup(key); err != nil {
			s.emit(fsutil.Notice{Kind: fsutil.KindUnknownKey, Path: s.path, Detail: key})
		}
	}
	return entries, nil
}

func (s *Store) save(entries map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomicBackup(s.path, append(b, '\n')); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
