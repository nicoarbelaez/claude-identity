package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/settings"
)

// configRig is a rig with a settings store on a temp dir and no env overrides.
func configRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	path := filepath.Join(filepath.Dir(r.deps.Config.ProfilesPath), "config.json")
	r.deps.Config.ConfigPath = path
	r.deps.Settings = settings.NewStore(path, func(string) string { return "" })
	return r
}

type configJSONOut struct {
	Paths    map[string]string `json:"paths"`
	Settings []struct {
		Key         string   `json:"key"`
		Value       string   `json:"value"`
		Source      string   `json:"source"`
		Default     string   `json:"default"`
		Kind        string   `json:"kind"`
		Allowed     []string `json:"allowed"`
		Env         string   `json:"env"`
		Description string   `json:"description"`
	} `json:"settings"`
}

func TestConfigShowsPathsAndSettingsTable(t *testing.T) {
	r := configRig(t)
	if code := r.run("config"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	out := r.stdout.String()
	for _, want := range []string{r.deps.Config.ProfilesPath, r.deps.Config.ConfigPath, r.deps.Config.ClaudeDir, "language = auto (default)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigSetGetUnset(t *testing.T) {
	r := configRig(t)
	if code := r.run("config", "set", "language", "es"); code != 0 {
		t.Fatalf("set exit %d: %s", code, r.stderr)
	}
	if code := r.run("config", "get", "language"); code != 0 || r.stdout.String() != "es\n" {
		t.Fatalf("get = %d %q", code, r.stdout)
	}
	r.run("config")
	if !strings.Contains(r.stdout.String(), "language = es (file)") {
		t.Errorf("table does not show the file source:\n%s", r.stdout)
	}
	if code := r.run("config", "unset", "language"); code != 0 {
		t.Fatalf("unset exit %d: %s", code, r.stderr)
	}
	if code := r.run("config", "get", "language"); code != 0 || r.stdout.String() != "auto\n" {
		t.Fatalf("get after unset = %d %q", code, r.stdout)
	}
}

func TestConfigUsageErrorsExitTwo(t *testing.T) {
	r := configRig(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"config", "get", "nope"}, "nope"},
		{[]string{"config", "set", "nope", "x"}, "nope"},
		{[]string{"config", "unset", "nope"}, "nope"},
		{[]string{"config", "set", "language", "fr"}, "auto, en, es"},
		{[]string{"config", "get"}, ""},
		{[]string{"config", "set", "language"}, ""},
		{[]string{"config", "set", "language", "es", "x"}, ""},
		{[]string{"config", "bogus"}, ""},
	}
	for _, c := range cases {
		if code := r.run(c.args...); code != 2 {
			t.Errorf("%v exit %d, want 2", c.args, code)
		}
		if !strings.Contains(r.stderr.String(), c.want) {
			t.Errorf("%v stderr %q lacks %q", c.args, r.stderr, c.want)
		}
	}
	if _, err := os.Stat(r.deps.Config.ConfigPath); err == nil {
		t.Error("rejected commands must not create config.json")
	}
}

func TestConfigJSONContract(t *testing.T) {
	r := configRig(t)
	r.run("config", "set", "language", "en")
	if code := r.run("config", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	var got configJSONOut
	if err := json.Unmarshal(r.stdout.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.stdout)
	}
	if got.Paths["profiles"] != r.deps.Config.ProfilesPath || got.Paths["config"] != r.deps.Config.ConfigPath || got.Paths["claude_dir"] != r.deps.Config.ClaudeDir {
		t.Errorf("paths = %v", got.Paths)
	}
	if len(got.Settings) != 1 {
		t.Fatalf("settings = %+v", got.Settings)
	}
	s := got.Settings[0]
	if s.Key != "language" || s.Value != "en" || s.Source != "file" || s.Default != "auto" || s.Kind != "enum" || s.Env != "CID_LANG" || s.Description == "" {
		t.Errorf("setting = %+v", s)
	}
	if strings.Join(s.Allowed, ",") != "auto,en,es" {
		t.Errorf("allowed = %v", s.Allowed)
	}

	if code := r.run("config", "get", "language", "--json"); code != 0 {
		t.Fatalf("get --json exit %d: %s", code, r.stderr)
	}
	var one struct{ Key, Value, Source string }
	if err := json.Unmarshal(r.stdout.Bytes(), &one); err != nil || one.Key != "language" || one.Value != "en" || one.Source != "file" {
		t.Errorf("get --json = %q (%v)", r.stdout, err)
	}
}

func TestConfigWithoutSettingsStillPrintsPaths(t *testing.T) {
	r := newRig(t) // Settings is nil
	if code := r.run("config"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(r.stdout.String(), r.deps.Config.ProfilesPath) {
		t.Errorf("output: %s", r.stdout)
	}
}

func TestConfigSetThroughHook(t *testing.T) {
	r := configRig(t)
	b := r.block(expansion("account", "config set language es"))
	if !strings.Contains(b.Reason, "language") {
		t.Errorf("reason = %q", b.Reason)
	}
	v, err := r.deps.Settings.Get("language")
	if err != nil || v.Value != "es" {
		t.Errorf("not applied: %+v %v", v, err)
	}
}

func TestUninstallRemovesConfigFiles(t *testing.T) {
	r := configRig(t)
	cfg := r.deps.Config.ConfigPath
	r.run("config", "set", "language", "es")
	if err := os.WriteFile(cfg+".corrupt-20260101T000000Z", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fsutil.BackupPath(cfg)); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if _, err := os.Stat(filepath.Dir(cfg)); err == nil {
		t.Error("config dir left behind")
	}
}
