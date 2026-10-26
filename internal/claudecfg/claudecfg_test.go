package claudecfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/credentials"
)

const (
	credsA = `{
  "claudeAiOauth": {"accessToken":"tok-a","refreshToken":"ref-a","expiresAt":1000},
  "mcpOAuth": {"srv": {"k":  "v"}}
}`
	claudeJSONA = `{
  "numStartups": 7,
  "oauthAccount": {"emailAddress":"a@example.com","displayName":"A"},
  "projects": {"x":   {"allowedTools": []}},
  "zeta": "last"
}`
)

func bundleB() credentials.Bundle {
	return credentials.Bundle{
		ClaudeAIOAuth: json.RawMessage(`{"accessToken":"tok-b","refreshToken":"ref-b","expiresAt":2000}`),
		OAuthAccount:  json.RawMessage(`{"emailAddress":"b@example.com","displayName":"B"}`),
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fakeHome(t *testing.T) Paths {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := ResolvePaths(t.TempDir())
	writeFile(t, p.Credentials, credsA)
	writeFile(t, p.ClaudeJSON, claudeJSONA)
	return p
}

func topLevel(t *testing.T, doc string) (keys []string, vals map[string]json.RawMessage) {
	t.Helper()
	o, err := parseObject([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return o.keys, o.vals
}

func TestResolvePathsHonorsConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join("cfg", "dir"))
	p := ResolvePaths("home")
	if p.Credentials != filepath.Join("cfg", "dir", ".credentials.json") ||
		p.ClaudeJSON != filepath.Join("cfg", "dir", ".claude.json") {
		t.Fatalf("unexpected paths: %+v", p)
	}
}

func TestFileStoreReadAndCurrent(t *testing.T) {
	s := NewFileStore(fakeHome(t))
	b, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if b.Email() != "a@example.com" || b.ExpiresAt() != 1000 {
		t.Fatalf("unexpected bundle: email=%q exp=%d", b.Email(), b.ExpiresAt())
	}
	if b.CapturedAt.IsZero() {
		t.Error("CapturedAt not set")
	}
	email, err := s.Current()
	if err != nil || email != "a@example.com" {
		t.Fatalf("Current() = %q, %v", email, err)
	}
}

func TestFileStoreReadWithoutCredentials(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	s := NewFileStore(ResolvePaths(t.TempDir()))
	if _, err := s.Read(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestFileStoreWriteChangesOnlyTwoKeys(t *testing.T) {
	p := fakeHome(t)
	s := NewFileStore(p)
	if err := s.Write(bundleB()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path, before, key string
		want                    string
	}{
		{"credentials", p.Credentials, credsA, "claudeAiOauth", string(bundleB().ClaudeAIOAuth)},
		{"claude.json", p.ClaudeJSON, claudeJSONA, "oauthAccount", string(bundleB().OAuthAccount)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bk, bv := topLevel(t, c.before)
			ak, av := topLevel(t, readFile(t, c.path))
			if strings.Join(bk, ",") != strings.Join(ak, ",") {
				t.Errorf("key order changed: %v -> %v", bk, ak)
			}
			for _, k := range bk {
				if k == c.key {
					if string(av[k]) != c.want {
						t.Errorf("%s not replaced", k)
					}
					continue
				}
				if !bytes.Equal(bv[k], av[k]) {
					t.Errorf("unrelated key %q changed: %s -> %s", k, bv[k], av[k])
				}
			}
		})
	}
	email, _ := s.Current()
	if email != "b@example.com" {
		t.Errorf("Current() after write = %q", email)
	}
}

func TestFileStoreWriteAppendsMissingKey(t *testing.T) {
	p := fakeHome(t)
	writeFile(t, p.ClaudeJSON, `{"a":1}`)
	if err := NewFileStore(p).Write(bundleB()); err != nil {
		t.Fatal(err)
	}
	keys, _ := topLevel(t, readFile(t, p.ClaudeJSON))
	if strings.Join(keys, ",") != "a,oauthAccount" {
		t.Fatalf("keys = %v", keys)
	}
}

func TestFileStoreWriteCreatesCredentialsWithPrivateMode(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := ResolvePaths(t.TempDir())
	if err := NewFileStore(p).Write(bundleB()); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	st, err := os.Stat(p.Credentials)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", st.Mode().Perm(), err)
	}
}

func TestFileStoreWriteFailureLeavesOriginalIntact(t *testing.T) {
	p := fakeHome(t)
	s := NewFileStore(p)
	s.rename = func(string, string) error { return errors.New("boom") }
	if err := s.Write(bundleB()); err == nil {
		t.Fatal("expected error")
	}
	if got := readFile(t, p.Credentials); got != credsA {
		t.Error("credentials file modified despite failed write")
	}
	entries, _ := os.ReadDir(filepath.Dir(p.Credentials))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFileStoreWriteRejectsEmptyBundle(t *testing.T) {
	p := fakeHome(t)
	if err := NewFileStore(p).Write(credentials.Bundle{}); err == nil {
		t.Fatal("expected error for bundle without OAuth block")
	}
	if readFile(t, p.Credentials) != credsA {
		t.Error("file modified")
	}
}

func TestFileStoreRejectsMalformedJSONWithoutLeakingContent(t *testing.T) {
	p := fakeHome(t)
	writeFile(t, p.Credentials, `{"claudeAiOauth": tok-secret-value`)
	_, err := NewFileStore(p).Read()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "tok-secret-value") {
		t.Error("error leaks file content")
	}
}

// ---- Keychain ----

type fakeRunner struct {
	item    string // JSON stored in the keychain item; "" means no item
	acct    string
	calls   [][]string
	failAdd bool
	failAll error // when set, every find-generic-password fails with it
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if name != "/usr/bin/security" || len(args) == 0 {
		return nil, errors.New("unexpected command")
	}
	switch args[0] {
	case "find-generic-password":
		if f.failAll != nil {
			return nil, f.failAll
		}
		if f.item == "" {
			return nil, ErrKeychainItemNotFound
		}
		for _, a := range args {
			if a == "-w" {
				return []byte(f.item + "\n"), nil
			}
		}
		return []byte(`keychain: "/Users/x/Library/Keychains/login.keychain-db"
attributes:
    "acct"<blob>="` + f.acct + `"
    "svce"<blob>="Claude Code-credentials"
`), nil
	case "add-generic-password":
		if f.failAdd {
			return nil, errors.New("exit status 1")
		}
		for i, a := range args {
			switch a {
			case "-a":
				f.acct = args[i+1]
			case "-w":
				f.item = args[i+1]
			}
		}
		return nil, nil
	}
	return nil, errors.New("unexpected subcommand")
}

func keychainFixture(t *testing.T) (*KeychainStore, *fakeRunner, Paths) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := ResolvePaths(t.TempDir())
	writeFile(t, p.ClaudeJSON, claudeJSONA)
	f := &fakeRunner{item: credsA, acct: "alice"}
	return NewKeychainStore(f, p), f, p
}

func TestKeychainReadAndCurrent(t *testing.T) {
	k, _, _ := keychainFixture(t)
	b, err := k.Read()
	if err != nil {
		t.Fatal(err)
	}
	if b.Email() != "a@example.com" || b.ExpiresAt() != 1000 {
		t.Fatalf("unexpected bundle: email=%q exp=%d", b.Email(), b.ExpiresAt())
	}
	if email, err := k.Current(); err != nil || email != "a@example.com" {
		t.Fatalf("Current() = %q, %v", email, err)
	}
}

func TestKeychainWriteReusesAccountAndPreservesOtherKeys(t *testing.T) {
	k, f, p := keychainFixture(t)
	if err := k.Write(bundleB()); err != nil {
		t.Fatal(err)
	}
	if f.acct != "alice" {
		t.Errorf("account attribute changed to %q", f.acct)
	}
	keys, vals := topLevel(t, f.item)
	if strings.Join(keys, ",") != "claudeAiOauth,mcpOAuth" {
		t.Errorf("keychain item keys = %v", keys)
	}
	if string(vals["claudeAiOauth"]) != string(bundleB().ClaudeAIOAuth) {
		t.Error("OAuth block not replaced")
	}
	_, ov := topLevel(t, credsA)
	if !bytes.Equal(vals["mcpOAuth"], ov["mcpOAuth"]) {
		t.Error("unrelated keychain key changed")
	}
	if email, _ := NewFileStore(p).Current(); email != "b@example.com" {
		t.Errorf("oauthAccount in .claude.json = %q", email)
	}
	var used bool
	for _, c := range f.calls {
		if c[1] == "add-generic-password" {
			used = contains(c, "-U")
		}
	}
	if !used {
		t.Error("add-generic-password -U not used")
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestKeychainErrorsCarryNoPayload(t *testing.T) {
	k, f, _ := keychainFixture(t)
	f.failAdd = true
	err := k.Write(bundleB())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "tok-b") || strings.Contains(err.Error(), "tok-a") {
		t.Error("error leaks token")
	}
}

func TestKeychainReadMissingItem(t *testing.T) {
	k, f, _ := keychainFixture(t)
	f.item = ""
	if _, err := k.Read(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestKeychainReadTransientFailureIsNotNoCredentials(t *testing.T) {
	k, f, _ := keychainFixture(t)
	f.failAll = errKeychain
	_, err := k.Read()
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want a non-ErrNoCredentials error, got %v", err)
	}
}

func TestFileStoreWriteRollsBackCredentialsOnAccountFailure(t *testing.T) {
	p := fakeHome(t)
	writeFile(t, p.ClaudeJSON, "{bad")
	if err := NewFileStore(p).Write(bundleB()); err == nil {
		t.Fatal("expected error")
	}
	if got := readFile(t, p.Credentials); got != credsA {
		t.Errorf("credentials not rolled back: %q", got)
	}
}

func TestKeychainWriteRollsBackItemOnAccountFailure(t *testing.T) {
	k, f, p := keychainFixture(t)
	writeFile(t, p.ClaudeJSON, "{bad")
	if err := k.Write(bundleB()); err == nil {
		t.Fatal("expected error")
	}
	_, got := topLevel(t, f.item)
	_, want := topLevel(t, credsA)
	if string(got["claudeAiOauth"]) != string(want["claudeAiOauth"]) {
		t.Errorf("keychain item not rolled back: %s", got["claudeAiOauth"])
	}
}

func TestOpenSelection(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := ResolvePaths(t.TempDir())
	withItem := &fakeRunner{item: credsA, acct: "alice"}
	without := &fakeRunner{}
	tests := []struct {
		name     string
		goos     string
		run      Runner
		wantType string
	}{
		{"darwin with item", "darwin", withItem, "*claudecfg.KeychainStore"},
		{"darwin without item", "darwin", without, "*claudecfg.FileStore"},
		{"linux", "linux", withItem, "*claudecfg.FileStore"},
		{"windows", "windows", withItem, "*claudecfg.FileStore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Open(tt.goos, tt.run, p)
			if err != nil {
				t.Fatal(err)
			}
			if name := typeName(got); name != tt.wantType {
				t.Fatalf("got %s, want %s", name, tt.wantType)
			}
		})
	}
}

func TestOpenFailsClosedOnProbeError(t *testing.T) {
	p := ResolvePaths(t.TempDir())
	got, err := Open("darwin", &fakeRunner{failAll: errKeychain}, p)
	if !errors.Is(err, errKeychain) || got != nil {
		t.Fatalf("want errKeychain and nil store, got %v, %v", got, err)
	}
}

type exitErr int

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return int(e) }

func TestClassifyExitCodes(t *testing.T) {
	if err := classify(exitErr(44)); !errors.Is(err, ErrKeychainItemNotFound) {
		t.Errorf("44 -> %v", err)
	}
	if err := classify(exitErr(36)); !errors.Is(err, errKeychain) {
		t.Errorf("36 -> %v", err)
	}
	if err := classify(errors.New("timeout")); !errors.Is(err, errKeychain) {
		t.Errorf("generic -> %v", err)
	}
}

func typeName(s Store) string {
	switch s.(type) {
	case *KeychainStore:
		return "*claudecfg.KeychainStore"
	case *FileStore:
		return "*claudecfg.FileStore"
	}
	return "unknown"
}

func TestBundleCapturedAtIsRecent(t *testing.T) {
	b, err := NewFileStore(fakeHome(t)).Read()
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(b.CapturedAt) > time.Minute {
		t.Errorf("CapturedAt too old: %v", b.CapturedAt)
	}
}
