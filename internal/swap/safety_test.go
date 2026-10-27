package swap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/cclock"
	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

// withCCLocks gives e Claude Code lock directories under a temp dir and a short
// timeout, and returns the dirs.
func withCCLocks(t *testing.T, e *env) cclock.Dirs {
	t.Helper()
	root := t.TempDir()
	d := cclock.Dirs{
		OAuthRefresh: filepath.Join(root, "cfg", ".oauth_refresh.lock"),
		Legacy:       filepath.Join(root, "cfg.lock"),
		Config:       filepath.Join(root, "cfg.json.lock"),
	}
	e.svc.CCLocks = cclock.Set{Dirs: d, Timeout: 150 * time.Millisecond, Poll: 5 * time.Millisecond}
	return d
}

// lockSpy records whether the Claude Code locks were held during a live write.
type lockSpy struct {
	claudecfg.Store
	dirs   cclock.Dirs
	heldAt []bool
}

func (l *lockSpy) Write(b credentials.Bundle) error {
	held := true
	for _, d := range []string{l.dirs.OAuthRefresh, l.dirs.Legacy, l.dirs.Config} {
		if _, err := os.Stat(d); err != nil {
			held = false
		}
	}
	l.heldAt = append(l.heldAt, held)
	return l.Store.Write(b)
}

func TestSwitchHoldsClaudeCodeLocksDuringLiveWrite(t *testing.T) {
	e := twoAccounts(t)
	dirs := withCCLocks(t, e)
	spy := &lockSpy{Store: e.live, dirs: dirs}
	e.svc.Live = spy
	if _, err := e.svc.Switch("a"); err != nil {
		t.Fatal(err)
	}
	if len(spy.heldAt) == 0 || !spy.heldAt[0] {
		t.Fatalf("locks not held during write: %v", spy.heldAt)
	}
	for _, d := range []string{dirs.OAuthRefresh, dirs.Legacy, dirs.Config} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("lock %s not released", filepath.Base(d))
		}
	}
}

func TestSwitchTimesOutWhenClaudeCodeHoldsALockAndWritesNothing(t *testing.T) {
	e := twoAccounts(t)
	dirs := withCCLocks(t, e)
	if err := os.MkdirAll(dirs.Legacy, 0o700); err != nil { // fresh lock of a live Claude Code
		t.Fatal(err)
	}
	before := e.stored(t, "b")
	_, err := e.svc.Switch("a")
	if !errors.Is(err, cclock.ErrTimeout) {
		t.Fatalf("err = %v, want cclock.ErrTimeout", err)
	}
	if email, _ := e.live.Current(); email != "b@example.com" {
		t.Errorf("live login changed to %s", email)
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != before.ExpiresAt() {
		t.Error("vault was written")
	}
	if active, _ := e.prof.Active(); active.Name != "b" {
		t.Errorf("active = %s", active.Name)
	}
	if _, err := os.Stat(dirs.OAuthRefresh); !os.IsNotExist(err) {
		t.Error("first lock leaked after timeout")
	}
}

func TestSwitchTakesOverStaleClaudeCodeLock(t *testing.T) {
	e := twoAccounts(t)
	dirs := withCCLocks(t, e)
	e.svc.CCLocks = cclock.Set{Dirs: dirs, Timeout: 2 * time.Second, Poll: 5 * time.Millisecond}
	if err := os.MkdirAll(dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(dirs.Config, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Switch("a"); err != nil {
		t.Fatalf("stale lock blocked the switch: %v", err)
	}
	if email, _ := e.live.Current(); email != "a@example.com" {
		t.Errorf("live = %s", email)
	}
}

func TestReloginOfActiveProfileHoldsClaudeCodeLocks(t *testing.T) {
	e := twoAccounts(t)
	dirs := withCCLocks(t, e)
	if err := os.MkdirAll(dirs.OAuthRefresh, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Relogin("b", source(t, bundle("b@example.com", "tok-b2", 9000)))
	if !errors.Is(err, cclock.ErrTimeout) {
		t.Fatalf("err = %v, want cclock.ErrTimeout", err)
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 2000 {
		t.Error("live was written")
	}
}

func wipedBundle(email string) credentials.Bundle {
	b := bundle(email, "", 0)
	b.ClaudeAIOAuth = []byte(`{"accessToken":"","refreshToken":"","expiresAt":0}`)
	return b
}

func TestSwitchNeverCapturesWipedLiveCredentials(t *testing.T) {
	e := twoAccounts(t) // live and active: b
	e.login(t, wipedBundle("b@example.com"))
	if _, err := e.svc.Switch("a"); err != nil {
		t.Fatal(err)
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 2000 || got.Status(time.Unix(0, 0)) != credentials.StatusOK {
		t.Errorf("wiped login overwrote the vault copy of b: %+v", got.Status(time.Unix(0, 0)))
	}
	if email, _ := e.live.Current(); email != "a@example.com" {
		t.Errorf("live = %s, want a", email)
	}
}

func TestSwitchToActiveRepairsWipedLive(t *testing.T) {
	e := twoAccounts(t)
	e.login(t, wipedBundle("b@example.com"))
	res, err := e.svc.Switch("b")
	if err != nil {
		t.Fatal(err)
	}
	if res.NoOp {
		t.Error("wiped live must not count as already active")
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 2000 {
		t.Errorf("live not repaired: expiry %d", live.ExpiresAt())
	}
}

func TestSwitchStillRefusesForeignLiveLogin(t *testing.T) {
	e := twoAccounts(t)
	e.login(t, bundle("stranger@example.com", "tok-x", 5))
	if _, err := e.svc.Switch("a"); !errors.Is(err, ErrUnknownLiveAccount) {
		t.Fatalf("err = %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if got := e.stored(t, name); got.Email() == "stranger@example.com" {
			t.Errorf("foreign login stored in %s", name)
		}
	}
}

// corruptNthWrite writes a wrong expiry on write n (1-based) and fails every
// write after failFrom when failRestore is set.
type corruptNthWrite struct {
	claudecfg.Store
	n           *int
	failRestore bool
}

func (c corruptNthWrite) Write(b credentials.Bundle) error {
	*c.n++
	if *c.n == 1 {
		return c.Store.Write(bundle(b.Email(), "tok-wrong", 4242))
	}
	if c.failRestore {
		return errors.New("disk is read-only")
	}
	return c.Store.Write(b)
}

func TestSwitchRollsBackWhenVerifyFails(t *testing.T) {
	e := twoAccounts(t)
	n := 0
	e.svc.Live = corruptNthWrite{Store: e.live, n: &n}
	_, err := e.svc.Switch("a")
	if !errors.Is(err, ErrVerify) {
		t.Fatalf("err = %v, want ErrVerify", err)
	}
	var rb *RollbackError
	if errors.As(err, &rb) {
		t.Fatalf("rollback succeeded, error must not be a RollbackError: %v", err)
	}
	live, _ := e.live.Read()
	if live.Email() != "b@example.com" || live.ExpiresAt() != 2000 {
		t.Errorf("live not restored: %s %d", live.Email(), live.ExpiresAt())
	}
	if active, _ := e.prof.Active(); active.Name != "b" {
		t.Errorf("active = %s", active.Name)
	}
}

func TestSwitchReportsFailedRollback(t *testing.T) {
	e := twoAccounts(t)
	n := 0
	e.svc.Live = corruptNthWrite{Store: e.live, n: &n, failRestore: true}
	_, err := e.svc.Switch("a")
	var rb *RollbackError
	if !errors.As(err, &rb) {
		t.Fatalf("err = %v, want RollbackError", err)
	}
	if !errors.Is(err, ErrVerify) {
		t.Errorf("RollbackError must keep the verify error: %v", err)
	}
	if strings.Contains(err.Error(), "tok-") {
		t.Error("error leaks token values")
	}
}
