// Package settings defines cid's user-configurable options as a typed
// registry and persists them in config.json, validated against an embedded
// JSON Schema.
package settings

import (
	_ "embed" // schemaDoc
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
)

// Kind is the value type of a setting.
type Kind string

// Setting kinds.
const (
	String Kind = "string"
	Bool   Kind = "bool"
	Enum   Kind = "enum"
)

// Def describes one setting.
type Def struct {
	Key     string
	Kind    Kind
	Default string
	// Allowed lists the accepted values of an Enum.
	Allowed []string
	// Env is the environment variable that overrides the file ("" for none).
	Env string
	// Desc is the i18n key of the one-line description.
	Desc i18n.Key
}

// registry is the single source of truth for the available settings; the
// embedded schema must describe exactly these entries (see the sync test).
var registry = []Def{
	{
		Key:     "language",
		Kind:    Enum,
		Default: "auto",
		Allowed: []string{"auto", "en", "es"},
		Env:     "CID_LANG",
		Desc:    i18n.SettingLanguage,
	},
}

//go:embed config.schema.json
var schemaDoc []byte

var schema = fsutil.MustCompileSchema("config.schema.json", schemaDoc)

// Errors returned by the registry.
var (
	// ErrUnknownKey is returned for a key that is not in the registry.
	ErrUnknownKey = errors.New("unknown setting")
	// ErrInvalidValue matches every InvalidValueError.
	ErrInvalidValue = errors.New("invalid setting value")
)

// InvalidValueError reports a value a setting does not accept.
type InvalidValueError struct {
	Key     string
	Value   string
	Allowed []string
}

func (e *InvalidValueError) Error() string {
	return fmt.Sprintf("invalid value %q for %s (allowed: %s)", e.Value, e.Key, strings.Join(e.Allowed, ", "))
}

// Is lets errors.Is(err, ErrInvalidValue) match.
func (e *InvalidValueError) Is(target error) bool { return target == ErrInvalidValue }

// Defs returns a copy of the registry in declaration order.
func Defs() []Def {
	out := make([]Def, len(registry))
	for i, d := range registry {
		d.Allowed = slices.Clone(d.Allowed)
		out[i] = d
	}
	return out
}

// Lookup returns the definition of key, or ErrUnknownKey.
func Lookup(key string) (Def, error) {
	for _, d := range Defs() {
		if d.Key == key {
			return d, nil
		}
	}
	return Def{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
}

// Normalize validates v for the setting and returns its canonical form
// (enums and booleans are matched case-insensitively and lower-cased).
func (d Def) Normalize(v string) (string, error) {
	switch d.Kind {
	case Enum:
		n := strings.ToLower(strings.TrimSpace(v))
		if slices.Contains(d.Allowed, n) {
			return n, nil
		}
		return "", &InvalidValueError{Key: d.Key, Value: v, Allowed: slices.Clone(d.Allowed)}
	case Bool:
		n := strings.ToLower(strings.TrimSpace(v))
		if n == "true" || n == "false" {
			return n, nil
		}
		return "", &InvalidValueError{Key: d.Key, Value: v, Allowed: []string{"true", "false"}}
	}
	return v, nil
}

// AllowedValues returns the accepted values, or nil for free-form kinds.
func (d Def) AllowedValues() []string {
	switch d.Kind {
	case Enum:
		return slices.Clone(d.Allowed)
	case Bool:
		return []string{"true", "false"}
	}
	return nil
}

// validateEntry checks one config.json entry against the embedded schema by
// validating a single-key object, so one bad value does not discard the file.
func validateEntry(key string, raw json.RawMessage) error {
	doc, err := json.Marshal(map[string]json.RawMessage{key: raw})
	if err != nil {
		return err
	}
	return schema.Validate(doc)
}
