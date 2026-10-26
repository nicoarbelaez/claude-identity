package cli

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
)

// loginAs returns a fake runner whose "browser login" writes fixture
// credentials for email into the config dir it receives, like Claude Code does
// under CLAUDE_CONFIG_DIR.
func loginAs(t *testing.T, email, token string) *login.FakeRunner {
	t.Helper()
	return &login.FakeRunner{Fn: func(dir string, _ login.Options) error {
		return writeFixture(dir, email, token)
	}}
}

func writeFixture(dir, email, token string) error {
	b := fixtureBundle(email, token)
	return claudecfg.NewFileStore(claudecfg.PathsIn(dir)).Write(b)
}

func fixtureBundle(email, token string) credentials.Bundle {
	oauth, _ := json.Marshal(map[string]any{"accessToken": token, "refreshToken": "r-" + token, "expiresAt": 1000})
	acct, _ := json.Marshal(map[string]any{"emailAddress": email})
	return credentials.Bundle{ClaudeAIOAuth: oauth, OAuthAccount: acct}
}

func (r *rig) useLogin(f login.Runner) { r.deps.Login = f }

// dirGone asserts the isolated config dir of the first login call was removed.
func dirGone(t *testing.T, f *login.FakeRunner) {
	t.Helper()
	if len(f.Calls) == 0 {
		t.Fatal("login was not called")
	}
	if _, err := os.Stat(f.Calls[0].ConfigDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp config dir %s still exists (err %v)", f.Calls[0].ConfigDir, err)
	}
}

func TestAddAutoLoginSavesWithoutTouchingLiveLogin(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // a, b saved; b live and active
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)

	if code := r.run("add", "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileAdded, "c")) {
		t.Errorf("stdout = %q", r.stdout)
	}
	if p, err := r.prof.Resolve("c"); err != nil || p.Email != "c@example.com" {
		t.Fatalf("profile c = %+v, %v", p, err)
	}
	if act, _ := r.prof.Active(); act.Name != "b" {
		t.Errorf("active = %q, want b (unchanged)", act.Name)
	}
	if email, _ := r.live.Current(); email != "b@example.com" {
		t.Errorf("live login = %q, want b@example.com (unchanged)", email)
	}
	if len(f.Calls) != 1 || f.Calls[0].Opts.Email != "" {
		t.Errorf("calls = %+v", f.Calls)
	}
	dirGone(t, f)
}

func TestAddAutoLoginPassesConsoleAndSSO(t *testing.T) {
	for _, args := range [][]string{{"add", "c", "--console", "--sso"}, {"add", "--sso", "--console", "c"}} {
		r := newRig(t)
		f := loginAs(t, "c@example.com", "tok-c")
		r.useLogin(f)
		if code := r.run(args...); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, r.stderr)
		}
		if o := f.Calls[0].Opts; !o.Console || !o.SSO {
			t.Errorf("%v: opts = %+v", args, o)
		}
	}
}

func TestAddAutoLoginDifferentEmailNeedsForce(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	if code := r.run("add", "a"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.ProfileExists, "a", "a@example.com"); !strings.Contains(r.stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	dirGone(t, f)
	if code := r.run("add", "a", "--force"); code != 0 {
		t.Fatalf("forced: exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileReplaced, "a")) {
		t.Errorf("stdout = %q", r.stdout)
	}
	if p, _ := r.prof.Resolve("a"); p.Email != "c@example.com" {
		t.Errorf("profile a email = %q", p.Email)
	}
}

func TestAddAutoLoginRefusesDuplicateAccount(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	f := loginAs(t, "a@example.com", "tok-a2")
	r.useLogin(f)
	if code := r.run("add", "luna"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.AccountSaved, "a"); !strings.Contains(r.stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if ps, _ := r.prof.List(); len(ps) != 2 {
		t.Errorf("profiles = %d, want 2", len(ps))
	}
	dirGone(t, f)
}

func TestAddAutoLoginInvalidNameDoesNotStartLogin(t *testing.T) {
	r := newRig(t)
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	if code := r.run("add", "Bad Name"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.InvalidName)) {
		t.Errorf("exit %d, stderr %q", code, r.stderr)
	}
	if len(f.Calls) != 0 {
		t.Error("login started for an invalid name")
	}
}

func TestAddAutoLoginFailureFallsBackAndRemovesTempDir(t *testing.T) {
	r := newRig(t)
	f := &login.FakeRunner{Fn: func(dir string, _ login.Options) error {
		// Partial output must not survive either.
		if err := writeFixture(dir, "c@example.com", "tok-c"); err != nil {
			return err
		}
		return errors.New("exit status 1")
	}}
	r.useLogin(f)
	if code := r.run("add", "c"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{r.msg(i18n.LoginFailed), r.msg(i18n.AddFallback, "c")} {
		if !strings.Contains(r.stderr.String(), want) {
			t.Errorf("stderr = %q, missing %q", r.stderr, want)
		}
	}
	if _, err := r.prof.Resolve("c"); err == nil {
		t.Error("profile saved despite failed login")
	}
	dirGone(t, f)
}

func TestAddAutoLoginClaudeMissingFallsBack(t *testing.T) {
	r := newRig(t)
	f := &login.FakeRunner{Fn: func(string, login.Options) error { return login.ErrNotInstalled }}
	r.useLogin(f)
	if code := r.run("add", "c"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{r.msg(i18n.LoginClaudeMissing), r.msg(i18n.AddFallback, "c")} {
		if !strings.Contains(r.stderr.String(), want) {
			t.Errorf("stderr = %q, missing %q", r.stderr, want)
		}
	}
	dirGone(t, f)
}

func TestAddAutoLoginWithoutRunnerFallsBack(t *testing.T) {
	r := newRig(t)
	r.deps.Login = nil
	if code := r.run("add", "c"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.AddFallback, "c")) {
		t.Errorf("exit %d, stderr %q", code, r.stderr)
	}
}

func TestAddAutoLoginNoCredentialsFallsBack(t *testing.T) {
	r := newRig(t)
	f := &login.FakeRunner{} // exits 0 but writes nothing
	r.useLogin(f)
	if code := r.run("add", "c"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{r.msg(i18n.LoginNoCredentials), r.msg(i18n.AddFallback, "c")} {
		if !strings.Contains(r.stderr.String(), want) {
			t.Errorf("stderr = %q, missing %q", r.stderr, want)
		}
	}
	dirGone(t, f)
}

func TestAddAutoLoginDarwinFallsBackWithoutLogin(t *testing.T) {
	r := newRig(t)
	r.deps.GOOS = "darwin"
	f := loginAs(t, "c@example.com", "tok-c")
	r.useLogin(f)
	if code := r.run("add", "c"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{r.msg(i18n.LoginUnsupportedOS), r.msg(i18n.AddFallback, "c")} {
		if !strings.Contains(r.stderr.String(), want) {
			t.Errorf("stderr = %q, missing %q", r.stderr, want)
		}
	}
	if len(f.Calls) != 0 {
		t.Error("login started on darwin")
	}
}

func TestAddCurrentKeepsOldBehaviorAndNeverLogsIn(t *testing.T) {
	r := newRig(t)
	f := loginAs(t, "z@example.com", "tok-z")
	r.useLogin(f)
	r.login("a@example.com", "tok-a")
	if code := r.run("add", "a", "--current"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if act, err := r.prof.Active(); err != nil || act.Name != "a" {
		t.Errorf("active = %+v, %v; want a", act, err)
	}
	if len(f.Calls) != 0 {
		t.Error("--current started a login")
	}
}

func TestAddCurrentRejectsLoginFlags(t *testing.T) {
	for _, flag := range []string{"--console", "--sso"} {
		r := newRig(t)
		if code := r.run("add", "a", "--current", flag); code != 2 {
			t.Errorf("%s: exit %d, want 2", flag, code)
		}
	}
}
