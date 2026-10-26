package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
)

func TestReloginActiveProfileRefreshesStoredAndLive(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // b live and active
	f := loginAs(t, "b@example.com", "tok-b2")
	r.useLogin(f)

	if code := r.run("relogin", "b"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	out := r.stdout.String()
	for _, want := range []string{r.msg(i18n.ReloginDone, "b"), r.msg(i18n.ReloginLiveUpdated)} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, missing %q", out, want)
		}
	}
	if len(f.Calls) != 1 || f.Calls[0].Opts.Email != "b@example.com" {
		t.Fatalf("calls = %+v, want one call with the profile email", f.Calls)
	}
	live, err := r.live.Read()
	if err != nil || !strings.Contains(string(live.ClaudeAIOAuth), "tok-b2") {
		t.Errorf("live store not refreshed: %v", err)
	}
	raw, _ := r.vault.Get("b")
	if !strings.Contains(string(raw), "tok-b2") {
		t.Error("vault copy not refreshed")
	}
	dirGone(t, f)
}

func TestReloginInactiveProfileLeavesLiveStoreAlone(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // b live and active
	r.useLogin(loginAs(t, "a@example.com", "tok-a2"))

	if code := r.run("relogin", "1"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	out := r.stdout.String()
	if !strings.Contains(out, r.msg(i18n.ReloginDone, "a")) || strings.Contains(out, r.msg(i18n.ReloginLiveUpdated)) {
		t.Errorf("stdout = %q", out)
	}
	live, _ := r.live.Read()
	if email := live.Email(); email != "b@example.com" || strings.Contains(string(live.ClaudeAIOAuth), "tok-a2") {
		t.Errorf("live store changed (email %q)", email)
	}
	raw, _ := r.vault.Get("a")
	if !strings.Contains(string(raw), "tok-a2") {
		t.Error("vault copy of a not refreshed")
	}
}

func TestReloginRefusesDifferentAccount(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	if code := r.run("relogin", "a"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.ReloginEmailMismatch, "c@example.com", "a", "a@example.com", "a"); !strings.Contains(r.stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	raw, _ := r.vault.Get("a")
	if strings.Contains(string(raw), "tok-c") {
		t.Error("credentials of another account were saved")
	}
	dirGone(t, f)
}

func TestReloginPassesFlagsAndEmail(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	f := loginAs(t, "a@example.com", "tok-a2")
	r.useLogin(f)
	if code := r.run("relogin", "a", "--console", "--sso"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if o := f.Calls[0].Opts; !o.Console || !o.SSO || o.Email != "a@example.com" {
		t.Errorf("opts = %+v", o)
	}
}

func TestReloginUnknownProfileDoesNotStartLogin(t *testing.T) {
	r := newRig(t)
	f := loginAs(t, "a@example.com", "tok")
	r.useLogin(f)
	if code := r.run("relogin", "nope"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.UnknownProfile, "nope")) {
		t.Errorf("exit %d, stderr %q", code, r.stderr)
	}
	if len(f.Calls) != 0 {
		t.Error("login started for an unknown profile")
	}
	if code := r.run("relogin"); code != 2 {
		t.Errorf("missing arg: exit %d, want 2", code)
	}
}

func TestReloginFallbacksPrintManualSteps(t *testing.T) {
	steps := func(r *rig) string { return r.msg(i18n.ReloginSteps, "a@example.com", "a") }
	tests := []struct {
		name   string
		setup  func(r *rig) *login.FakeRunner
		reason i18n.Key
		args   []any
	}{
		{"claude missing", func(r *rig) *login.FakeRunner {
			return &login.FakeRunner{Fn: func(string, login.Options) error { return login.ErrNotInstalled }}
		}, i18n.LoginClaudeMissing, nil},
		{"login failed", func(r *rig) *login.FakeRunner {
			return &login.FakeRunner{Fn: func(string, login.Options) error { return errors.New("exit status 1") }}
		}, i18n.LoginFailed, []any{"exit status 1"}},
		{"no credentials", func(r *rig) *login.FakeRunner { return &login.FakeRunner{} }, i18n.LoginNoCredentials, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.twoProfiles()
			f := tt.setup(r)
			r.useLogin(f)
			if code := r.run("relogin", "a"); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			for _, want := range []string{r.msg(tt.reason, tt.args...), steps(r)} {
				if !strings.Contains(r.stderr.String(), want) {
					t.Errorf("stderr = %q, missing %q", r.stderr, want)
				}
			}
			dirGone(t, f)
		})
	}
}

func TestReloginDarwinFallsBackWithoutLogin(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.deps.GOOS = "darwin"
	f := loginAs(t, "a@example.com", "tok")
	r.useLogin(f)
	if code := r.run("relogin", "a"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{r.msg(i18n.LoginUnsupportedOS), r.msg(i18n.ReloginSteps, "a@example.com", "a")} {
		if !strings.Contains(r.stderr.String(), want) {
			t.Errorf("stderr = %q, missing %q", r.stderr, want)
		}
	}
	if len(f.Calls) != 0 {
		t.Error("login started on darwin")
	}
}

func TestReloginProfileWithoutEmailIsRefusedBeforeLogin(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if _, err := r.prof.Add("n"); err != nil { // no saved email
		t.Fatal(err)
	}
	f := loginAs(t, "n@example.com", "tok-n")
	r.useLogin(f)
	if code := r.run("relogin", "n"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.ReloginNoEmail, "n", "n"); !strings.Contains(r.stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if len(f.Calls) != 0 {
		t.Error("a login was started for an unverifiable profile")
	}
}
