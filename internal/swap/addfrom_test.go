package swap

import (
	"errors"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
)

// source returns a file store in a fresh temp dir holding b, standing in for
// the isolated config dir of a `claude auth login` run.
func source(t *testing.T, b credentials.Bundle) *claudecfg.FileStore {
	t.Helper()
	s := claudecfg.NewFileStore(claudecfg.PathsIn(t.TempDir()))
	if err := s.Write(b); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAddFromSavesWithoutTouchingLiveOrActive(t *testing.T) {
	e := twoAccounts(t) // live and active: b
	liveBefore, _ := e.live.Read()
	src := source(t, bundle("c@example.com", "tok-c", 3000))

	res, err := e.svc.AddFrom("c", src, false)
	if err != nil {
		t.Fatalf("AddFrom: %v", err)
	}
	if res.Replaced || res.Profile.Name != "c" || res.Profile.Email != "c@example.com" {
		t.Fatalf("res = %+v", res)
	}
	if got := e.stored(t, "c"); got.Email() != "c@example.com" {
		t.Errorf("stored email = %q", got.Email())
	}
	if act, err := e.prof.Active(); err != nil || act.Name != "b" {
		t.Errorf("active = %+v, %v; want b", act, err)
	}
	liveAfter, _ := e.live.Read()
	if liveAfter.Email() != liveBefore.Email() || liveAfter.ExpiresAt() != liveBefore.ExpiresAt() {
		t.Errorf("live login changed: %q -> %q", liveBefore.Email(), liveAfter.Email())
	}
}

func TestAddFromRefreshesSameNameSameEmail(t *testing.T) {
	e := twoAccounts(t)
	src := source(t, bundle("a@example.com", "tok-a2", 9000))
	res, err := e.svc.AddFrom("a", src, false)
	if err != nil || res.Replaced {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if got := e.stored(t, "a"); got.ExpiresAt() != 9000 {
		t.Errorf("stored expiry = %d, want refreshed 9000", got.ExpiresAt())
	}
}

func TestAddFromDifferentEmailNeedsForce(t *testing.T) {
	e := twoAccounts(t)
	src := source(t, bundle("c@example.com", "tok-c", 3000))
	_, err := e.svc.AddFrom("a", src, false)
	var exists *ProfileExistsError
	if !errors.As(err, &exists) || exists.Email != "a@example.com" {
		t.Fatalf("err = %v, want ProfileExistsError for a@example.com", err)
	}
	if got := e.stored(t, "a"); got.Email() != "a@example.com" {
		t.Errorf("profile a was overwritten: %q", got.Email())
	}
	res, err := e.svc.AddFrom("a", src, true)
	if err != nil || !res.Replaced {
		t.Fatalf("forced: res = %+v, err = %v", res, err)
	}
	if got := e.stored(t, "a"); got.Email() != "c@example.com" {
		t.Errorf("forced stored email = %q", got.Email())
	}
}

func TestAddFromRefusesAccountSavedUnderAnotherName(t *testing.T) {
	e := twoAccounts(t)
	src := source(t, bundle("a@example.com", "tok-a2", 9000))
	_, err := e.svc.AddFrom("z", src, false)
	var dup *DuplicateAccountError
	if !errors.As(err, &dup) || dup.Name != "a" {
		t.Fatalf("err = %v, want DuplicateAccountError for a", err)
	}
	if _, err := e.prof.Resolve("z"); !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("profile z was created: %v", err)
	}
}

func TestAddFromSourceWithoutLoginFails(t *testing.T) {
	e := newEnv(t)
	empty := claudecfg.NewFileStore(claudecfg.PathsIn(t.TempDir()))
	if _, err := e.svc.AddFrom("a", empty, false); !errors.Is(err, claudecfg.ErrNoCredentials) {
		t.Fatalf("err = %v, want ErrNoCredentials", err)
	}
}
