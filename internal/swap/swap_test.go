package swap

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

type env struct {
	svc   *Service
	live  *claudecfg.FileStore
	vault *vault.MemoryVault
	prof  *profile.Store
	lock  string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir := t.TempDir()
	live := claudecfg.NewFileStore(claudecfg.ResolvePaths(filepath.Join(dir, "home")))
	v := vault.NewMemoryVault()
	p := profile.NewStore(filepath.Join(dir, "cid", "profiles.json"))
	lock := filepath.Join(dir, "cid", "cid.lock")
	svc := New(p, v, live, lock)
	svc.LockTimeout = 300 * time.Millisecond
	return &env{svc: svc, live: live, vault: v, prof: p, lock: lock}
}

func bundle(email, token string, expires int64) credentials.Bundle {
	oauth, _ := json.Marshal(map[string]any{"accessToken": token, "refreshToken": "r-" + token, "expiresAt": expires})
	acct, _ := json.Marshal(map[string]any{"emailAddress": email})
	return credentials.Bundle{ClaudeAIOAuth: oauth, OAuthAccount: acct}
}

func (e *env) login(t *testing.T, b credentials.Bundle) {
	t.Helper()
	if err := e.live.Write(b); err != nil {
		t.Fatal(err)
	}
}

func (e *env) stored(t *testing.T, name string) credentials.Bundle {
	t.Helper()
	raw, err := e.vault.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := credentials.Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// twoAccounts captures a (live afterwards: b) and b, with a as active.
func twoAccounts(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.login(t, bundle("a@example.com", "tok-a1", 1000))
	if _, err := e.svc.Capture("a"); err != nil {
		t.Fatal(err)
	}
	e.login(t, bundle("b@example.com", "tok-b1", 2000))
	if _, err := e.svc.Capture("b"); err != nil {
		t.Fatal(err)
	}
	if err := e.prof.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCaptureCreatesProfileAndStoresBundle(t *testing.T) {
	e := newEnv(t)
	e.login(t, bundle("a@example.com", "tok-a1", 1000))
	p, err := e.svc.Capture("a")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "a" || p.Index != 1 {
		t.Fatalf("profile = %+v", p)
	}
	got, err := e.prof.Resolve("a")
	if err != nil || got.Email != "a@example.com" {
		t.Fatalf("email not recorded: %+v, %v", got, err)
	}
	if e.stored(t, "a").ExpiresAt() != 1000 {
		t.Error("vault bundle mismatch")
	}
}

func TestCaptureExistingProfileRefreshesBundle(t *testing.T) {
	e := newEnv(t)
	e.login(t, bundle("a@example.com", "tok-a1", 1000))
	if _, err := e.svc.Capture("a"); err != nil {
		t.Fatal(err)
	}
	e.login(t, bundle("a@example.com", "tok-a2", 3000))
	if _, err := e.svc.Capture("a"); err != nil {
		t.Fatal(err)
	}
	ps, _ := e.prof.List()
	if len(ps) != 1 || e.stored(t, "a").ExpiresAt() != 3000 {
		t.Fatalf("profiles=%d expires=%d", len(ps), e.stored(t, "a").ExpiresAt())
	}
}

func TestCaptureInvalidNameStoresNothing(t *testing.T) {
	e := newEnv(t)
	e.login(t, bundle("a@example.com", "tok-a1", 1000))
	if _, err := e.svc.Capture("Bad Name"); !errors.Is(err, profile.ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
	if _, err := e.vault.Get("Bad Name"); !errors.Is(err, vault.ErrNotFound) {
		t.Error("secret stored for invalid profile")
	}
}

func TestSwitchByNameAndIndex(t *testing.T) {
	for _, arg := range []string{"a", "1"} {
		t.Run(arg, func(t *testing.T) {
			e := twoAccounts(t)
			res, err := e.svc.Switch(arg)
			if err != nil {
				t.Fatal(err)
			}
			if res.To.Name != "a" || res.From.Name != "b" || res.NoOp || !res.RemoteControlWarning {
				t.Fatalf("result = %+v", res)
			}
			if email, _ := e.live.Current(); email != "a@example.com" {
				t.Errorf("live email = %q", email)
			}
			if act, _ := e.prof.Active(); act.Name != "a" {
				t.Errorf("active = %q", act.Name)
			}
		})
	}
}

func TestSwitchRecapturesRotatedTokenOfActive(t *testing.T) {
	e := twoAccounts(t)
	// Claude Code refreshed b's token after it was captured.
	e.login(t, bundle("b@example.com", "tok-b2", 9999))
	if _, err := e.svc.Switch("a"); err != nil {
		t.Fatal(err)
	}
	if got := e.stored(t, "b").ExpiresAt(); got != 9999 {
		t.Fatalf("vault kept stale bundle for b (expiresAt=%d)", got)
	}
}

func TestSwitchSameTargetIsNoOp(t *testing.T) {
	e := twoAccounts(t)
	res, err := e.svc.Switch("b")
	if err != nil {
		t.Fatal(err)
	}
	if !res.NoOp || res.RemoteControlWarning || res.To.Name != "b" {
		t.Fatalf("result = %+v", res)
	}
}

func TestSwitchRefusesUnknownLiveAccount(t *testing.T) {
	e := twoAccounts(t)
	e.login(t, bundle("stranger@example.com", "tok-x", 5))
	if _, err := e.svc.Switch("a"); !errors.Is(err, ErrUnknownLiveAccount) {
		t.Fatalf("want ErrUnknownLiveAccount, got %v", err)
	}
	if email, _ := e.live.Current(); email != "stranger@example.com" {
		t.Error("live login was overwritten")
	}
}

func TestSwitchWithoutLiveLoginSkipsRecapture(t *testing.T) {
	e := twoAccounts(t)
	e.svc.Live = loggedOut{Store: e.live, wrote: new(bool)}
	res, err := e.svc.Switch("a")
	if err != nil {
		t.Fatal(err)
	}
	if res.From.Name != "" {
		t.Errorf("From = %q, want none", res.From.Name)
	}
}

func TestSwitchUnknownProfile(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.svc.Switch("nope"); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSwitchDetectsFailedVerification(t *testing.T) {
	e := twoAccounts(t)
	e.svc.Live = dropWrites{e.live}
	if _, err := e.svc.Switch("a"); !errors.Is(err, ErrVerify) {
		t.Fatalf("want ErrVerify, got %v", err)
	}
	if active, err := e.prof.Active(); err != nil || active.Name != "b" {
		t.Fatalf("active profile changed despite failed verify: %+v, %v", active, err)
	}
}

func TestLockContentionTimesOut(t *testing.T) {
	e := twoAccounts(t)
	other := flock.New(e.lock)
	if err := other.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Unlock() }()
	if _, err := e.svc.Switch("a"); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("want ErrLockTimeout, got %v", err)
	}
	if _, err := e.svc.Capture("c"); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Capture: want ErrLockTimeout, got %v", err)
	}
}

func TestWithLockReleasesLock(t *testing.T) {
	e := twoAccounts(t)
	if err := e.svc.WithLock(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.WithLock(func() error { return nil }); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

// loggedOut pretends nobody is logged in until the first Write.
type loggedOut struct {
	claudecfg.Store
	wrote *bool
}

func (l loggedOut) Read() (credentials.Bundle, error) {
	if !*l.wrote {
		return credentials.Bundle{}, claudecfg.ErrNoCredentials
	}
	return l.Store.Read()
}

func (l loggedOut) Write(b credentials.Bundle) error {
	*l.wrote = true
	return l.Store.Write(b)
}

// dropWrites silently ignores Write, as a broken backend would.
type dropWrites struct{ claudecfg.Store }

func (dropWrites) Write(credentials.Bundle) error { return nil }

func TestAddCapturesLiveLoginAndMarksItActive(t *testing.T) {
	e := twoAccounts(t) // b is live and active
	e.login(t, bundle("c@example.com", "tok-c1", 3000))
	p, err := e.svc.Add("c")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "c" || p.Email != "c@example.com" {
		t.Fatalf("profile = %+v", p)
	}
	if act, err := e.prof.Active(); err != nil || act.Name != "c" {
		t.Fatalf("active = %+v, %v", act, err)
	}
	if e.stored(t, "c").ExpiresAt() != 3000 {
		t.Error("vault bundle mismatch")
	}
}

func TestAddInvalidNameDoesNotChangeActive(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.svc.Add("Bad Name"); !errors.Is(err, profile.ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
	if act, _ := e.prof.Active(); act.Name != "b" {
		t.Fatalf("active = %q, want b", act.Name)
	}
}

func TestAddWithoutLiveLogin(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Add("a"); !errors.Is(err, claudecfg.ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestRemoveDeletesProfileAndSecret(t *testing.T) {
	for _, arg := range []string{"a", "1"} {
		t.Run(arg, func(t *testing.T) {
			e := twoAccounts(t) // b active
			p, err := e.svc.Remove(arg)
			if err != nil {
				t.Fatal(err)
			}
			if p.Name != "a" {
				t.Fatalf("removed = %+v", p)
			}
			if _, err := e.prof.Resolve("a"); !errors.Is(err, profile.ErrNotFound) {
				t.Errorf("profile still present: %v", err)
			}
			if _, err := e.vault.Get("a"); !errors.Is(err, vault.ErrNotFound) {
				t.Errorf("secret still present: %v", err)
			}
		})
	}
}

func TestRemoveRefusesActiveProfile(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.svc.Remove("b"); !errors.Is(err, ErrRemoveActive) {
		t.Fatalf("want ErrRemoveActive, got %v", err)
	}
	if _, err := e.prof.Resolve("b"); err != nil {
		t.Errorf("active profile was removed: %v", err)
	}
	if _, err := e.vault.Get("b"); err != nil {
		t.Errorf("active secret was removed: %v", err)
	}
}

func TestRemoveUnknownProfileAndMissingSecret(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.svc.Remove("nope"); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// A profile whose vault entry is already gone is still removable.
	if err := e.vault.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Remove("a"); err != nil {
		t.Fatalf("remove with missing secret: %v", err)
	}
}

func TestAddAndRemoveHonorLock(t *testing.T) {
	e := twoAccounts(t)
	other := flock.New(e.lock)
	if err := other.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Unlock() }()
	if _, err := e.svc.Add("c"); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Add: want ErrLockTimeout, got %v", err)
	}
	if _, err := e.svc.Remove("a"); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Remove: want ErrLockTimeout, got %v", err)
	}
}
