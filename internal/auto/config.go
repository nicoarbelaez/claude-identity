package auto

import (
	_ "embed" // auto.schema.json
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
)

// Default lines, in percent of a window.
const (
	DefaultFiveHour = 90
	DefaultWeek     = 95
)

var (
	// ErrInvalidPercent means a line is not a whole number from 1 to 100.
	ErrInvalidPercent = errors.New("percentage must be from 1 to 100")
	// ErrNotEnrolled means the profile is not managed by auto-switch.
	ErrNotEnrolled = errors.New("profile is not enrolled in auto-switch")
)

// Lines are the two thresholds of an account, in percent.
type Lines struct {
	FiveHour int `json:"fiveHour"`
	Week     int `json:"week"`
}

// AccountConfig is one enrolled account. A nil line falls back to the default.
type AccountConfig struct {
	FiveHour *int `json:"fiveHour"`
	Week     *int `json:"week"`
	Role     Role `json:"role"`
}

// Config is the content of auto.json.
type Config struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Enabled       bool                     `json:"enabled"`
	Defaults      Lines                    `json:"defaults"`
	Accounts      map[string]AccountConfig `json:"accounts"`
}

// NewConfig returns the config of a user who never enrolled anything:
// disabled, default lines, no accounts.
func NewConfig() Config {
	return Config{
		SchemaVersion: 1,
		Defaults:      Lines{FiveHour: DefaultFiveHour, Week: DefaultWeek},
		Accounts:      map[string]AccountConfig{},
	}
}

// ValidPercent reports whether n is a usable line.
func ValidPercent(n int) bool { return n >= 1 && n <= 100 }

func checkPercents(ns ...*int) error {
	for _, n := range ns {
		if n != nil && !ValidPercent(*n) {
			return fmt.Errorf("%w: %d", ErrInvalidPercent, *n)
		}
	}
	return nil
}

// SetDefaults changes the default lines; nil keeps the current one.
func (c *Config) SetDefaults(five, week *int) error {
	if err := checkPercents(five, week); err != nil {
		return err
	}
	if five != nil {
		c.Defaults.FiveHour = *five
	}
	if week != nil {
		c.Defaults.Week = *week
	}
	return nil
}

// Enroll adds name, or updates its lines when it is already enrolled. A nil
// line keeps what the account had (the default for a new one); the role is kept.
func (c *Config) Enroll(name string, five, week *int) error {
	if err := checkPercents(five, week); err != nil {
		return err
	}
	if c.Accounts == nil {
		c.Accounts = map[string]AccountConfig{}
	}
	a := c.Accounts[name]
	if five != nil {
		a.FiveHour = five
	}
	if week != nil {
		a.Week = week
	}
	c.Accounts[name] = a
	return nil
}

// Remove drops name and reports whether it was enrolled.
func (c *Config) Remove(name string) bool {
	_, ok := c.Accounts[name]
	delete(c.Accounts, name)
	return ok
}

// SetRole gives name a role. There is at most one principal: naming a new one
// clears the old.
func (c *Config) SetRole(name string, role Role) error {
	a, ok := c.Accounts[name]
	if !ok {
		return ErrNotEnrolled
	}
	if role == RolePrincipal {
		for n, other := range c.Accounts {
			if other.Role == RolePrincipal {
				other.Role = RoleNone
				c.Accounts[n] = other
			}
		}
	}
	a.Role = role
	c.Accounts[name] = a
	return nil
}

// Lines returns the lines that apply to name, resolved from the defaults, and
// whether name is enrolled.
func (c Config) Lines(name string) (five, week int, ok bool) {
	a, ok := c.Accounts[name]
	if !ok {
		return 0, 0, false
	}
	five, week = c.Defaults.FiveHour, c.Defaults.Week
	if a.FiveHour != nil {
		five = *a.FiveHour
	}
	if a.Week != nil {
		week = *a.Week
	}
	return five, week, true
}

// ConfigStore persists Config in auto.json.
type ConfigStore struct{ f jsonFile }

// NewConfigStore returns a store for the auto.json at path.
func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{f: jsonFile{path: path, timeout: lockTimeout}}
}

// WithNotifier sets the callback that receives recovery notices. It returns s.
func (s *ConfigStore) WithNotifier(fn func(fsutil.Notice)) *ConfigStore {
	s.f.notify = fn
	return s
}

// Load returns the config; a missing or unreadable file yields NewConfig (an
// unreadable one is moved aside first, see fsutil.LoadValidated).
func (s *ConfigStore) Load() (Config, error) {
	raw, err := s.f.read(validateConfig)
	if err != nil {
		return NewConfig(), err
	}
	return decodeConfig(raw)
}

func decodeConfig(raw []byte) (Config, error) {
	c := NewConfig()
	if raw == nil {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil { // unreachable after validate
		return NewConfig(), fmt.Errorf("parse auto config: %w", err)
	}
	if c.Accounts == nil {
		c.Accounts = map[string]AccountConfig{}
	}
	return c, nil
}

// Update loads the config under the lock, applies fn and saves it. When fn
// returns an error nothing is written.
func (s *ConfigStore) Update(fn func(*Config) error) error {
	return s.f.locked(func() error {
		c, err := s.Load()
		if err != nil {
			return err
		}
		if err := fn(&c); err != nil {
			return err
		}
		return s.f.write(c)
	})
}

// Forget drops name from the config. It writes only when name was enrolled.
func (s *ConfigStore) Forget(name string) error {
	return s.f.locked(func() error {
		c, err := s.Load()
		if err != nil {
			return err
		}
		if !c.Remove(name) {
			return nil
		}
		return s.f.write(c)
	})
}

//go:embed auto.schema.json
var configSchemaDoc []byte

var configSchema = fsutil.MustCompileSchema("auto.schema.json", configSchemaDoc)

// validateConfig checks the schema plus what it cannot express: at most one
// principal.
func validateConfig(raw []byte) error {
	if err := configSchema.Validate(raw); err != nil {
		return err
	}
	c, err := decodeConfig(raw)
	if err != nil {
		return err
	}
	principals := 0
	for _, a := range c.Accounts {
		if a.Role == RolePrincipal {
			principals++
		}
	}
	if principals > 1 {
		return errors.New("more than one principal")
	}
	return nil
}
