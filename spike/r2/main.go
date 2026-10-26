// Command r2 is a throwaway experiment kit for spike T1/R2: does an already
// running Claude Code session pick up OAuth credentials swapped on disk by
// another process, and does it write its old credentials back on exit?
//
// It only ever touches the top-level "claudeAiOauth" key of
// .credentials.json and the top-level "oauthAccount" key of .claude.json.
// It never prints token values. Profiles are stored in plaintext under
// ~/.cid-spike for the duration of the spike only: delete that directory
// when done. Never commit it.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	keyOAuth   = "claudeAiOauth"
	keyAccount = "oauthAccount"
)

// Seams for tests.
var (
	renameFn     = os.Rename
	pollInterval = time.Second
	goos         = runtime.GOOS
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type paths struct {
	home      string
	creds     string
	claudeJSN string
	spike     string
}

type profile struct {
	Name          string          `json:"name"`
	ClaudeAiOauth json.RawMessage `json:"claudeAiOauth"`
	OauthAccount  json.RawMessage `json:"oauthAccount"`
}

type lastSwap struct {
	Profile    string `json:"profile"`
	SwappedAt  string `json:"swappedAt"`
	CredsSHA   string `json:"credentialsSha256"`
	ClaudeSHA  string `json:"claudeJsonSha256"`
	CredsPath  string `json:"credentialsPath"`
	ClaudePath string `json:"claudeJsonPath"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// errWriter remembers the first write error so output helpers need no checks.
type errWriter struct {
	w   io.Writer
	err error
}

func say(e *errWriter, format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

func run(args []string, w io.Writer) error {
	out := &errWriter{w: w}
	if err := runCmd(args, out); err != nil {
		return err
	}
	return out.err
}

func runCmd(args []string, out *errWriter) error {
	fs := flag.NewFlagSet("r2", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	home := fs.String("home", "", "override home directory (used by tests)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: r2 [--home dir] backup|capture <name>|swap <name>|watch [seconds]|restore <backup-dir>|status")
	}
	if goos == "darwin" && *home == "" {
		return errors.New("not supported in spike on macOS (credentials live in the Keychain)")
	}
	p, err := resolvePaths(*home)
	if err != nil {
		return err
	}
	cmd, a := rest[0], rest[1:]
	switch cmd {
	case "backup":
		return cmdBackup(p, out)
	case "capture":
		return withName(a, func(n string) error { return cmdCapture(p, n, out) })
	case "swap":
		return withName(a, func(n string) error { return cmdSwap(p, n, out) })
	case "watch":
		secs := 120
		if len(a) > 0 {
			if secs, err = strconv.Atoi(a[0]); err != nil || secs <= 0 {
				return fmt.Errorf("invalid seconds %q", a[0])
			}
		}
		return cmdWatch(p, secs, out)
	case "restore":
		if len(a) != 1 {
			return errors.New("usage: restore <backup-dir>")
		}
		return cmdRestore(p, a[0], out)
	case "status":
		return cmdStatus(p, out)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func withName(a []string, f func(string) error) error {
	if len(a) != 1 || !nameRe.MatchString(a[0]) {
		return errors.New("expected exactly one profile name matching [A-Za-z0-9_-]{1,64}")
	}
	return f(a[0])
}

func resolvePaths(home string) (paths, error) {
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return paths{}, err
		}
		home = h
	}
	p := paths{home: home, spike: filepath.Join(home, ".cid-spike")}
	if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
		p.creds = filepath.Join(cfg, ".credentials.json")
		p.claudeJSN = filepath.Join(cfg, ".claude.json")
	} else {
		p.creds = filepath.Join(home, ".claude", ".credentials.json")
		p.claudeJSN = filepath.Join(home, ".claude.json")
	}
	return p, nil
}

// ---- commands ----

func cmdBackup(p paths, out *errWriter) error {
	dir := filepath.Join(p.spike, "backups", time.Now().Format("20060102-150405.000"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for src, name := range map[string]string{p.creds: "credentials.json", p.claudeJSN: "claude.json"} {
		b, err := os.ReadFile(src)
		if errors.Is(err, os.ErrNotExist) {
			say(out, "skipped (missing): %s\n", filepath.Base(src))
			continue
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
	}
	say(out, "backup: %s\n", dir)
	return nil
}

func cmdRestore(p paths, dir string, out *errWriter) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("backup dir %q not found", dir)
	}
	restored := 0
	for dst, name := range map[string]string{p.creds: "credentials.json", p.claudeJSN: "claude.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := writeAtomic(dst, b); err != nil {
			return err
		}
		restored++
		say(out, "restored: %s\n", filepath.Base(dst))
	}
	if restored == 0 {
		return errors.New("backup dir contained no files to restore")
	}
	return nil
}

func cmdCapture(p paths, name string, out *errWriter) error {
	creds, err := readObj(p.creds)
	if err != nil {
		return err
	}
	cj, err := readObj(p.claudeJSN)
	if err != nil {
		return err
	}
	oa, ok1 := creds[keyOAuth]
	acct, ok2 := cj[keyAccount]
	if !ok1 || !ok2 {
		return fmt.Errorf("current files lack %s or %s (are you logged in?)", keyOAuth, keyAccount)
	}
	b, err := json.MarshalIndent(profile{Name: name, ClaudeAiOauth: oa, OauthAccount: acct}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(p.spike, "profiles"), 0o700); err != nil {
		return err
	}
	if err := writeAtomic(profilePath(p, name), b); err != nil {
		return err
	}
	say(out, "captured %q: account=%s expires=%s\n", name, accountLabel(acct), expiry(oa))
	return nil
}

func cmdSwap(p paths, name string, out *errWriter) error {
	pr, err := loadProfile(p, name)
	if err != nil {
		return err
	}
	if err := setKey(p.creds, keyOAuth, pr.ClaudeAiOauth); err != nil {
		return err
	}
	if err := setKey(p.claudeJSN, keyAccount, pr.OauthAccount); err != nil {
		return err
	}
	ls := lastSwap{
		Profile:    name,
		SwappedAt:  time.Now().UTC().Format(time.RFC3339),
		CredsSHA:   sha(p.creds),
		ClaudeSHA:  sha(p.claudeJSN),
		CredsPath:  p.creds,
		ClaudePath: p.claudeJSN,
	}
	b, _ := json.MarshalIndent(ls, "", "  ")
	if err := os.MkdirAll(p.spike, 0o700); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(p.spike, "last-swap.json"), b); err != nil {
		return err
	}
	say(out, "swapped to %q: account=%s expires=%s\n", name, accountLabel(pr.OauthAccount), expiry(pr.ClaudeAiOauth))
	return nil
}

func cmdStatus(p paths, out *errWriter) error {
	cj, err := readObj(p.claudeJSN)
	if err != nil {
		return err
	}
	creds, _ := readObj(p.creds)
	acct := cj[keyAccount]
	say(out, "current account: %s\n", accountLabel(acct))
	say(out, "current token expiry: %s\n", expiry(creds[keyOAuth]))
	matched := false
	ents, _ := os.ReadDir(filepath.Join(p.spike, "profiles"))
	for _, e := range ents {
		n := strings.TrimSuffix(e.Name(), ".json")
		pr, err := loadProfile(p, n)
		if err != nil {
			continue
		}
		if email(pr.OauthAccount) != "" && email(pr.OauthAccount) == email(acct) {
			say(out, "matches profile: %s\n", n)
			matched = true
		}
	}
	if !matched {
		say(out, "matches profile: (none)\n")
	}
	return nil
}

func cmdWatch(p paths, secs int, out *errWriter) error {
	var ls lastSwap
	b, err := os.ReadFile(filepath.Join(p.spike, "last-swap.json"))
	if err != nil {
		return errors.New("no last-swap.json: run swap first")
	}
	if err := json.Unmarshal(b, &ls); err != nil {
		return err
	}
	pr, _ := loadProfile(p, ls.Profile)
	say(out, "watching for %ds; baseline = profile %q\n", secs, ls.Profile)
	lastC, lastJ := ls.CredsSHA, ls.ClaudeSHA
	detected := false
	deadline := time.Now().Add(time.Duration(secs) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		c, j := sha(p.creds), sha(p.claudeJSN)
		if (c == lastC && j == lastJ) || c == "" || j == "" {
			continue
		}
		detected = true
		lastC, lastJ = c, j
		say(out, "WRITE-BACK detected at %s (credentials changed=%t, claude.json changed=%t vs last swap)\n",
			time.Now().Format("15:04:05"), c != ls.CredsSHA, j != ls.ClaudeSHA)
		creds, _ := readObj(p.creds)
		cj, _ := readObj(p.claudeJSN)
		reportKey(out, keyOAuth, creds[keyOAuth], pr.ClaudeAiOauth, "expires="+expiry(creds[keyOAuth]))
		reportKey(out, keyAccount, cj[keyAccount], pr.OauthAccount, "account="+accountLabel(cj[keyAccount]))
	}
	if !detected {
		say(out, "no write-back detected\n")
	}
	return nil
}

func reportKey(out *errWriter, key string, now, want json.RawMessage, detail string) {
	if sameJSON(now, want) {
		say(out, "  %s: unchanged vs profile (%s)\n", key, detail)
		return
	}
	say(out, "  %s: DIFFERS from swapped profile (%s)\n", key, detail)
}

// ---- helpers ----

func profilePath(p paths, name string) string {
	return filepath.Join(p.spike, "profiles", name+".json")
}

func loadProfile(p paths, name string) (profile, error) {
	var pr profile
	b, err := os.ReadFile(profilePath(p, name))
	if err != nil {
		return pr, fmt.Errorf("profile %q not found", name)
	}
	if err := json.Unmarshal(b, &pr); err != nil {
		return pr, err
	}
	if len(pr.ClaudeAiOauth) == 0 || len(pr.OauthAccount) == 0 {
		return pr, fmt.Errorf("profile %q is incomplete", name)
	}
	return pr, nil
}

func readObj(path string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return m, nil
}

// setKey replaces exactly one top-level key, preserving all others
// (raw values are kept verbatim; key order becomes alphabetical).
func setKey(path, key string, val json.RawMessage) error {
	m, err := readObj(path)
	if errors.Is(err, os.ErrNotExist) {
		m = map[string]json.RawMessage{}
	} else if err != nil {
		return err
	}
	m[key] = val
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

// writeAtomic writes to a temp file in the same directory, then renames.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = os.Remove(tmp) }
	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	_ = os.Chmod(tmp, mode)
	if err := renameFn(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func sha(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	if json.Compact(&x, a) != nil || json.Compact(&y, b) != nil {
		return false
	}
	var va, vb any
	if json.Unmarshal(x.Bytes(), &va) != nil || json.Unmarshal(y.Bytes(), &vb) != nil {
		return false
	}
	ja, _ := json.Marshal(va)
	jb, _ := json.Marshal(vb)
	return bytes.Equal(ja, jb)
}

func email(acct json.RawMessage) string {
	var a struct {
		Email string `json:"emailAddress"`
	}
	_ = json.Unmarshal(acct, &a)
	return a.Email
}

func accountLabel(acct json.RawMessage) string {
	var a struct {
		Email string `json:"emailAddress"`
		Name  string `json:"displayName"`
	}
	_ = json.Unmarshal(acct, &a)
	switch {
	case a.Email != "" && a.Name != "":
		return a.Email + " (" + a.Name + ")"
	case a.Email != "":
		return a.Email
	case a.Name != "":
		return a.Name
	}
	return "(unknown)"
}

func expiry(oauth json.RawMessage) string {
	var o struct {
		ExpiresAt float64 `json:"expiresAt"`
	}
	if json.Unmarshal(oauth, &o) != nil || o.ExpiresAt == 0 {
		return "(unknown)"
	}
	return time.UnixMilli(int64(o.ExpiresAt)).UTC().Format(time.RFC3339)
}
