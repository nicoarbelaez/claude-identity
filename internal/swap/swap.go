// Package swap implements account capture and hot-swap on top of the profile
// store, the secret vault, and the live Claude Code credential store.
package swap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

var (
	// ErrLockTimeout means another cid process held the lock too long.
	ErrLockTimeout = errors.New("another cid operation is in progress (lock timeout)")
	// ErrUnknownLiveAccount means the live login matches no saved profile, so
	// switching would discard its tokens.
	ErrUnknownLiveAccount = errors.New("the current Claude Code login is not a saved profile; run `cid add <name>` first")
	// ErrRemoveActive means the profile to remove is the active one.
	ErrRemoveActive = errors.New("cannot remove the active profile")
	// ErrVerify means the live store did not reflect the written profile.
	ErrVerify = errors.New("swap verification failed: live credentials do not match the target profile")
)

// ProfileExistsError means Add found a profile with the requested name that
// holds a different account. Email is the account that profile holds.
type ProfileExistsError struct{ Name, Email string }

func (e *ProfileExistsError) Error() string {
	return fmt.Sprintf("profile %q already holds %s; use --force to replace it", e.Name, e.Email)
}

// DuplicateAccountError means the live account is already saved as profile Name.
type DuplicateAccountError struct{ Name string }

func (e *DuplicateAccountError) Error() string {
	return fmt.Sprintf("this account is already saved as %q", e.Name)
}

// Result describes a completed Switch.
type Result struct {
	From, To profile.Profile
	// NoOp is true when the target was already live; nothing was written.
	NoOp bool
	// RemoteControlWarning is true when a swap happened: Claude Code Remote
	// Control sessions disconnect and must be reconnected.
	RemoteControlWarning bool
}

// Service coordinates profiles, vault, and the live credential store.
type Service struct {
	Profiles    *profile.Store
	Vault       vault.Vault
	Live        claudecfg.Store
	LockPath    string
	LockTimeout time.Duration

	// activeLookup overrides Profiles.Active in tests; nil means use the store.
	activeLookup func() (profile.Profile, error)
}

func (s *Service) active() (profile.Profile, error) {
	if s.activeLookup != nil {
		return s.activeLookup()
	}
	return s.Profiles.Active()
}

// New returns a Service with a 10s lock timeout.
func New(p *profile.Store, v vault.Vault, live claudecfg.Store, lockPath string) *Service {
	return &Service{Profiles: p, Vault: v, Live: live, LockPath: lockPath, LockTimeout: 10 * time.Second}
}

// WithLock runs fn while holding the cross-process cid lock. Callers that
// write profiles outside Capture/Switch (add, remove) should wrap them in it.
func (s *Service) WithLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.LockPath), 0o700); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	l := flock.New(s.LockPath)
	ctx, cancel := context.WithTimeout(context.Background(), s.LockTimeout)
	defer cancel()
	ok, err := l.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return ErrLockTimeout
	}
	defer func() { _ = l.Unlock() }()
	return fn()
}

// AddResult describes a completed Add.
type AddResult struct {
	Profile profile.Profile
	// Replaced is true when force overwrote a profile that held another account.
	Replaced bool
}

// Add captures the live login as profile name (see Capture) and marks it as
// the active profile, since the captured login is by definition the live one.
// It never discards another account's saved tokens: it refuses when name holds
// a different account (unless force) or when the live account is already saved
// under another name.
func (s *Service) Add(name string, force bool) (AddResult, error) {
	var res AddResult
	err := s.WithLock(func() error {
		var err error
		if res, err = s.addFrom(name, s.Live, force); err != nil {
			return err
		}
		return s.Profiles.SetActive(res.Profile.Name)
	})
	return res, err
}

// Source is a read-only credential source, such as the isolated config dir of
// a `claude auth login` run.
type Source interface {
	Read() (credentials.Bundle, error)
}

// AddFrom saves the login held by src as profile name under the same guards
// as Add. Unlike Add it never marks the profile active and never touches the
// live Claude Code login.
func (s *Service) AddFrom(name string, src Source, force bool) (AddResult, error) {
	var res AddResult
	err := s.WithLock(func() error {
		var err error
		res, err = s.addFrom(name, src, force)
		return err
	})
	return res, err
}

// addFrom applies the Add guards and saves src's login. Callers hold the lock.
func (s *Service) addFrom(name string, src Source, force bool) (AddResult, error) {
	b, err := src.Read()
	if err != nil {
		return AddResult{}, err
	}
	replaced, err := s.checkAdd(name, b.Email(), force)
	if err != nil {
		return AddResult{}, err
	}
	p, err := s.captureLive(name, b)
	if err != nil {
		return AddResult{}, err
	}
	return AddResult{Profile: p, Replaced: replaced}, nil
}

// checkAdd applies the Add guards for the live account email and reports
// whether the add replaces a profile that held a different account.
func (s *Service) checkAdd(name, email string, force bool) (replaced bool, err error) {
	if !profile.ValidName(name) {
		return false, profile.ErrInvalidName
	}
	if email == "" {
		return false, ErrUnknownLiveAccount
	}
	ps, err := s.Profiles.List()
	if err != nil {
		return false, err
	}
	var existing *profile.Profile
	for i, p := range ps {
		if strings.EqualFold(p.Name, name) {
			existing = &ps[i]
		} else if strings.EqualFold(p.Email, email) {
			return false, &DuplicateAccountError{Name: p.Name}
		}
	}
	if existing == nil || strings.EqualFold(existing.Email, email) {
		return false, nil
	}
	if !force {
		return false, &ProfileExistsError{Name: existing.Name, Email: existing.Email}
	}
	return true, nil
}

// EmailMismatchError means a relogin produced a different account than the
// profile holds, so nothing was saved.
type EmailMismatchError struct{ Name, Want, Got string }

func (e *EmailMismatchError) Error() string {
	return fmt.Sprintf("profile %q holds %s but the login is for %s", e.Name, e.Want, e.Got)
}

// ReloginResult describes a completed Relogin.
type ReloginResult struct {
	Profile profile.Profile
	// LiveUpdated is true when the profile was the active one and the fresh
	// credentials were also written to the live Claude Code store.
	LiveUpdated bool
}

// Relogin replaces the saved credentials of the profile identified by arg
// (index or name) with the login held by src. It refuses when src holds a
// different account than the profile. When the profile is the active one, the
// fresh credentials are also written to the live store so the running Claude
// Code session uses them.
func (s *Service) Relogin(arg string, src Source) (ReloginResult, error) {
	var res ReloginResult
	err := s.WithLock(func() error {
		p, err := s.Profiles.Resolve(arg)
		if err != nil {
			return err
		}
		b, err := src.Read()
		if err != nil {
			return err
		}
		if b.Email() == "" {
			return ErrUnknownLiveAccount
		}
		if p.Email != "" && !strings.EqualFold(p.Email, b.Email()) {
			return &EmailMismatchError{Name: p.Name, Want: p.Email, Got: b.Email()}
		}
		// Fail closed: only "no active profile" lets the relogin proceed.
		active, aerr := s.active()
		if aerr != nil && !errors.Is(aerr, profile.ErrNoActive) {
			return aerr
		}
		if res.Profile, err = s.captureLive(p.Name, b); err != nil {
			return err
		}
		if aerr == nil && active.Name == p.Name {
			if err := s.Live.Write(b); err != nil {
				return err
			}
			if err := s.verify(b); err != nil {
				return err
			}
			res.LiveUpdated = true
		}
		return nil
	})
	return res, err
}

// Remove deletes the profile identified by arg (index or name) together with
// its vault entry. It refuses to remove the active profile.
func (s *Service) Remove(arg string) (profile.Profile, error) {
	var p profile.Profile
	err := s.WithLock(func() error {
		var err error
		if p, err = s.Profiles.Resolve(arg); err != nil {
			return err
		}
		// Fail closed: only "no active profile" lets the removal proceed.
		active, aerr := s.active()
		if aerr != nil && !errors.Is(aerr, profile.ErrNoActive) {
			return aerr
		}
		if aerr == nil && active.Name == p.Name {
			return fmt.Errorf("%w: %s", ErrRemoveActive, p.Name)
		}
		if err := s.Profiles.Remove(p.Name); err != nil {
			return err
		}
		if err := s.Vault.Delete(p.Name); err != nil && !errors.Is(err, vault.ErrNotFound) {
			return err
		}
		return nil
	})
	return p, err
}

// Status reports the local token status of profile name: from the live store
// when it is the active profile, otherwise from its saved vault bundle. Any
// read failure yields StatusUnknown.
func (s *Service) Status(name string, active bool) credentials.Status {
	var b credentials.Bundle
	var err error
	if active {
		b, err = s.Live.Read()
	} else {
		b, err = s.load(name)
	}
	if err != nil {
		return credentials.StatusUnknown
	}
	return b.Status(time.Now())
}

// Capture saves the live login as profile name, creating the profile if needed.
func (s *Service) Capture(name string) (profile.Profile, error) {
	var p profile.Profile
	err := s.WithLock(func() error {
		var err error
		p, err = s.capture(name)
		return err
	})
	return p, err
}

func (s *Service) capture(name string) (profile.Profile, error) {
	live, err := s.Live.Read()
	if err != nil {
		return profile.Profile{}, err
	}
	return s.captureLive(name, live)
}

func (s *Service) captureLive(name string, live credentials.Bundle) (profile.Profile, error) {
	p, created, err := s.ensureProfile(name)
	if err != nil {
		return profile.Profile{}, err
	}
	if err := s.store(p.Name, live); err != nil {
		if created {
			err = errors.Join(err, s.Profiles.Remove(p.Name))
		}
		return profile.Profile{}, err
	}
	if err := s.Profiles.SetEmail(p.Name, live.Email()); err != nil {
		return profile.Profile{}, err
	}
	p.Email = live.Email()
	return p, nil
}

func (s *Service) ensureProfile(name string) (p profile.Profile, created bool, err error) {
	ps, err := s.Profiles.List()
	if err != nil {
		return p, false, err
	}
	for _, existing := range ps {
		if strings.EqualFold(existing.Name, name) {
			return existing, false, nil
		}
	}
	p, err = s.Profiles.Add(name)
	return p, err == nil, err
}

func (s *Service) store(name string, b credentials.Bundle) error {
	data, err := b.Marshal()
	if err != nil {
		return err
	}
	return s.Vault.Put(name, data)
}

// Switch makes the profile identified by arg (index or name) the live login.
// Before overwriting, it re-captures the live account (its tokens rotate) into
// its own profile, and refuses if the live account is not a saved profile.
func (s *Service) Switch(arg string) (Result, error) {
	var res Result
	err := s.WithLock(func() error {
		var err error
		res, err = s.switchTo(arg)
		return err
	})
	return res, err
}

func (s *Service) switchTo(arg string) (Result, error) {
	target, err := s.Profiles.Resolve(arg)
	if err != nil {
		return Result{}, err
	}
	live, err := s.Live.Read()
	hasLive := err == nil
	if err != nil && !errors.Is(err, claudecfg.ErrNoCredentials) {
		return Result{}, err
	}

	res := Result{To: target}
	if hasLive {
		owner, err := s.liveOwner(live.Email())
		if err != nil {
			return Result{}, err
		}
		if err := s.store(owner.Name, live); err != nil {
			return Result{}, err
		}
		res.From = owner
		// Already live and active: nothing to write (the vault copy was refreshed).
		if active, aerr := s.Profiles.Active(); aerr == nil && active.Name == target.Name && owner.Name == target.Name {
			res.NoOp = true
			return res, nil
		}
	}

	next, err := s.load(target.Name)
	if err != nil {
		return Result{}, err
	}
	if err := s.Live.Write(next); err != nil {
		return Result{}, err
	}
	if err := s.verify(next); err != nil {
		return Result{}, err
	}
	if err := s.Profiles.SetActive(target.Name); err != nil {
		return Result{}, err
	}
	res.RemoteControlWarning = true
	return res, nil
}

// liveOwner finds the saved profile whose email matches the live account,
// preferring the active one.
func (s *Service) liveOwner(email string) (profile.Profile, error) {
	if email == "" {
		return profile.Profile{}, ErrUnknownLiveAccount
	}
	if active, err := s.Profiles.Active(); err == nil && strings.EqualFold(active.Email, email) {
		return active, nil
	}
	ps, err := s.Profiles.List()
	if err != nil {
		return profile.Profile{}, err
	}
	for _, p := range ps {
		if strings.EqualFold(p.Email, email) {
			return p, nil
		}
	}
	return profile.Profile{}, ErrUnknownLiveAccount
}

func (s *Service) load(name string) (credentials.Bundle, error) {
	raw, err := s.Vault.Get(name)
	if err != nil {
		return credentials.Bundle{}, fmt.Errorf("load credentials for %q: %w", name, err)
	}
	return credentials.Unmarshal(raw)
}

// verify re-reads the live store and checks account and token expiry.
func (s *Service) verify(want credentials.Bundle) error {
	got, err := s.Live.Read()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	if !strings.EqualFold(got.Email(), want.Email()) || got.ExpiresAt() != want.ExpiresAt() {
		return ErrVerify
	}
	return nil
}
