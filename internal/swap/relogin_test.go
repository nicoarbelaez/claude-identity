package swap

import (
	"errors"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
)

func TestReloginOverwritesStoredCredentialsOfInactiveProfile(t *testing.T) {
	e := twoAccounts(t) // a stored, b live and active
	liveBefore, _ := e.live.Read()
	src := source(t, bundle("a@example.com", "tok-a2", 9000))

	res, err := e.svc.Relogin("a", src)
	if err != nil {
		t.Fatalf("Relogin: %v", err)
	}
	if res.LiveUpdated || res.Profile.Name != "a" {
		t.Fatalf("res = %+v", res)
	}
	if got := e.stored(t, "a"); got.ExpiresAt() != 9000 {
		t.Errorf("stored expiry = %d, want 9000", got.ExpiresAt())
	}
	liveAfter, _ := e.live.Read()
	if liveAfter.Email() != liveBefore.Email() || liveAfter.ExpiresAt() != liveBefore.ExpiresAt() {
		t.Error("live store was touched for an inactive profile")
	}
}

func TestReloginActiveProfileAlsoUpdatesLiveStore(t *testing.T) {
	e := twoAccounts(t) // b is live and active
	src := source(t, bundle("b@example.com", "tok-b2", 9000))

	res, err := e.svc.Relogin("b", src)
	if err != nil || !res.LiveUpdated {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	live, _ := e.live.Read()
	if live.Email() != "b@example.com" || live.ExpiresAt() != 9000 {
		t.Errorf("live = %q / %d, want refreshed b / 9000", live.Email(), live.ExpiresAt())
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 9000 {
		t.Errorf("stored expiry = %d, want 9000", got.ExpiresAt())
	}
}

func TestReloginRefusesDifferentAccount(t *testing.T) {
	e := twoAccounts(t)
	src := source(t, bundle("c@example.com", "tok-c", 9000))
	_, err := e.svc.Relogin("b", src)
	var mm *EmailMismatchError
	if !errors.As(err, &mm) || mm.Name != "b" || mm.Want != "b@example.com" || mm.Got != "c@example.com" {
		t.Fatalf("err = %v, want EmailMismatchError b/b@example.com/c@example.com", err)
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 2000 {
		t.Errorf("stored credentials changed: expiry %d", got.ExpiresAt())
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 2000 {
		t.Errorf("live credentials changed: expiry %d", live.ExpiresAt())
	}
}

func TestReloginResolvesByIndexAndRejectsUnknown(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.svc.Relogin("1", source(t, bundle("a@example.com", "t", 5))); err != nil {
		t.Fatalf("by index: %v", err)
	}
	if _, err := e.svc.Relogin("nope", source(t, bundle("a@example.com", "t", 5))); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReloginSourceWithoutLoginFails(t *testing.T) {
	e := twoAccounts(t)
	empty := claudecfg.NewFileStore(claudecfg.PathsIn(t.TempDir()))
	if _, err := e.svc.Relogin("a", empty); !errors.Is(err, claudecfg.ErrNoCredentials) {
		t.Fatalf("err = %v, want ErrNoCredentials", err)
	}
}
