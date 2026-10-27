package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

// execCall records one wrapped-command run of the fake Exec.
type execCall struct {
	argv  []string
	stdin []byte
}

// fakeExec replaces the real process runner: it records the call, writes out
// to stdout and returns code.
func (r *rig) fakeExec(out string, code int) *execCall {
	call := &execCall{}
	r.deps.Statusline.Exec = func(argv []string, stdin io.Reader, stdout, _ io.Writer) int {
		call.argv = argv
		call.stdin, _ = io.ReadAll(stdin)
		_, _ = io.WriteString(stdout, out)
		return code
	}
	return call
}

func (r *rig) statusline(stdin string, args ...string) int {
	r.t.Helper()
	r.deps.Stdin = strings.NewReader(stdin)
	return r.run(append([]string{"statusline"}, args...)...)
}

func rateLimits(now time.Time) string {
	return fmt.Sprintf(`{"model":{"id":"x"},"rate_limits":{"five_hour":{"used_percentage":42.5,"resets_at":%d},"seven_day":{"used_percentage":10,"resets_at":%d}}}`,
		now.Add(3*time.Hour).Unix(), now.Add(48*time.Hour).Unix())
}

func newStatuslineRig(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r := newRig(t)
	r.twoProfiles() // b is active and live
	now := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	r.deps.Statusline = StatuslineDeps{
		LiveEmail:  r.live.Current,
		Executable: "/opt/cid/cid",
		Now:        func() time.Time { return now },
	}
	return r, now
}

func (r *rig) readUsage() map[string]usage.Entry {
	r.t.Helper()
	all, err := r.usage.All()
	if err != nil {
		r.t.Fatal(err)
	}
	return all
}

func TestStatuslineRecordsActiveProfile(t *testing.T) {
	r, now := newStatuslineRig(t)
	if code := r.statusline(rateLimits(now)); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if r.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty without a wrapped command", r.stdout)
	}
	all := r.readUsage()
	e, ok := all["b"]
	if !ok || len(all) != 1 {
		t.Fatalf("usage = %+v, want only b", all)
	}
	if !e.FetchedAt.Equal(now) {
		t.Errorf("fetchedAt = %v", e.FetchedAt)
	}
	five := e.Windows[usage.FiveHour]
	if five.PercentUsed != 42.5 || !five.ResetsAt.Equal(now.Add(3*time.Hour)) {
		t.Errorf("five_hour = %+v", five)
	}
	if e.Windows[usage.SevenDay].PercentUsed != 10 {
		t.Errorf("seven_day = %+v", e.Windows[usage.SevenDay])
	}
}

func TestStatuslineRecordsPartialWindows(t *testing.T) {
	r, now := newStatuslineRig(t)
	in := fmt.Sprintf(`{"rate_limits":{"seven_day":{"used_percentage":7,"resets_at":%d}}}`, now.Add(time.Hour).Unix())
	if code := r.statusline(in); code != 0 {
		t.Fatalf("exit %d", code)
	}
	e := r.readUsage()["b"]
	if _, ok := e.Windows[usage.FiveHour]; ok || e.Windows[usage.SevenDay].PercentUsed != 7 {
		t.Errorf("windows = %+v", e.Windows)
	}
}

func TestStatuslineAcceptsRFC3339Reset(t *testing.T) {
	r, now := newStatuslineRig(t)
	in := `{"rate_limits":{"five_hour":{"used_percentage":7,"resets_at":"2026-03-04T13:00:00Z"}}}`
	if code := r.statusline(in); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := r.readUsage()["b"].Windows[usage.FiveHour].ResetsAt; !got.Equal(now.Add(3 * time.Hour)) {
		t.Errorf("resetsAt = %v", got)
	}
}

func TestStatuslineDoesNotRecord(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*rig)
		stdin func(time.Time) string
	}{
		{"live account differs from the active profile", func(r *rig) { r.login("other@example.com", "tok-o") }, rateLimits},
		{"no rate limits", nil, func(time.Time) string { return `{"model":{"id":"x"}}` }},
		{"spend limit only", nil, func(time.Time) string { return `{"rate_limits":{"spend_limit":{"used":1}}}` }},
		{"not json", nil, func(time.Time) string { return `not json` }},
		{"empty stdin", nil, func(time.Time) string { return `` }},
		{"window without reset", nil, func(time.Time) string { return `{"rate_limits":{"five_hour":{"used_percentage":5}}}` }},
		{"negative percentage", nil, func(now time.Time) string {
			return fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":-1,"resets_at":%d}}}`, now.Unix()+60)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, now := newStatuslineRig(t)
			if tc.setup != nil {
				tc.setup(r)
			}
			if code := r.statusline(tc.stdin(now)); code != 0 {
				t.Fatalf("exit %d: %s", code, r.stderr)
			}
			if all := r.readUsage(); len(all) != 0 {
				t.Errorf("recorded %+v", all)
			}
		})
	}
}

func TestStatuslineDoesNotRecordWithoutActiveProfile(t *testing.T) {
	r := newRig(t)
	r.login("a@example.com", "tok-a")
	r.deps.Statusline = StatuslineDeps{LiveEmail: r.live.Current}
	if code := r.statusline(rateLimits(time.Now())); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if all := r.readUsage(); len(all) != 0 {
		t.Errorf("recorded %+v", all)
	}
}

func TestStatuslinePassesStdinAndOutputToWrappedCommand(t *testing.T) {
	r, now := newStatuslineRig(t)
	call := r.fakeExec("my status\n", 7)
	in := rateLimits(now)
	code := r.statusline(in, "--", "node", "C:/Users/x/.claude/statusline.mjs", "--flag")
	if code != 7 {
		t.Errorf("exit = %d, want the wrapped command's 7", code)
	}
	if got := r.stdout.String(); got != "my status\n" {
		t.Errorf("stdout = %q", got)
	}
	if strings.Join(call.argv, "|") != "node|C:/Users/x/.claude/statusline.mjs|--flag" {
		t.Errorf("argv = %q", call.argv)
	}
	if string(call.stdin) != in {
		t.Errorf("wrapped stdin = %q, want the original bytes", call.stdin)
	}
	if _, ok := r.readUsage()["b"]; !ok {
		t.Error("usage not recorded alongside the passthrough")
	}
}

type brokenUsage struct{ panics bool }

func (b brokenUsage) Record(string, map[string]usage.Window, time.Time) error {
	if b.panics {
		panic("boom")
	}
	return errors.New("disk on fire")
}
func (brokenUsage) Forget(string) error { return nil }

func TestStatuslineFailsOpen(t *testing.T) {
	for _, panics := range []bool{false, true} {
		r, now := newStatuslineRig(t)
		r.deps.Usage = brokenUsage{panics: panics}
		call := r.fakeExec("still shown", 0)
		if code := r.statusline(rateLimits(now), "--", "node", "x.mjs"); code != 0 {
			t.Fatalf("panics=%v: exit %d", panics, code)
		}
		if r.stdout.String() != "still shown" || len(call.argv) == 0 {
			t.Errorf("panics=%v: wrapped command did not run (stdout %q)", panics, r.stdout)
		}
	}
}

func TestStatuslineFailsOpenWithoutDependencies(t *testing.T) {
	r := newRig(t)
	r.deps.Usage = nil
	call := r.fakeExec("ok", 0)
	if code := r.statusline(`{"rate_limits":{}}`, "--", "echo"); code != 0 || len(call.argv) != 1 {
		t.Errorf("exit %d, argv %v", code, call.argv)
	}
}

func TestStatuslineRejectsBadArguments(t *testing.T) {
	r, _ := newStatuslineRig(t)
	for _, args := range [][]string{{"bogus"}, {"install", "extra"}, {"uninstall", "extra"}, {"--json"}} {
		if code := r.statusline("", args...); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
}

func TestStatuslineEmptyWrappedCommandPrintsNothing(t *testing.T) {
	r, now := newStatuslineRig(t)
	call := r.fakeExec("should not run", 0)
	if code := r.statusline(rateLimits(now), "--"); code != 0 || r.stdout.Len() != 0 || call.argv != nil {
		t.Errorf("exit %d, stdout %q, argv %v", code, r.stdout, call.argv)
	}
}

func TestStatuslineWrappedCommandThatCannotStart(t *testing.T) {
	r, now := newStatuslineRig(t) // real runner: Exec stays nil
	code := r.statusline(rateLimits(now), "--", filepath.Join(t.TempDir(), "no-such-binary"))
	if code != exitRuntime || r.stderr.Len() == 0 {
		t.Errorf("exit %d, stderr %q", code, r.stderr)
	}
	if _, ok := r.readUsage()["b"]; !ok {
		t.Error("usage must be recorded even when the wrapped command cannot start")
	}
}

// TestRunArgvStreamsAndExitCode runs the test binary itself as the wrapped
// command (TestHelperWrapped), covering the real argv runner end to end.
func TestRunArgvStreamsAndExitCode(t *testing.T) {
	t.Setenv("CID_STATUSLINE_HELPER", "1")
	var out, errOut bytes.Buffer
	code := runArgv([]string{os.Args[0], "-test.run=^TestHelperWrapped$"}, strings.NewReader("hello"), &out, &errOut)
	if code != 3 {
		t.Errorf("exit = %d, want 3 (stderr %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "got:hello") || errOut.String() != "err-line" {
		t.Errorf("stdout %q, stderr %q", out.String(), errOut.String())
	}
}

// TestHelperWrapped is the wrapped command of TestRunArgvStreamsAndExitCode.
func TestHelperWrapped(t *testing.T) {
	if os.Getenv("CID_STATUSLINE_HELPER") != "1" {
		t.Skip("helper process")
	}
	b, _ := io.ReadAll(os.Stdin)
	fmt.Fprint(os.Stdout, "got:"+string(b))
	fmt.Fprint(os.Stderr, "err-line")
	os.Exit(3)
}

// ---- install / uninstall ----

const prevSettings = `{
  "theme": "dark",
  "statusLine": {
    "type": "command",
    "command": "node C:/Users/x/.claude/statusline.mjs",
    "padding": 2
  },
  "env": {"A": "1"}
}
`

func (r *rig) settingsPath() string { return filepath.Join(r.deps.Config.ClaudeDir, "settings.json") }

func (r *rig) backupPath() string {
	return filepath.Join(filepath.Dir(r.deps.Config.ProfilesPath), "statusline.backup.json")
}

func (r *rig) writeSettings(content string) {
	r.t.Helper()
	if err := os.MkdirAll(r.deps.Config.ClaudeDir, 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(r.settingsPath(), []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) readSettings() map[string]json.RawMessage {
	r.t.Helper()
	b, err := os.ReadFile(r.settingsPath())
	if err != nil {
		r.t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		r.t.Fatalf("settings.json is not valid JSON: %v\n%s", err, b)
	}
	return m
}

func (r *rig) statusLineCommand() string {
	r.t.Helper()
	var sl struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(r.readSettings()["statusLine"], &sl); err != nil {
		r.t.Fatal(err)
	}
	return sl.Command
}

func TestStatuslineInstallWrapsExistingCommand(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.writeSettings(prevSettings)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if got, want := r.statusLineCommand(), "/opt/cid/cid statusline -- node C:/Users/x/.claude/statusline.mjs"; got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
	var sl map[string]any
	_ = json.Unmarshal(r.readSettings()["statusLine"], &sl)
	if sl["type"] != "command" || sl["padding"] != float64(2) {
		t.Errorf("other statusLine fields lost: %v", sl)
	}
	m := r.readSettings()
	if string(m["theme"]) != `"dark"` || string(m["env"]) != `{"A": "1"}` {
		t.Errorf("other keys changed: theme=%s env=%s", m["theme"], m["env"])
	}
	b, _ := os.ReadFile(r.settingsPath())
	theme, line, env := strings.Index(string(b), `"theme"`), strings.Index(string(b), `"statusLine"`), strings.Index(string(b), `"env"`)
	if !(theme < line && line < env) {
		t.Errorf("key order changed:\n%s", b)
	}
	// settings.json.bak holds the file as it was before the change.
	if bak, err := os.ReadFile(r.settingsPath() + ".bak"); err != nil || string(bak) != prevSettings {
		t.Errorf("settings.json.bak = %q (%v)", bak, err)
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.StatuslineInstalled, r.settingsPath())) {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestStatuslineInstallWithoutPreviousStatusLine(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.writeSettings(`{"theme":"dark"}`)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	var sl struct{ Type, Command string }
	_ = json.Unmarshal(r.readSettings()["statusLine"], &sl)
	if sl.Type != "command" || sl.Command != "/opt/cid/cid statusline" {
		t.Errorf("statusLine = %+v", sl)
	}
}

func TestStatuslineInstallCreatesMissingSettings(t *testing.T) {
	r, _ := newStatuslineRig(t)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if r.statusLineCommand() != "/opt/cid/cid statusline" {
		t.Errorf("command = %q", r.statusLineCommand())
	}
}

func TestStatuslineInstallQuoting(t *testing.T) {
	tests := []struct {
		name, exe, prev, want string
	}{
		{"spaces in cid path", "/opt/my tools/cid", "node a.mjs", `"/opt/my tools/cid" statusline -- node a.mjs`},
		{"quoted argument with spaces", "/opt/cid/cid", `node "C:/Program Files/x.mjs" --flag`, `/opt/cid/cid statusline -- node "C:/Program Files/x.mjs" --flag`},
		{"single quoted argument", "/opt/cid/cid", `node 'a b.mjs'`, `/opt/cid/cid statusline -- node "a b.mjs"`},
		{"windows drive path", "C:/Users/x/bin/cid.exe", "node a.mjs", `C:/Users/x/bin/cid.exe statusline -- node a.mjs`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newStatuslineRig(t)
			r.deps.Statusline.Executable = tc.exe
			prev, _ := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": tc.prev}})
			r.writeSettings(string(prev))
			if code := r.run("statusline", "install"); code != 0 {
				t.Fatalf("exit %d: %s", code, r.stderr)
			}
			if got := r.statusLineCommand(); got != tc.want {
				t.Errorf("command = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatuslineInstallIsIdempotent(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.writeSettings(prevSettings)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	before, _ := os.ReadFile(r.settingsPath())
	backup, _ := os.ReadFile(r.backupPath())
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	after, _ := os.ReadFile(r.settingsPath())
	backup2, _ := os.ReadFile(r.backupPath())
	if !bytes.Equal(before, after) || !bytes.Equal(backup, backup2) {
		t.Error("a second install changed files")
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.StatuslineAlready)) {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestStatuslineInstallRecognizesRenamedBinary(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.deps.Statusline.Executable = "/opt/tools/cid-dev"
	r.writeSettings(prevSettings)
	_ = r.run("statusline", "install")
	before, _ := os.ReadFile(r.settingsPath())
	if code := r.run("statusline", "install"); code != 0 || !strings.Contains(r.stdout.String(), r.msg(i18n.StatuslineAlready)) {
		t.Fatalf("exit %d, stdout %q", code, r.stdout)
	}
	if after, _ := os.ReadFile(r.settingsPath()); !bytes.Equal(before, after) {
		t.Error("second install double-wrapped the command")
	}
	if code := r.run("statusline", "uninstall"); code != 0 {
		t.Fatalf("uninstall exit %d: %s", code, r.stderr)
	}
}

func TestStatuslineInstallRefusesWhatItCannotWrapSafely(t *testing.T) {
	tests := []struct{ name, settings string }{
		{"pipe", `{"statusLine":{"type":"command","command":"node a.mjs | tee x"}}`},
		{"variable", `{"statusLine":{"type":"command","command":"node $HOME/a.mjs"}}`},
		{"backslash path", `{"statusLine":{"type":"command","command":"node C:\\Users\\x\\a.mjs"}}`},
		{"unbalanced quote", `{"statusLine":{"type":"command","command":"node \"a.mjs"}}`},
		{"empty command", `{"statusLine":{"type":"command","command":""}}`},
		{"statusLine is not an object", `{"statusLine":"node a.mjs"}`},
		{"command is not a string", `{"statusLine":{"type":"command","command":5}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newStatuslineRig(t)
			r.writeSettings(tc.settings)
			if code := r.run("statusline", "install"); code != exitRuntime {
				t.Fatalf("exit %d, want %d", code, exitRuntime)
			}
			if !strings.Contains(r.stderr.String(), r.msg(i18n.StatuslineUnsupported)) {
				t.Errorf("stderr = %q", r.stderr)
			}
			if b, _ := os.ReadFile(r.settingsPath()); string(b) != tc.settings {
				t.Errorf("settings changed: %s", b)
			}
			if exists(r.backupPath()) {
				t.Error("backup written for a refused install")
			}
		})
	}
}

func TestStatuslineInstallRefusesInvalidSettings(t *testing.T) {
	for _, content := range []string{`{oops`, `[]`, `"x"`} {
		r, _ := newStatuslineRig(t)
		r.writeSettings(content)
		if code := r.run("statusline", "install"); code != exitRuntime {
			t.Fatalf("%q: exit %d", content, code)
		}
		if !strings.Contains(r.stderr.String(), r.msg(i18n.StatuslineBadSettings, r.settingsPath())) {
			t.Errorf("%q: stderr = %q", content, r.stderr)
		}
		if b, _ := os.ReadFile(r.settingsPath()); string(b) != content {
			t.Errorf("%q: settings changed to %s", content, b)
		}
	}
}

func TestStatuslineUninstallRestoresExactPreviousValue(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.writeSettings(prevSettings)
	var orig map[string]json.RawMessage
	_ = json.Unmarshal([]byte(prevSettings), &orig)

	if code := r.run("statusline", "install"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	if code := r.run("statusline", "uninstall"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	got := r.readSettings()
	if string(got["statusLine"]) != string(orig["statusLine"]) {
		t.Errorf("statusLine = %s, want %s", got["statusLine"], orig["statusLine"])
	}
	if exists(r.backupPath()) {
		t.Error("backup file should be removed")
	}
	if !strings.Contains(r.stdout.String(), r.msg(i18n.StatuslineRemoved, r.settingsPath())) {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestStatuslineUninstallRestoresBytesVerbatim(t *testing.T) {
	r, _ := newStatuslineRig(t)
	const value = `{"type":"command",   "command":"node  a.mjs",
      "padding":1}`
	r.writeSettings(`{"a":1,"statusLine":` + value + `,"b":2}`)
	_ = r.run("statusline", "install")
	if code := r.run("statusline", "uninstall"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if got := string(r.readSettings()["statusLine"]); got != value {
		t.Errorf("statusLine = %q, want %q", got, value)
	}
}

func TestStatuslineUninstallDeletesKeyWhenThereWasNone(t *testing.T) {
	r, _ := newStatuslineRig(t)
	r.writeSettings(`{"theme":"dark"}`)
	_ = r.run("statusline", "install")
	if code := r.run("statusline", "uninstall"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	m := r.readSettings()
	if _, ok := m["statusLine"]; ok || string(m["theme"]) != `"dark"` {
		t.Errorf("settings = %v", m)
	}
}

func TestStatuslineUninstallRefuses(t *testing.T) {
	t.Run("current statusLine is not the cid wrapper", func(t *testing.T) {
		r, _ := newStatuslineRig(t)
		r.writeSettings(prevSettings)
		if code := r.run("statusline", "uninstall"); code != exitRuntime {
			t.Fatalf("exit %d", code)
		}
		if !strings.Contains(r.stderr.String(), r.msg(i18n.StatuslineNotOurs)) {
			t.Errorf("stderr = %q", r.stderr)
		}
		if b, _ := os.ReadFile(r.settingsPath()); string(b) != prevSettings {
			t.Error("settings changed")
		}
	})
	t.Run("no settings file", func(t *testing.T) {
		r, _ := newStatuslineRig(t)
		if code := r.run("statusline", "uninstall"); code != exitRuntime || !strings.Contains(r.stderr.String(), r.msg(i18n.StatuslineNotOurs)) {
			t.Errorf("exit %d, stderr %q", code, r.stderr)
		}
	})
	t.Run("backup is gone", func(t *testing.T) {
		r, _ := newStatuslineRig(t)
		r.writeSettings(prevSettings)
		_ = r.run("statusline", "install")
		_ = os.Remove(r.backupPath())
		installed, _ := os.ReadFile(r.settingsPath())
		if code := r.run("statusline", "uninstall"); code != exitRuntime || !strings.Contains(r.stderr.String(), r.msg(i18n.StatuslineNoBackup)) {
			t.Errorf("exit %d, stderr %q", code, r.stderr)
		}
		if b, _ := os.ReadFile(r.settingsPath()); !bytes.Equal(b, installed) {
			t.Error("settings changed")
		}
	})
}

func TestCidUninstallRestoresTheStatusLine(t *testing.T) {
	r, _, _ := uninstallRig(t)
	r.deps.Statusline = StatuslineDeps{Executable: "/opt/cid/cid"}
	r.writeSettings(prevSettings)
	if code := r.run("statusline", "install"); code != 0 {
		t.Fatal(r.stderr.String())
	}
	if code := r.run("uninstall", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if got := r.statusLineCommand(); got != "node C:/Users/x/.claude/statusline.mjs" {
		t.Errorf("statusLine command = %q after cid uninstall", got)
	}
	if exists(r.backupPath()) || exists(filepath.Dir(r.backupPath())) {
		t.Error("cid config dir or backup left behind")
	}
}

// gatedUsage is a usage port whose Record waits for gate to close (or for a
// timeout) before it returns, to model a slow or locked usage store.
type gatedUsage struct {
	UsagePort
	gate    <-chan struct{}
	waited  *bool // set to true when Record saw the gate close in time
	timeout time.Duration
}

func (g gatedUsage) Record(string, map[string]usage.Window, time.Time) error {
	select {
	case <-g.gate:
		*g.waited = true
	case <-time.After(g.timeout):
	}
	return errors.New("usage store is locked")
}

func TestStatuslineRunsWrappedCommandWithoutWaitingForRecording(t *testing.T) {
	r, now := newStatuslineRig(t)
	started := make(chan struct{})
	var sawExec bool
	r.deps.Usage = gatedUsage{gate: started, waited: &sawExec, timeout: 2 * time.Second}
	r.deps.Statusline.Exec = func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		close(started) // the wrapped command is running while Record is still blocked
		_, _ = io.WriteString(stdout, "line")
		return 3
	}
	if code := r.statusline(rateLimits(now), "--", "node", "x.mjs"); code != 3 {
		t.Errorf("exit = %d, want the wrapped command's 3", code)
	}
	if !sawExec {
		t.Error("recording ran before (or without) the wrapped command; it must not delay it")
	}
	if got := r.stdout.String(); got != "line" {
		t.Errorf("stdout = %q", got)
	}
}

func TestStatuslineDoesNotWaitForeverOnALockedStore(t *testing.T) {
	r, now := newStatuslineRig(t)
	old := recordGrace
	recordGrace = 30 * time.Millisecond
	defer func() { recordGrace = old }()
	never := make(chan struct{})
	var sawExec bool
	r.deps.Usage = gatedUsage{gate: never, waited: &sawExec, timeout: 3 * time.Second}
	r.fakeExec("untouched", 0)
	begin := time.Now()
	code := r.statusline(rateLimits(now), "--", "node", "x.mjs")
	if code != 0 || r.stdout.String() != "untouched" {
		t.Errorf("exit %d, stdout %q: a locked store changed the wrapped command's result", code, r.stdout)
	}
	if took := time.Since(begin); took > time.Second {
		t.Errorf("took %v: the locked store delayed the status line", took)
	}
}
