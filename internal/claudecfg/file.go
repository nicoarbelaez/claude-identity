package claudecfg

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

const (
	keyOAuth   = "claudeAiOauth"
	keyAccount = "oauthAccount"
)

var errNoOAuthBlock = errors.New("bundle has no OAuth credentials to write")

// FileStore keeps the OAuth block in .credentials.json (Windows, Linux, and
// macOS without a Keychain item) and the account block in .claude.json.
type FileStore struct {
	paths  Paths
	rename func(oldpath, newpath string) error // test seam; nil means os.Rename
}

// NewFileStore returns a FileStore for the given paths.
func NewFileStore(p Paths) *FileStore { return &FileStore{paths: p} }

// Read returns the live OAuth and account blocks.
func (s *FileStore) Read() (credentials.Bundle, error) {
	o, err := readObject(s.paths.Credentials)
	if err != nil {
		return credentials.Bundle{}, err
	}
	if o == nil || len(o.vals[keyOAuth]) == 0 {
		return credentials.Bundle{}, ErrNoCredentials
	}
	account, err := readAccount(s.paths.ClaudeJSON)
	if err != nil {
		return credentials.Bundle{}, err
	}
	return credentials.Bundle{
		ClaudeAIOAuth: o.vals[keyOAuth],
		OAuthAccount:  account,
		CapturedAt:    time.Now().UTC(),
	}, nil
}

// Write replaces only claudeAiOauth and oauthAccount; each file is replaced
// atomically and keeps its mode. A bundle without an account block leaves
// .claude.json untouched.
func (s *FileStore) Write(b credentials.Bundle) error {
	if len(b.ClaudeAIOAuth) == 0 {
		return errNoOAuthBlock
	}
	if err := replaceKey(s.paths.Credentials, keyOAuth, b.ClaudeAIOAuth, s.rename); err != nil {
		return err
	}
	return writeAccount(s.paths.ClaudeJSON, b.OAuthAccount, s.rename)
}

// Current returns the email of the account recorded in .claude.json.
func (s *FileStore) Current() (string, error) { return currentEmail(s.paths.ClaudeJSON) }

func readAccount(path string) (json.RawMessage, error) {
	o, err := readObject(path)
	if err != nil || o == nil {
		return nil, err
	}
	return o.vals[keyAccount], nil
}

func writeAccount(path string, account json.RawMessage, rename func(string, string) error) error {
	if len(account) == 0 {
		return nil
	}
	return replaceKey(path, keyAccount, account, rename)
}

func currentEmail(claudeJSON string) (string, error) {
	account, err := readAccount(claudeJSON)
	if err != nil {
		return "", err
	}
	if len(account) == 0 {
		return "", ErrNoCredentials
	}
	return credentials.Bundle{OAuthAccount: account}.Email(), nil
}
