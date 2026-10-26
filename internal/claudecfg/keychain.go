package claudecfg

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

const (
	securityBin     = "/usr/bin/security"
	keychainService = "Claude Code-credentials"
)

var (
	errKeychain     = errors.New("keychain access failed")
	errKeychainAcct = errors.New("cannot determine keychain account attribute")
	acctRE          = regexp.MustCompile(`"acct"<blob>="([^"]*)"`)
)

// ErrKeychainItemNotFound means the Keychain item does not exist.
var ErrKeychainItemNotFound = errors.New("keychain item not found")

// Runner executes an external command and returns its stdout.
// Implementations must not put stdout/stderr or arguments in errors.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

// ExecRunner runs real commands with a timeout.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, classify(err)
	}
	return out, nil
}

// classify maps a command failure to a payload-free sentinel. `security`
// exits 44 (errSecItemNotFound) when the item is absent; anything else
// (timeout, locked, denied) is a generic keychain failure.
func classify(err error) error {
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) && ec.ExitCode() == 44 {
		return ErrKeychainItemNotFound
	}
	return errKeychain
}

// KeychainStore keeps the OAuth block in the macOS Keychain item that Claude
// Code uses; the account block still lives in .claude.json.
//
// Note: `security add-generic-password -w` takes the secret as an argument, so
// it is briefly visible to other local processes of the same user. The CLI
// offers no non-interactive stdin alternative for -U updates.
type KeychainStore struct {
	run   Runner
	paths Paths
}

// NewKeychainStore returns a KeychainStore.
func NewKeychainStore(run Runner, p Paths) *KeychainStore {
	return &KeychainStore{run: run, paths: p}
}

// Exists reports whether the Keychain item is present. A probe that cannot
// answer (locked, denied, timeout) returns an error, never "absent".
func (k *KeychainStore) Exists() (bool, error) {
	_, err := k.run.Run(securityBin, "find-generic-password", "-s", keychainService)
	if errors.Is(err, ErrKeychainItemNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Read returns the Keychain OAuth block and the .claude.json account block.
func (k *KeychainStore) Read() (credentials.Bundle, error) {
	o, err := k.item()
	if err != nil {
		return credentials.Bundle{}, err
	}
	if len(o.vals[keyOAuth]) == 0 {
		return credentials.Bundle{}, ErrNoCredentials
	}
	account, err := readAccount(k.paths.ClaudeJSON)
	if err != nil {
		return credentials.Bundle{}, err
	}
	return credentials.Bundle{
		ClaudeAIOAuth: o.vals[keyOAuth],
		OAuthAccount:  account,
		CapturedAt:    time.Now().UTC(),
	}, nil
}

// Write replaces the OAuth key inside the existing item (reusing its account
// attribute) and then the oauthAccount key in .claude.json.
func (k *KeychainStore) Write(b credentials.Bundle) error {
	if len(b.ClaudeAIOAuth) == 0 {
		return errNoOAuthBlock
	}
	o, err := k.item()
	if err != nil {
		return err
	}
	acct, err := k.account()
	if err != nil {
		return err
	}
	o.set(keyOAuth, b.ClaudeAIOAuth)
	data, err := o.encode()
	if err != nil {
		return err
	}
	if _, err := k.run.Run(securityBin, "add-generic-password", "-U",
		"-s", keychainService, "-a", acct, "-w", string(bytes.TrimSpace(data))); err != nil {
		return errKeychain
	}
	return writeAccount(k.paths.ClaudeJSON, b.OAuthAccount, nil)
}

// Current returns the email of the account recorded in .claude.json.
func (k *KeychainStore) Current() (string, error) { return currentEmail(k.paths.ClaudeJSON) }

func (k *KeychainStore) item() (*object, error) {
	out, err := k.run.Run(securityBin, "find-generic-password", "-s", keychainService, "-w")
	if err != nil {
		return nil, ErrNoCredentials
	}
	return parseObject(bytes.TrimSpace(out))
}

func (k *KeychainStore) account() (string, error) {
	out, err := k.run.Run(securityBin, "find-generic-password", "-s", keychainService)
	if err != nil {
		return "", errKeychain
	}
	m := acctRE.FindSubmatch(out)
	if m == nil {
		return "", errKeychainAcct
	}
	return string(m[1]), nil
}
