// Package claudecfg reads and writes the live Claude Code identity: the OAuth
// credentials block and the oauthAccount block. It touches only those two
// keys and preserves everything else Claude Code keeps in its files.
package claudecfg

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

// ErrNoCredentials is returned when Claude Code has no stored login.
var ErrNoCredentials = errors.New("no Claude Code credentials found (run `claude` and /login first)")

// Store is the port to the live Claude Code identity.
// Implementations never include token values in errors.
type Store interface {
	// Read returns the live bundle (CapturedAt set to now).
	Read() (credentials.Bundle, error)
	// Write replaces only the OAuth and account keys with the bundle's.
	Write(credentials.Bundle) error
	// Current returns the live account email (cheap: no secrets involved).
	Current() (string, error)
}

// Paths locates Claude Code's files.
type Paths struct {
	Credentials string // <configDir>/.credentials.json
	ClaudeJSON  string // ~/.claude.json, or <configDir>/.claude.json
}

// ResolvePaths mirrors Claude Code's own lookup: CLAUDE_CONFIG_DIR wins,
// otherwise ~/.claude/.credentials.json and ~/.claude.json under home.
func ResolvePaths(home string) Paths {
	if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
		return Paths{
			Credentials: filepath.Join(cfg, ".credentials.json"),
			ClaudeJSON:  filepath.Join(cfg, ".claude.json"),
		}
	}
	return Paths{
		Credentials: filepath.Join(home, ".claude", ".credentials.json"),
		ClaudeJSON:  filepath.Join(home, ".claude.json"),
	}
}

// Open picks the store for the platform: on macOS the Keychain when its item
// exists, the credentials file when it is definitively absent, and an error
// when the probe cannot tell (no silent fallback); elsewhere the file.
func Open(goos string, run Runner, p Paths) (Store, error) {
	if goos != "darwin" {
		return NewFileStore(p), nil
	}
	kc := NewKeychainStore(run, p)
	ok, err := kc.Exists()
	if err != nil {
		return nil, err
	}
	if ok {
		return kc, nil
	}
	return NewFileStore(p), nil
}
