package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

type fakePathEnv struct {
	dirs    []string
	removed bool
	err     error
}

func (f *fakePathEnv) RemoveDir(dir string) (bool, error) {
	f.dirs = append(f.dirs, dir)
	return f.removed, f.err
}

// failingVault fails Delete for the named profiles and delegates the rest.
type failingVault struct {
	*vault.MemoryVault
	fail map[string]bool
}

func (v failingVault) Delete(p string) error {
	if v.fail[p] {
		return errors.New("keyring locked")
	}
	return v.MemoryVault.Delete(p)
}

// uninstallRig prepares two saved profiles and the uninstall dependencies.
func uninstallRig(t *testing.T) (*rig, *fakePathEnv, string) {
	t.Helper()
	r := newRig(t)
	r.twoProfiles()
	pe := &fakePathEnv{removed: true}
	exe := filepath.Join(t.TempDir(), "bin", "cid.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	r.deps.Uninstall = UninstallDeps{
		Vault:      r.vault,
		PathEnv:    pe,
		LockPath:   filepath.Join(filepath.Dir(r.deps.Config.ProfilesPath), "cid.lock"),
		Executable: exe,
		GOOS:       "windows",
	}
	return r, pe, exe
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestUninstallYesRemovesEverything(t *testing.T) {
	r, pe, exe := uninstallRig(t)
	cfgDir := filepath.Dir(r.deps.Config.ProfilesPath)
	if !exists(r.deps.Uninstall.LockPath) {
		t.Fatal("precondition: lock file should exist after add")
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	for _, name := range []string{"a", "b"} { // b is the active profile
		if _, err := r.vault.Get(name); !errors.Is(err, vault.ErrNotFound) {
			t.Errorf("vault entry %q still present (err %v)", name, err)
		}
	}
	if exists(r.deps.Config.ProfilesPath) || exists(r.deps.Uninstall.LockPath) || exists(cfgDir) {
		t.Errorf("config files or dir left behind in %s", cfgDir)
	}
	if len(pe.dirs) != 1 || pe.dirs[0] != filepath.Dir(exe) {
		t.Errorf("PATH editor got %v, want [%s]", pe.dirs, filepath.Dir(exe))
	}
	out := r.stdout.String()
	for _, want := range []string{exe, "/plugin uninstall account-switch@nicoarbelaez-tools", "/plugin marketplace remove nicoarbelaez-tools", "Remove-Item"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestUninstallKeepsSharedBinDirOnPath(t *testing.T) {
	r, pe, exe := uninstallRig(t)
	// A directory shared with other tools (for example ~/go/bin after go install).
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), "gopls.exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if len(pe.dirs) != 0 {
		t.Errorf("PATH editor called with %v for a shared directory", pe.dirs)
	}
}

func TestUninstallLeavesLiveCredentials(t *testing.T) {
	r, _, _ := uninstallRig(t)
	before, err := r.live.Read()
	if err != nil {
		t.Fatal(err)
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	after, err := r.live.Read()
	if err != nil {
		t.Fatalf("live credentials gone: %v", err)
	}
	if string(after.ClaudeAIOAuth) != string(before.ClaudeAIOAuth) || string(after.OAuthAccount) != string(before.OAuthAccount) {
		t.Error("live credentials changed")
	}
}

func TestUninstallConfirmation(t *testing.T) {
	tests := []struct {
		input   string
		removed bool
	}{
		{"y\n", true}, {"yes\n", true}, {"YES\n", true}, {"s\n", true}, {"si\n", true}, {"sí\n", true},
		{"n\n", false}, {"\n", false}, {"", false}, {"maybe\n", false},
	}
	for _, tc := range tests {
		t.Run(strings.TrimSpace(tc.input), func(t *testing.T) {
			r, _, _ := uninstallRig(t)
			r.deps.Stdin = strings.NewReader(tc.input)
			if code := r.run("uninstall"); code != 0 {
				t.Fatalf("exit %d: %s", code, r.stderr)
			}
			_, err := r.vault.Get("a")
			gone := errors.Is(err, vault.ErrNotFound)
			if gone != tc.removed {
				t.Fatalf("removed=%v, want %v", gone, tc.removed)
			}
			if exists(r.deps.Config.ProfilesPath) == tc.removed {
				t.Errorf("profiles file existence wrong for removed=%v", tc.removed)
			}
			if !tc.removed && !strings.Contains(r.stdout.String(), i18n.T(i18n.EN, i18n.UninstallAborted)) {
				t.Errorf("missing abort message: %s", r.stdout)
			}
		})
	}
}

func TestUninstallPromptListsWhatWillBeRemoved(t *testing.T) {
	r, _, _ := uninstallRig(t)
	r.deps.Stdin = strings.NewReader("n\n")
	r.run("uninstall")
	out := r.stdout.String()
	if !strings.Contains(out, filepath.Dir(r.deps.Config.ProfilesPath)) || !strings.Contains(out, "2 saved") {
		t.Errorf("plan should name the config folder and profile count:\n%s", out)
	}
}

func TestUninstallPartialFailureKeepsGoing(t *testing.T) {
	r, pe, _ := uninstallRig(t)
	r.deps.Uninstall.Vault = failingVault{MemoryVault: r.vault, fail: map[string]bool{"a": true}}
	pe.err = errors.New("registry denied")
	pe.removed = false
	if code := r.run("uninstall", "--yes"); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if _, err := r.vault.Get("b"); !errors.Is(err, vault.ErrNotFound) {
		t.Error("profile b should still be deleted after a failed")
	}
	if exists(r.deps.Config.ProfilesPath) {
		t.Error("profiles file should still be removed")
	}
	errOut := r.stderr.String()
	if !strings.Contains(errOut, "keyring locked") || !strings.Contains(errOut, "registry denied") {
		t.Errorf("failures not reported:\n%s", errOut)
	}
}

func TestUninstallMissingVaultEntryIsNotAFailure(t *testing.T) {
	r, _, _ := uninstallRig(t)
	if err := r.vault.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
}

func TestUninstallKeepsNonEmptyConfigDir(t *testing.T) {
	r, _, _ := uninstallRig(t)
	other := filepath.Join(filepath.Dir(r.deps.Config.ProfilesPath), "keep.txt")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !exists(other) {
		t.Error("unrelated file in config dir was removed")
	}
}

func TestUninstallUnixHints(t *testing.T) {
	r, pe, exe := uninstallRig(t)
	r.deps.Uninstall.GOOS = "linux"
	r.deps.Uninstall.PathEnv = nil
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if len(pe.dirs) != 0 {
		t.Error("PATH editor must not run without an editor dep")
	}
	if out := r.stdout.String(); !strings.Contains(out, "rm ") || !strings.Contains(out, exe) {
		t.Errorf("missing rm hint:\n%s", out)
	}
}

func TestUninstallRejectsExtraArgs(t *testing.T) {
	r, _, _ := uninstallRig(t)
	if code := r.run("uninstall", "--yes", "extra"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if _, err := r.vault.Get("a"); err != nil {
		t.Error("nothing should be removed on usage error")
	}
}

func TestUsageMentionsUninstall(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		if !strings.Contains(i18n.T(lang, i18n.Usage), "cid uninstall") {
			t.Errorf("%s usage misses cid uninstall", lang)
		}
	}
}

func TestHookRejectsUninstall(t *testing.T) {
	r, _, _ := uninstallRig(t)
	for _, args := range []string{"uninstall", "uninstall --yes", "UNINSTALL"} {
		code, out := r.hook(expansion("account", args))
		if code != 0 {
			t.Fatalf("%q: exit %d", args, code)
		}
		if !strings.Contains(out, "rejected") {
			t.Errorf("%q not rejected: %s", args, out)
		}
	}
	if _, err := r.vault.Get("a"); err != nil {
		t.Error("hook must not run uninstall")
	}
}
