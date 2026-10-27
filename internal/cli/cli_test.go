package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/cclock"
	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

// rig wires the real profile store, swap service and file-based live store on
// temp dirs and an in-memory vault. It never touches ~/.claude or the OS keyring.
type rig struct {
	t      *testing.T
	live   *claudecfg.FileStore
	prof   *profile.Store
	vault  *vault.MemoryVault
	usage  *usage.Store
	deps   Deps
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newRig(t *testing.T) *rig {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir := t.TempDir()
	paths := claudecfg.ResolvePaths(filepath.Join(dir, "home"))
	live := claudecfg.NewFileStore(paths)
	v := vault.NewMemoryVault()
	profilesPath := filepath.Join(dir, "cid", "profiles.json")
	p := profile.NewStore(profilesPath)
	svc := swap.New(p, v, live, filepath.Join(dir, "cid", "cid.lock"))
	us := usage.NewStore(usage.PathFor(profilesPath))
	r := &rig{t: t, live: live, prof: p, vault: v, usage: us, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	r.deps = Deps{
		Profiles: p,
		Swap:     svc,
		Usage:    us,
		Lang:     i18n.EN,
		GOOS:     "linux",
		Config:   Config{ProfilesPath: profilesPath, ClaudeDir: filepath.Dir(paths.Credentials)},
		Stdout:   r.stdout,
		Stderr:   r.stderr,
	}
	return r
}

func (r *rig) run(args ...string) int {
	r.t.Helper()
	r.stdout.Reset()
	r.stderr.Reset()
	return Run(r.deps, args)
}

func (r *rig) login(email, token string) {
	r.t.Helper()
	oauth, _ := json.Marshal(map[string]any{"accessToken": token, "refreshToken": "r-" + token, "expiresAt": 1000})
	acct, _ := json.Marshal(map[string]any{"emailAddress": email})
	if err := r.live.Write(credentials.Bundle{ClaudeAIOAuth: oauth, OAuthAccount: acct}); err != nil {
		r.t.Fatal(err)
	}
}

// twoProfiles saves a then b (b ends up live and active).
func (r *rig) twoProfiles() {
	r.t.Helper()
	r.login("a@example.com", "tok-a")
	if code := r.run("add", "a", "--current"); code != 0 {
		r.t.Fatalf("add a: exit %d: %s", code, r.stderr)
	}
	r.login("b@example.com", "tok-b")
	if code := r.run("add", "b", "--current"); code != 0 {
		r.t.Fatalf("add b: exit %d: %s", code, r.stderr)
	}
}

func (r *rig) msg(key i18n.Key, args ...any) string { return i18n.T(r.deps.Lang, key, args...) }

func TestListEmpty(t *testing.T) {
	for _, args := range [][]string{nil, {"list"}} {
		r := newRig(t)
		if code := r.run(args...); code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, r.stderr)
		}
		if !strings.Contains(r.stdout.String(), r.msg(i18n.ListEmpty)) {
			t.Errorf("%v: stdout = %q", args, r.stdout)
		}
	}
}

func TestListShowsIndexNameEmailAndActiveMarker(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.deps.Usage = nil // no usage cache: no usage lines and no hint
	if code := r.run(); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	want := []string{
		r.msg(i18n.ProfileListHeader),
		"  1  a  a@example.com  " + r.msg(i18n.StatusAccessExpired),
		"● 2  b  b@example.com  " + r.msg(i18n.StatusAccessExpired),
	}
	got := strings.Split(strings.TrimRight(r.stdout.String(), "\n"), "\n")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stdout lines = %q, want %q", got, want)
	}
}

func TestListJSON(t *testing.T) {
	r := newRig(t)
	if code := r.run("list", "--json"); code != 0 || strings.TrimSpace(r.stdout.String()) != "[]" {
		t.Fatalf("empty: exit %d, stdout %q", code, r.stdout)
	}
	r.twoProfiles()
	if code := r.run("list", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	var got []jsonProfile
	if err := json.Unmarshal(r.stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid json %q: %v", r.stdout, err)
	}
	want := []jsonProfile{{Index: 1, Name: "a", Email: "a@example.com", Status: "access_expired"}, {Index: 2, Name: "b", Email: "b@example.com", Active: true, Status: "access_expired"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if out := r.stdout.String(); strings.Contains(out, "tok-") || strings.Contains(strings.ToLower(out), "token") {
		t.Errorf("json leaks token data: %q", out)
	}
}

func TestAddCapturesLiveLoginAndActivates(t *testing.T) {
	r := newRig(t)
	r.login("a@example.com", "tok-a")
	if code := r.run("add", "a", "--current"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileAdded, "a")) {
		t.Errorf("stdout = %q", r.stdout)
	}
	act, err := r.prof.Active()
	if err != nil || act.Name != "a" || act.Email != "a@example.com" {
		t.Fatalf("active = %+v, %v", act, err)
	}
	if _, err := r.vault.Get("a"); err != nil {
		t.Errorf("secret not stored: %v", err)
	}
}

func TestAddErrors(t *testing.T) {
	r := newRig(t)
	if code := r.run("add"); code != 2 || !strings.Contains(r.stderr.String(), r.msg(i18n.MissingName)) || !strings.Contains(r.stderr.String(), r.msg(i18n.Usage)) {
		t.Errorf("missing name: exit %d, stderr %q", code, r.stderr)
	}
	if code := r.run("add", "--force"); code != 2 || !strings.Contains(r.stderr.String(), r.msg(i18n.MissingName)) {
		t.Errorf("only --force: exit %d, stderr %q", code, r.stderr)
	}
	if code := r.run("add", "a", "--current"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.NoLiveLogin)) {
		t.Errorf("no live login: exit %d, stderr %q", code, r.stderr)
	}
	r.login("a@example.com", "tok-a")
	if code := r.run("add", "Bad Name", "--current"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.InvalidName)) {
		t.Errorf("invalid name: exit %d, stderr %q", code, r.stderr)
	}
}

func TestAddSameAccountRefreshesSilently(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.login("a@example.com", "tok-a2")
	if code := r.run("add", "a", "--current"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileAdded, "a")) {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestAddBlocksOverwriteOfOtherAccount(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // a, b saved; b live
	r.login("c@example.com", "tok-c")
	if code := r.run("add", "a", "--current"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if want := r.msg(i18n.ProfileExists, "a", "a@example.com"); !strings.Contains(r.stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if strings.Contains(r.stderr.String()+r.stdout.String(), "tok-") {
		t.Error("output leaks token data")
	}
	if p, _ := r.prof.Resolve("a"); p.Email != "a@example.com" {
		t.Errorf("profile a email = %q", p.Email)
	}
}

func TestAddForceReplaces(t *testing.T) {
	for _, args := range [][]string{{"add", "a", "--force", "--current"}, {"add", "--force", "a", "--current"}} {
		r := newRig(t)
		r.twoProfiles()
		r.login("c@example.com", "tok-c")
		if code := r.run(args...); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, r.stderr)
		}
		if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileReplaced, "a")) {
			t.Errorf("%v: stdout = %q", args, r.stdout)
		}
		if p, _ := r.prof.Resolve("a"); p.Email != "c@example.com" {
			t.Errorf("%v: profile a email = %q", args, p.Email)
		}
	}
}

func TestAddBlocksDuplicateAccount(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // b live and saved as b
	for _, args := range [][]string{{"add", "luna", "--current"}, {"add", "luna", "--force", "--current"}} {
		if code := r.run(args...); code != 1 {
			t.Fatalf("%v: exit %d, want 1", args, code)
		}
		if want := r.msg(i18n.AccountSaved, "b"); !strings.Contains(r.stderr.String(), want) {
			t.Errorf("%v: stderr = %q, want %q", args, r.stderr, want)
		}
	}
	if ps, _ := r.prof.List(); len(ps) != 2 {
		t.Errorf("profiles = %d, want 2", len(ps))
	}
}

func TestSwitchByCommandNameAndIndex(t *testing.T) {
	for _, args := range [][]string{{"switch", "a"}, {"switch", "1"}, {"a"}, {"1"}} {
		r := newRig(t)
		r.twoProfiles()
		if code := r.run(args...); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, r.stderr)
		}
		out := r.stdout.String()
		if !strings.Contains(out, r.msg(i18n.SwitchedFromTo, "b", "a")) {
			t.Errorf("%v: missing from/to line in %q", args, out)
		}
		if !strings.Contains(out, r.msg(i18n.RemoteControlWarn)) {
			t.Errorf("%v: missing remote control warning in %q", args, out)
		}
		if act, _ := r.prof.Active(); act.Name != "a" {
			t.Errorf("%v: active = %q", args, act.Name)
		}
	}
}

func TestSwitchNoOp(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if code := r.run("switch", "b"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	out := r.stdout.String()
	if !strings.Contains(out, r.msg(i18n.AlreadyActive, "b")) || strings.Contains(out, r.msg(i18n.RemoteControlWarn)) {
		t.Errorf("stdout = %q", out)
	}
}

func TestSwitchWithoutPreviousLoginPrintsOnlyTarget(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	r.deps.Swap = fakeSwap{switchRes: swap.Result{To: profile.Profile{Name: "a"}, RemoteControlWarning: true}}
	if code := r.run("switch", "a"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.SwitchedTo, "a")) {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestSwitchMissingArgIsUsageError(t *testing.T) {
	r := newRig(t)
	if code := r.run("switch"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if code := r.run("bogus"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	errOut := r.stderr.String()
	if !strings.Contains(errOut, r.msg(i18n.UnknownCommand, "bogus")) || !strings.Contains(errOut, r.msg(i18n.Usage)) {
		t.Errorf("stderr = %q", errOut)
	}
	if r.stdout.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", r.stdout)
	}
}

func TestBadFlagsAndExtraArgsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--bogus"}, {"list", "extra"}, {"current", "--bogus"}, {"current", "x"},
		{"config", "x"}, {"version", "x"}, {"add", "a", "b"}, {"remove"}, {"switch", "a", "b"},
	} {
		r := newRig(t)
		if code := r.run(args...); code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", args, code, r.stderr)
		}
	}
}

func TestHelpPrintsUsageToStdout(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		r := newRig(t)
		if code := r.run(arg); code != 0 || !strings.Contains(r.stdout.String(), r.msg(i18n.Usage)) {
			t.Errorf("%s: exit %d, stdout %q", arg, code, r.stdout)
		}
	}
}

func TestRemove(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if code := r.run("remove", "1"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ProfileRemoved, "a")) {
		t.Errorf("stdout = %q", r.stdout)
	}
	if _, err := r.vault.Get("a"); !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("secret not deleted: %v", err)
	}
}

func TestRemoveForgetsUsageOfThatProfileOnly(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	w := map[string]usage.Window{usage.FiveHour: {PercentUsed: 5, ResetsAt: time.Now().Add(time.Hour)}}
	_ = r.usage.Record("a", w, time.Now())
	_ = r.usage.Record("b", w, time.Now())
	if code := r.run("remove", "a"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	all, _ := r.usage.All()
	if _, ok := all["a"]; ok || len(all) != 1 {
		t.Errorf("usage after remove = %+v, want only b", all)
	}
}

func TestRemoveErrors(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if code := r.run("remove", "b"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.RemoveActive, "b")) {
		t.Errorf("active: exit %d, stderr %q", code, r.stderr)
	}
	if code := r.run("remove", "nope"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.UnknownProfile, "nope")) {
		t.Errorf("unknown: exit %d, stderr %q", code, r.stderr)
	}
}

func TestRemoveActiveByIndexReportsResolvedName(t *testing.T) {
	r := newRig(t)
	r.twoProfiles()
	if code := r.run("remove", "2"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.RemoveActive, "b")) {
		t.Errorf("exit %d, stderr %q, want resolved name b", code, r.stderr)
	}
}

type activeFailsProfiles struct{}

func (activeFailsProfiles) List() ([]profile.Profile, error) {
	return []profile.Profile{{Name: "a", Index: 1}}, nil
}
func (activeFailsProfiles) Active() (profile.Profile, error) {
	return profile.Profile{}, errors.New("disk on fire")
}
func (activeFailsProfiles) Resolve(string) (profile.Profile, error) {
	return profile.Profile{}, errors.New("disk on fire")
}

func TestListSurfacesActiveLookupErrors(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"list", "--json"}} {
		r := newRig(t)
		r.deps.Profiles = activeFailsProfiles{}
		if code := r.run(args...); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.GenericError, "disk on fire")) {
			t.Errorf("%v: exit %d, stderr %q", args, code, r.stderr)
		}
	}
}

func TestCurrent(t *testing.T) {
	r := newRig(t)
	if code := r.run("current"); code != 0 || !strings.Contains(r.stdout.String(), r.msg(i18n.CurrentNone)) {
		t.Errorf("none: exit %d, stdout %q", code, r.stdout)
	}
	if code := r.run("current", "--json"); code != 0 || strings.TrimSpace(r.stdout.String()) != "null" {
		t.Errorf("none json: exit %d, stdout %q", code, r.stdout)
	}
	r.twoProfiles()
	if code := r.run("current"); code != 0 || !strings.Contains(r.stdout.String(), r.msg(i18n.CurrentActive, "b", "b@example.com")) {
		t.Errorf("active: exit %d, stdout %q", code, r.stdout)
	}
	if code := r.run("current", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	var got jsonProfile
	if err := json.Unmarshal(r.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != (jsonProfile{Index: 2, Name: "b", Email: "b@example.com", Active: true, Status: "access_expired"}) {
		t.Errorf("got %+v", got)
	}
	if out := r.run("current"); out != 0 || !strings.Contains(r.stdout.String(), r.msg(i18n.CurrentStatus, r.msg(i18n.StatusAccessExpired))) {
		t.Errorf("current status line missing: %q", r.stdout)
	}
}

func TestListStatusFollowsLiveAndVault(t *testing.T) {
	r := newRig(t)
	r.twoProfiles() // b is live and active; both stored with an expired access token
	oauth := json.RawMessage(`{"accessToken":"x","refreshToken":"","expiresAt":4102444800000}`)
	acct, _ := json.Marshal(map[string]any{"emailAddress": "b@example.com"})
	if err := r.live.Write(credentials.Bundle{ClaudeAIOAuth: oauth, OAuthAccount: acct}); err != nil {
		t.Fatal(err)
	}
	if code := r.run("list", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	var got []jsonProfile
	if err := json.Unmarshal(r.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "access_expired" || got[1].Status != "needs_login" {
		t.Errorf("statuses = %q, %q", got[0].Status, got[1].Status)
	}
	r.deps.Lang = i18n.ES
	r.run("list")
	if !strings.Contains(r.stdout.String(), i18n.T(i18n.ES, i18n.StatusNeedsLogin)) {
		t.Errorf("spanish status missing: %q", r.stdout)
	}
}

func TestConfig(t *testing.T) {
	r := newRig(t)
	r.deps.Config.ProfilesPath = "/p/profiles.json"
	r.deps.Config.ClaudeDir = "/c/.claude"
	if code := r.run("config"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	out := r.stdout.String()
	for _, want := range []string{
		r.msg(i18n.ConfigProfilesPath, "/p/profiles.json"),
		r.msg(i18n.ConfigClaudeDir, "/c/.claude"),
		r.msg(i18n.ConfigBackend, r.msg(i18n.BackendFile)),
		r.msg(i18n.ConfigLanguage, "en"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	r.deps.Config.Keychain = true
	r.run("config")
	if !strings.Contains(r.stdout.String(), r.msg(i18n.ConfigBackend, r.msg(i18n.BackendKeychain))) {
		t.Errorf("keychain backend not reported: %q", r.stdout)
	}
}

func TestVersion(t *testing.T) {
	r := newRig(t)
	if code := r.run("version"); code != 0 || !strings.HasPrefix(r.stdout.String(), "cid ") {
		t.Errorf("exit %d, stdout %q", code, r.stdout)
	}
}

func TestSpanishMessages(t *testing.T) {
	r := newRig(t)
	r.deps.Lang = i18n.ES
	if code := r.run("list"); code != 0 || !strings.Contains(r.stdout.String(), i18n.T(i18n.ES, i18n.ListEmpty)) {
		t.Errorf("exit %d, stdout %q", code, r.stdout)
	}
	if i18n.T(i18n.ES, i18n.ListEmpty) == i18n.T(i18n.EN, i18n.ListEmpty) {
		t.Error("spanish message identical to english")
	}
}

// fakeSwap returns canned results so error mapping can be tested in isolation.
type fakeSwap struct {
	switchRes swap.Result
	err       error
}

func (f fakeSwap) Switch(string) (swap.Result, error)       { return f.switchRes, f.err }
func (f fakeSwap) Add(string, bool) (swap.AddResult, error) { return swap.AddResult{}, f.err }
func (f fakeSwap) AddFrom(string, swap.Source, bool) (swap.AddResult, error) {
	return swap.AddResult{}, f.err
}
func (f fakeSwap) Relogin(string, swap.Source) (swap.ReloginResult, error) {
	return swap.ReloginResult{}, f.err
}
func (f fakeSwap) Remove(string) (profile.Profile, error) { return profile.Profile{}, f.err }
func (f fakeSwap) Status(string, bool) credentials.Status { return credentials.StatusUnknown }

func TestRuntimeErrorMapping(t *testing.T) {
	secret := "sk-ant-oat01-SECRET"
	tests := []struct {
		name string
		err  error
		want func(r *rig) string
	}{
		{"unknown live account", swap.ErrUnknownLiveAccount, func(r *rig) string { return r.msg(i18n.LiveAccountUnknown) }},
		{"lock timeout", swap.ErrLockTimeout, func(r *rig) string { return r.msg(i18n.LockTimeout) }},
		{"claude code lock", fmt.Errorf("take lock: %w", cclock.ErrTimeout), func(r *rig) string { return r.msg(i18n.ClaudeBusy) }},
		{"rollback failed", &swap.RollbackError{Err: swap.ErrVerify, Rollback: errors.New("disk")}, func(r *rig) string { return r.msg(i18n.SwitchRolledBack) }},
		{"no credentials", claudecfg.ErrNoCredentials, func(r *rig) string { return r.msg(i18n.NoLiveLogin) }},
		{"generic", errors.New("boom"), func(r *rig) string { return r.msg(i18n.GenericError, "boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.deps.Swap = fakeSwap{err: tt.err}
			if code := r.run("switch", "a"); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(r.stderr.String(), tt.want(r)) {
				t.Errorf("stderr = %q, want %q", r.stderr, tt.want(r))
			}
			if strings.Contains(r.stderr.String()+r.stdout.String(), secret) {
				t.Error("output leaks secret")
			}
		})
	}
}

func TestStoreErrorsAreReportedWithExitCode1(t *testing.T) {
	r := newRig(t)
	r.deps.Profiles = failingProfiles{}
	if code := r.run("list"); code != 1 || !strings.Contains(r.stderr.String(), r.msg(i18n.GenericError, "disk on fire")) {
		t.Errorf("exit %d, stderr %q", code, r.stderr)
	}
	if code := r.run("current"); code != 1 {
		t.Errorf("current: exit %d", code)
	}
}

type failingProfiles struct{}

func (failingProfiles) List() ([]profile.Profile, error) { return nil, errors.New("disk on fire") }
func (failingProfiles) Active() (profile.Profile, error) {
	return profile.Profile{}, errors.New("disk on fire")
}
func (failingProfiles) Resolve(string) (profile.Profile, error) {
	return profile.Profile{}, errors.New("disk on fire")
}
