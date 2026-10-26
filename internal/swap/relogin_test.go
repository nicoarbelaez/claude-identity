package swap

import (
	"errors"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
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

func TestReloginRefusesProfileWithoutStoredEmail(t *testing.T) {
	e := twoAccounts(t)
	if _, err := e.prof.Add("n"); err != nil { // no email: the account cannot be verified
		t.Fatal(err)
	}
	_, err := e.svc.Relogin("n", source(t, bundle("n@example.com", "tok-n", 5)))
	var ne *NoStoredEmailError
	if !errors.As(err, &ne) || ne.Name != "n" {
		t.Fatalf("err = %v, want NoStoredEmailError for n", err)
	}
	if _, err := e.vault.Get("n"); err == nil {
		t.Error("credentials were saved for an unverifiable profile")
	}
}

func TestAddFromRefusesLoginWithoutEmail(t *testing.T) {
	e := twoAccounts(t)
	_, err := e.svc.AddFrom("c", source(t, bundle("", "tok-c", 5)), false)
	if !errors.Is(err, ErrUnknownLiveAccount) {
		t.Fatalf("err = %v, want ErrUnknownLiveAccount", err)
	}
	if _, err := e.prof.Resolve("c"); err == nil {
		t.Error("profile c was created from an account without email")
	}
}

func TestReloginRefusesAccountSavedUnderAnotherName(t *testing.T) {
	e := twoAccounts(t)
	if err := e.prof.SetEmail("a", "b@example.com"); err != nil { // a and b now share an email
		t.Fatal(err)
	}
	_, err := e.svc.Relogin("a", source(t, bundle("b@example.com", "tok-x", 9000)))
	var dup *DuplicateAccountError
	if !errors.As(err, &dup) || dup.Name != "b" {
		t.Fatalf("err = %v, want DuplicateAccountError for b", err)
	}
	if got := e.stored(t, "a"); got.ExpiresAt() != 1000 {
		t.Errorf("stored credentials changed: expiry %d", got.ExpiresAt())
	}
}

// failWrite makes every live Write fail without changing the store.
type failWrite struct{ claudecfg.Store }

func (failWrite) Write(credentials.Bundle) error { return errors.New("live write exploded") }

func TestReloginActiveLiveWriteFailureLeavesVaultUntouched(t *testing.T) {
	e := twoAccounts(t) // b is live and active
	e.svc.Live = failWrite{e.live}
	_, err := e.svc.Relogin("b", source(t, bundle("b@example.com", "tok-b2", 9000)))
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 2000 {
		t.Errorf("vault changed after a failed live write: expiry %d", got.ExpiresAt())
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 2000 {
		t.Errorf("live changed: expiry %d", live.ExpiresAt())
	}
}

// wrongFirstWrite writes a different bundle on its first Write (so verify
// fails) and passes later writes through, as a flaky backend would.
type wrongFirstWrite struct {
	claudecfg.Store
	n *int
}

func (w wrongFirstWrite) Write(b credentials.Bundle) error {
	*w.n++
	if *w.n == 1 {
		return w.Store.Write(bundle(b.Email(), "tok-wrong", 4242))
	}
	return w.Store.Write(b)
}

func TestReloginActiveVerifyFailureRestoresLiveAndSkipsVault(t *testing.T) {
	e := twoAccounts(t)
	n := 0
	e.svc.Live = wrongFirstWrite{e.live, &n}
	_, err := e.svc.Relogin("b", source(t, bundle("b@example.com", "tok-b2", 9000)))
	if !errors.Is(err, ErrVerify) {
		t.Fatalf("err = %v, want ErrVerify", err)
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 2000 {
		t.Errorf("live not restored: expiry %d", live.ExpiresAt())
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 2000 {
		t.Errorf("vault changed: expiry %d", got.ExpiresAt())
	}
}

// failPut fails every vault Put.
type failPut struct{ vault.Vault }

func (failPut) Put(string, []byte) error { return errors.New("vault exploded") }

func TestReloginActiveVaultFailureAfterLiveWriteIsSpecific(t *testing.T) {
	e := twoAccounts(t)
	e.svc.Vault = failPut{e.vault}
	_, err := e.svc.Relogin("b", source(t, bundle("b@example.com", "tok-b2", 9000)))
	var np *ProfileNotUpdatedError
	if !errors.As(err, &np) || np.Name != "b" {
		t.Fatalf("err = %v, want ProfileNotUpdatedError for b", err)
	}
	if live, _ := e.live.Read(); live.ExpiresAt() != 9000 {
		t.Errorf("live expiry = %d, want the new login (9000)", live.ExpiresAt())
	}
	if got := e.stored(t, "b"); got.ExpiresAt() != 2000 {
		t.Errorf("vault changed: expiry %d", got.ExpiresAt())
	}
}
