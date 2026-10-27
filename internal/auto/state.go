package auto

import (
	_ "embed" // auto-state.schema.json
	"encoding/json"
	"fmt"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

// Pending is a switch the status line decided and the next prompt will apply.
type Pending struct {
	From   string    `json:"from"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
	Window string    `json:"window"`
	Pct    float64   `json:"pct"`
	At     time.Time `json:"at"`
}

// Notice is the line the status line shows after a switch, until Until.
type Notice struct {
	Text  string    `json:"text"`
	Until time.Time `json:"until"`
}

// MaxHistory is how many switch records the state keeps.
const MaxHistory = 20

// HistoryEntry is one automatic switch that was applied or that failed.
type HistoryEntry struct {
	At     time.Time `json:"at"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Reason string    `json:"reason,omitempty"`
	Window string    `json:"window,omitempty"`
	Pct    float64   `json:"pct,omitempty"`
	// Error is set when the switch failed; empty means it was applied.
	Error string `json:"error,omitempty"`
}

// AddHistory appends e and keeps only the last MaxHistory entries.
func (s *State) AddHistory(e HistoryEntry) {
	s.History = append(s.History, e)
	if n := len(s.History); n > MaxHistory {
		s.History = append([]HistoryEntry(nil), s.History[n-MaxHistory:]...)
	}
}

// State is the content of auto-state.json: what was decided, not what the user
// configured.
type State struct {
	SchemaVersion int      `json:"schemaVersion"`
	Pending       *Pending `json:"pending"`
	// LastSwitchAt is when cid last switched automatically (cooldown clock).
	LastSwitchAt time.Time `json:"lastSwitchAt,omitzero"`
	Notice       *Notice   `json:"notice"`
	// LastNoCandidateAt is when the user was last told that nobody can take over.
	LastNoCandidateAt time.Time `json:"lastNoCandidateAt,omitzero"`
	// History is a small log of applied and failed switches, oldest first.
	History []HistoryEntry `json:"history,omitempty"`
}

// StateStore persists State in auto-state.json.
type StateStore struct{ f jsonFile }

// NewStateStore returns a store for the auto-state.json at path.
func NewStateStore(path string) *StateStore {
	return &StateStore{f: jsonFile{path: path, timeout: lockTimeout}}
}

// WithNotifier sets the callback that receives recovery notices. It returns s.
func (s *StateStore) WithNotifier(fn func(fsutil.Notice)) *StateStore {
	s.f.notify = fn
	return s
}

// Load returns the state; a missing or unreadable file yields an empty one.
func (s *StateStore) Load() (State, error) {
	raw, err := s.f.read(validateState)
	if err != nil {
		return State{SchemaVersion: 1}, err
	}
	return decodeState(raw)
}

func decodeState(raw []byte) (State, error) {
	st := State{SchemaVersion: 1}
	if raw == nil {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil { // unreachable after validate
		return State{SchemaVersion: 1}, fmt.Errorf("parse auto state: %w", err)
	}
	return st, nil
}

// Update loads the state under the lock, applies fn and saves it.
func (s *StateStore) Update(fn func(*State)) error {
	return s.f.locked(func() error {
		st, err := s.Load()
		if err != nil {
			return err
		}
		fn(&st)
		return s.f.write(st)
	})
}

// Forget drops the pending switch when it involves profile. It writes only
// when there is something to drop.
func (s *StateStore) Forget(profile string) error {
	return s.f.locked(func() error {
		st, err := s.Load()
		if err != nil {
			return err
		}
		if st.Pending == nil || (st.Pending.From != profile && st.Pending.To != profile) {
			return nil
		}
		st.Pending = nil
		return s.f.write(st)
	})
}

//go:embed auto-state.schema.json
var stateSchemaDoc []byte

var stateSchema = fsutil.MustCompileSchema("auto-state.schema.json", stateSchemaDoc)

// validateState checks the schema plus what it cannot express: the timestamps
// must parse as times.
func validateState(raw []byte) error {
	if err := stateSchema.Validate(raw); err != nil {
		return err
	}
	_, err := decodeState(raw)
	return err
}
