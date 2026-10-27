package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
)

// maxStatuslineInput caps how much of Claude Code's status line JSON is read.
const maxStatuslineInput = 1 << 20

// StatuslineDeps are the collaborators of `cid statusline`.
type StatuslineDeps struct {
	// LiveEmail returns the email of the account Claude Code is logged in as
	// (oauthAccount.emailAddress; no secrets). Nil disables recording.
	LiveEmail func() (string, error)
	// Executable is the path of the running cid binary, written into the wrapper.
	Executable string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// Exec runs the wrapped command; nil means run it as a child process.
	// Tests inject a fake.
	Exec func(argv []string, stdin io.Reader, stdout, stderr io.Writer) int
}

// statusline dispatches `cid statusline [install|uninstall|-- <command>...]`.
// The record-and-passthrough form is what Claude Code runs on every refresh,
// so it fails open: nothing in the recording can change the wrapped command's
// output or exit code.
func (a *app) statusline(args []string) int {
	switch {
	case len(args) == 0:
		return a.statuslineRun(nil)
	case args[0] == "--":
		return a.statuslineRun(args[1:])
	case args[0] == "install" && len(args) == 1:
		return a.statuslineInstall()
	case args[0] == "uninstall" && len(args) == 1:
		return a.statuslineUninstall()
	}
	return a.usageError("")
}

// statuslineRun records the usage found in the status line JSON on stdin, then
// runs the wrapped command (if any) with the same bytes.
func (a *app) statuslineRun(wrapped []string) int {
	var raw []byte
	if a.Stdin != nil {
		raw, _ = io.ReadAll(io.LimitReader(a.Stdin, maxStatuslineInput))
	}
	a.recordUsage(raw)
	if len(wrapped) == 0 {
		return exitOK
	}
	run := a.Statusline.Exec
	if run == nil {
		run = runArgv
	}
	return run(wrapped, bytes.NewReader(raw), a.Stdout, a.Stderr)
}

// recordUsage stores the rate limits of raw for the active profile. It is best
// effort and never panics out: it only records when the live account is the
// active profile's account, so a reading is never filed under the wrong one.
func (a *app) recordUsage(raw []byte) {
	defer func() { _ = recover() }()
	if a.Usage == nil || a.Statusline.LiveEmail == nil {
		return
	}
	windows := parseRateLimits(raw)
	if len(windows) == 0 {
		return
	}
	p, err := a.Profiles.Active()
	if err != nil || p.Email == "" {
		return
	}
	if live, err := a.Statusline.LiveEmail(); err != nil || !strings.EqualFold(live, p.Email) {
		return
	}
	now := time.Now
	if a.Statusline.Now != nil {
		now = a.Statusline.Now
	}
	_ = a.Usage.Record(p.Name, windows, now())
}

// rateWindow is one rate_limits entry of the status line JSON. resets_at is
// Unix epoch seconds; an RFC 3339 string is accepted too.
type rateWindow struct {
	Used   *float64        `json:"used_percentage"`
	Resets json.RawMessage `json:"resets_at"`
}

// parseRateLimits extracts the five-hour and seven-day windows from Claude
// Code's status line JSON. Windows that are absent or incomplete are skipped.
func parseRateLimits(raw []byte) map[string]usage.Window {
	var in struct {
		RateLimits struct {
			FiveHour *rateWindow `json:"five_hour"`
			SevenDay *rateWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	out := map[string]usage.Window{}
	for name, w := range map[string]*rateWindow{usage.FiveHour: in.RateLimits.FiveHour, usage.SevenDay: in.RateLimits.SevenDay} {
		if w == nil || w.Used == nil || *w.Used < 0 || math.IsNaN(*w.Used) || math.IsInf(*w.Used, 0) {
			continue
		}
		at, ok := parseReset(w.Resets)
		if !ok {
			continue
		}
		out[name] = usage.Window{PercentUsed: *w.Used, ResetsAt: at}
	}
	return out
}

func parseReset(raw json.RawMessage) (time.Time, bool) {
	var secs float64
	if json.Unmarshal(raw, &secs) == nil && secs > 0 {
		if secs > 1e11 { // milliseconds
			secs /= 1000
		}
		whole, frac := math.Modf(secs)
		return time.Unix(int64(whole), int64(frac*1e9)).UTC(), true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// runArgv runs argv as a child process (no shell) wired to the given streams
// and returns its exit code. A command that cannot start is reported on stderr
// and returns exitRuntime.
func runArgv(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	err := cmd.Run()
	if err == nil {
		return exitOK
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code > 0 {
			return code
		}
		return exitRuntime // killed by a signal
	}
	_, _ = fmt.Fprintln(stderr, err)
	return exitRuntime
}

// ---- install / uninstall ----

// statuslineBackup is what install saves in cid's own state so uninstall can
// put the user's previous statusLine back exactly.
type statuslineBackup struct {
	HadStatusLine bool   `json:"hadStatusLine"`
	StatusLine    string `json:"statusLine,omitempty"` // the original JSON text, byte for byte
}

func (a *app) settingsPath() string { return filepath.Join(a.Config.ClaudeDir, "settings.json") }

func (a *app) backupPath() string {
	return filepath.Join(filepath.Dir(a.Config.ProfilesPath), "statusline.backup.json")
}

func (a *app) statuslineInstall() int {
	if a.Config.ClaudeDir == "" || a.Statusline.Executable == "" {
		return a.fail(errors.New("cannot locate the Claude config dir or the cid executable"), "")
	}
	settings := a.settingsPath()
	f, err := loadSettings(settings)
	if err != nil {
		return a.claudeSettingsFail(err, settings)
	}

	var sl *jsonObject
	cur, has := f.obj.get("statusLine")
	if has {
		if sl, err = parseObject(cur); err != nil {
			return a.unsupported()
		}
		raw, ok := sl.get("command")
		var command string
		if !ok || json.Unmarshal(raw, &command) != nil {
			return a.unsupported()
		}
		if isWrapper(command, a.Statusline.Executable) {
			a.printf(a.Stdout, "%s\n", a.t(i18n.StatuslineAlready))
			return exitOK
		}
		prev, ok := splitCommand(command)
		if !ok {
			return a.unsupported()
		}
		sl.set("command", jsonString(wrapperCommand(a.Statusline.Executable, prev)))
	} else {
		sl = newObject()
		sl.set("type", jsonString("command"))
		sl.set("command", jsonString(wrapperCommand(a.Statusline.Executable, nil)))
	}

	b, err := json.MarshalIndent(statuslineBackup{HadStatusLine: has, StatusLine: string(cur)}, "", "  ")
	if err != nil {
		return a.fail(err, "")
	}
	if err := fsutil.WriteAtomic(a.backupPath(), append(b, '\n')); err != nil {
		return a.fail(err, "")
	}
	f.obj.set("statusLine", sl.inline())
	if err := f.save(); err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.StatuslineInstalled, settings))
	return exitOK
}

func (a *app) statuslineUninstall() int {
	if a.Config.ClaudeDir == "" {
		return a.fail(errors.New("cannot locate the Claude config dir"), "")
	}
	switch out, err := a.restoreStatusline(); {
	case err != nil:
		return a.claudeSettingsFail(err, a.settingsPath())
	case out == restoreNotOurs:
		a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineNotOurs))
		return exitRuntime
	case out == restoreNoBackup:
		a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineNoBackup))
		return exitRuntime
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.StatuslineRemoved, a.settingsPath()))
	return exitOK
}

type restoreOutcome int

const (
	restored restoreOutcome = iota
	restoreNotOurs
	restoreNoBackup
)

// restoreStatusline puts the saved statusLine back, only while the current one
// is still cid's wrapper. On success the saved copy is deleted.
func (a *app) restoreStatusline() (restoreOutcome, error) {
	f, err := loadSettings(a.settingsPath())
	if err != nil {
		return 0, err
	}
	cur, has := f.obj.get("statusLine")
	if !has || !a.wrapperIn(cur) {
		return restoreNotOurs, nil
	}
	data, err := os.ReadFile(a.backupPath())
	var bk statuslineBackup
	if err != nil || json.Unmarshal(data, &bk) != nil || (bk.HadStatusLine && len(bk.StatusLine) == 0) {
		return restoreNoBackup, nil
	}
	if bk.HadStatusLine {
		f.obj.set("statusLine", json.RawMessage(bk.StatusLine))
	} else {
		f.obj.delete("statusLine")
	}
	if err := f.save(); err != nil {
		return 0, err
	}
	if err := os.Remove(a.backupPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return restored, nil
}

func (a *app) unsupported() int {
	a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineUnsupported))
	return exitRuntime
}

func (a *app) claudeSettingsFail(err error, settings string) int {
	if errors.Is(err, errBadSettings) {
		a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineBadSettings, settings))
		return exitRuntime
	}
	return a.fail(err, "")
}

// wrapperIn reports whether a statusLine value is cid's wrapper.
func (a *app) wrapperIn(statusLine json.RawMessage) bool {
	sl, err := parseObject(statusLine)
	if err != nil {
		return false
	}
	raw, ok := sl.get("command")
	var command string
	return ok && json.Unmarshal(raw, &command) == nil && isWrapper(command, a.Statusline.Executable)
}

// isWrapper reports whether a status line command is `cid statusline ...`:
// the program is named cid, or is the running binary under another name.
func isWrapper(command, exe string) bool {
	argv, err := shellwords.Parse(command)
	if err != nil || len(argv) < 2 || argv[1] != "statusline" {
		return false
	}
	prog := strings.ReplaceAll(argv[0], `\`, "/")
	if exe != "" && strings.EqualFold(prog, filepath.ToSlash(exe)) {
		return true
	}
	switch strings.ToLower(path.Base(prog)) {
	case "cid", "cid.exe":
		return true
	}
	return false
}

// shellSyntax lists characters whose meaning a split-and-requote round trip
// would lose (expansion, operators, escapes); such commands are not wrapped.
const shellSyntax = "$`|;&<>()*?[]{}~\\\n\r"

// splitCommand splits a status line command into argv with a shell-words
// parser. It reports false for an empty or unparsable command and for one that
// uses shell syntax.
func splitCommand(command string) ([]string, bool) {
	if strings.ContainsAny(command, shellSyntax) {
		return nil, false
	}
	argv, err := shellwords.Parse(command)
	return argv, err == nil && len(argv) > 0
}

// wrapperCommand builds `<cid> statusline [-- <argv>...]` for a shell.
func wrapperCommand(exe string, prev []string) string {
	words := []string{shellQuote(filepath.ToSlash(exe)), "statusline"}
	if len(prev) > 0 {
		words = append(words, "--")
		for _, w := range prev {
			words = append(words, shellQuote(w))
		}
	}
	return strings.Join(words, " ")
}

var safeWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

var doubleQuoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")

// shellQuote leaves plain words alone and double-quotes the rest, a form that
// POSIX shells, cmd and PowerShell all read the same way.
func shellQuote(s string) string {
	if safeWord.MatchString(s) {
		return s
	}
	return `"` + doubleQuoteEscaper.Replace(s) + `"`
}

// ---- settings.json as an order-preserving object ----

var errBadSettings = errors.New("settings.json is not a JSON object")

// jsonObject is a top-level JSON object that keeps key order and every value
// as raw bytes, so changing one key leaves all others untouched.
type jsonObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *jsonObject { return &jsonObject{vals: map[string]json.RawMessage{}} }

func parseObject(data []byte) (*jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errBadSettings
	}
	o := newObject()
	for dec.More() {
		tok, err := dec.Token()
		key, ok := tok.(string)
		if err != nil || !ok {
			return nil, errBadSettings
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, errBadSettings
		}
		o.set(key, raw)
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, errBadSettings
	}
	if _, err := dec.Token(); err != io.EOF { // trailing content
		return nil, errBadSettings
	}
	return o, nil
}

func (o *jsonObject) get(key string) (json.RawMessage, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// set replaces a key in place, or appends it when new.
func (o *jsonObject) set(key string, val json.RawMessage) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = append(json.RawMessage(nil), val...)
}

func (o *jsonObject) delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// encode renders the object one key per line; values are written verbatim.
func (o *jsonObject) encode() []byte {
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString("\n  ")
		buf.Write(jsonString(k))
		buf.WriteString(": ")
		buf.Write(o.vals[k])
	}
	if len(o.keys) > 0 {
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

// inline renders the object on one line, for a nested value.
func (o *jsonObject) inline() json.RawMessage {
	parts := make([]string, len(o.keys))
	for i, k := range o.keys {
		parts[i] = string(jsonString(k)) + ": " + string(o.vals[k])
	}
	return json.RawMessage("{" + strings.Join(parts, ", ") + "}")
}

// jsonString encodes s as a JSON string without HTML escaping.
func jsonString(s string) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// settingsFile is the user's settings.json, loaded for a targeted edit.
type settingsFile struct {
	path    string
	prev    []byte // original bytes; nil when the file did not exist
	obj     *jsonObject
	existed bool
}

// loadSettings reads settings.json; a missing file is an empty object. Content
// that is not a JSON object returns errBadSettings and is never overwritten.
func loadSettings(p string) (*settingsFile, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &settingsFile{path: p, obj: newObject()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings.json: %w", err)
	}
	o, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	return &settingsFile{path: p, prev: data, obj: o, existed: true}, nil
}

// save writes the edited object atomically, first keeping the file as it was
// in settings.json.bak.
func (f *settingsFile) save() error {
	if f.existed {
		if err := fsutil.WriteAtomic(fsutil.BackupPath(f.path), f.prev); err != nil {
			return fmt.Errorf("back up settings.json: %w", err)
		}
	}
	// WriteAtomic creates 0600 files; keep the mode the user had.
	var mode os.FileMode
	if st, err := os.Stat(f.path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := fsutil.WriteAtomic(f.path, f.obj.encode()); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	if mode != 0 {
		_ = os.Chmod(f.path, mode) // best effort: the content is already saved
	}
	return nil
}
