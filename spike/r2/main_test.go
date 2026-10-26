package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const dummyPrefix = "DUMMY-SECRET-"

type fakeHome struct {
	t    *testing.T
	home string
}

func newFakeHome(t *testing.T) *fakeHome {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	h := &fakeHome{t: t, home: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(h.home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *fakeHome) creds() string { return filepath.Join(h.home, ".claude", ".credentials.json") }
func (h *fakeHome) cjson() string { return filepath.Join(h.home, ".claude.json") }

// setAccount writes both files with dummy data for the given account tag.
func (h *fakeHome) setAccount(tag string, expires int64) {
	h.t.Helper()
	creds := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  dummyPrefix + "ACCESS-" + tag,
			"refreshToken": dummyPrefix + "REFRESH-" + tag,
			"expiresAt":    expires,
		},
		"unrelatedCred": "keep-me",
	}
	h.write(h.creds(), creds)
	cj := map[string]any{
		"oauthAccount": map[string]any{"emailAddress": tag + "@example.com", "displayName": "User " + tag},
		"numStartups":  42,
		"projects":     map[string]any{"/p": map[string]any{"allowedTools": []string{"a"}}},
	}
	h.write(h.cjson(), cj)
}

func (h *fakeHome) write(path string, v any) {
	h.t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *fakeHome) read(path string) map[string]json.RawMessage {
	h.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		h.t.Fatal(err)
	}
	return m
}

func (h *fakeHome) run(args ...string) (string, error) {
	h.t.Helper()
	var buf bytes.Buffer
	err := run(append([]string{"--home", h.home}, args...), &buf)
	return buf.String(), err
}

func (h *fakeHome) mustRun(args ...string) string {
	h.t.Helper()
	out, err := h.run(args...)
	if err != nil {
		h.t.Fatalf("run %v: %v\noutput: %s", args, err, out)
	}
	return out
}

func rawEmail(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var a struct {
		Email string `json:"emailAddress"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	return a.Email
}

func compact(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSwapPreservesUnrelatedKeys(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("b", 1900000000000)
	h.mustRun("capture", "b")
	h.setAccount("a", 1800000000000)
	h.mustRun("capture", "a")
	before := h.read(h.cjson())
	beforeCreds := h.read(h.creds())

	h.mustRun("swap", "b")

	after := h.read(h.cjson())
	afterCreds := h.read(h.creds())
	tests := []struct {
		name string
		got  json.RawMessage
		want json.RawMessage
	}{
		{"numStartups kept", after["numStartups"], before["numStartups"]},
		{"projects kept", after["projects"], before["projects"]},
		{"unrelatedCred kept", afterCreds["unrelatedCred"], beforeCreds["unrelatedCred"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.got) == 0 || compact(t, tt.got) != compact(t, tt.want) {
				t.Fatalf("got %s want %s", tt.got, tt.want)
			}
		})
	}
	if e := rawEmail(t, after["oauthAccount"]); e != "b@example.com" {
		t.Fatalf("oauthAccount email = %q, want b@example.com", e)
	}
	if !strings.Contains(string(afterCreds["claudeAiOauth"]), "ACCESS-b") {
		t.Fatalf("claudeAiOauth not swapped to b")
	}
	if _, err := os.Stat(filepath.Join(h.home, ".cid-spike", "last-swap.json")); err != nil {
		t.Fatalf("last-swap.json missing: %v", err)
	}
}

func TestSwapIsAtomicTempPlusRename(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("b", 1)
	h.mustRun("capture", "b")
	h.setAccount("a", 1)
	orig, _ := os.ReadFile(h.cjson())

	oren := renameFn
	t.Cleanup(func() { renameFn = oren })

	t.Run("failed rename leaves original untouched and no temp files", func(t *testing.T) {
		renameFn = func(_, _ string) error { return errors.New("boom") }
		if _, err := h.run("swap", "b"); err == nil {
			t.Fatal("expected error")
		}
		now, _ := os.ReadFile(h.cjson())
		if !bytes.Equal(orig, now) {
			t.Fatal("original file modified despite failed rename")
		}
		for _, dir := range []string{h.home, filepath.Join(h.home, ".claude")} {
			ents, _ := os.ReadDir(dir)
			for _, e := range ents {
				if strings.Contains(e.Name(), ".tmp") {
					t.Fatalf("leftover temp file %s", e.Name())
				}
			}
		}
	})

	t.Run("success writes temp in same dir then renames", func(t *testing.T) {
		var calls [][2]string
		renameFn = func(a, b string) error { calls = append(calls, [2]string{a, b}); return oren(a, b) }
		h.mustRun("swap", "b")
		if len(calls) == 0 {
			t.Fatal("rename not used")
		}
		for _, c := range calls {
			if c[0] == c[1] || filepath.Dir(c[0]) != filepath.Dir(c[1]) {
				t.Fatalf("rename %q -> %q not temp-in-same-dir", c[0], c[1])
			}
		}
	})
}

func TestOutputsNeverContainTokens(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("a", 1900000000000)
	pollInterval = 10 * time.Millisecond
	var all strings.Builder
	all.WriteString(h.mustRun("backup"))
	all.WriteString(h.mustRun("capture", "a"))
	all.WriteString(h.mustRun("status"))
	all.WriteString(h.mustRun("swap", "a"))
	all.WriteString(h.mustRun("watch", "1"))
	out := all.String()
	if strings.Contains(out, dummyPrefix) {
		t.Fatalf("token leaked in output:\n%s", out)
	}
	if !strings.Contains(out, "a@example.com") {
		t.Fatalf("expected email in output:\n%s", out)
	}
}

func TestStatusMatchesProfileByEmail(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("a", 1)
	h.mustRun("capture", "a")
	h.setAccount("b", 1)
	h.mustRun("capture", "b")
	h.mustRun("swap", "a")
	out := h.mustRun("status")
	if !strings.Contains(out, "matches profile: a") || strings.Contains(out, "matches profile: b") {
		t.Fatalf("unexpected status:\n%s", out)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("b", 2)
	h.mustRun("capture", "b")
	h.setAccount("a", 1)
	origC, _ := os.ReadFile(h.creds())
	origJ, _ := os.ReadFile(h.cjson())

	out := h.mustRun("backup")
	dir := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "backup: ") {
			dir = strings.TrimSpace(strings.TrimPrefix(l, "backup: "))
		}
	}
	if dir == "" {
		t.Fatalf("no backup dir in output:\n%s", out)
	}
	h.mustRun("swap", "b")
	if c, _ := os.ReadFile(h.creds()); bytes.Equal(c, origC) {
		t.Fatal("swap did not change credentials")
	}
	h.mustRun("restore", dir)
	gotC, _ := os.ReadFile(h.creds())
	gotJ, _ := os.ReadFile(h.cjson())
	if !bytes.Equal(gotC, origC) || !bytes.Equal(gotJ, origJ) {
		t.Fatal("restore did not round-trip byte-for-byte")
	}
}

func TestWatchDetectsRewrite(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("a", 1)
	h.mustRun("capture", "a")
	h.setAccount("b", 2)
	h.mustRun("capture", "b")
	h.mustRun("swap", "b")

	pollInterval = 20 * time.Millisecond
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(150 * time.Millisecond)
		h.setAccount("a", 1) // simulate running session writing old account back
	}()
	out := h.mustRun("watch", "1")
	wg.Wait()
	for _, want := range []string{"WRITE-BACK", "claudeAiOauth", "oauthAccount", "a@example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("watch output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, dummyPrefix) {
		t.Fatal("token leaked")
	}
}

func TestWatchQuietWhenUnchanged(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("b", 2)
	h.mustRun("capture", "b")
	h.mustRun("swap", "b")
	pollInterval = 10 * time.Millisecond
	out := h.mustRun("watch", "1")
	if strings.Contains(out, "WRITE-BACK") || !strings.Contains(out, "no write-back") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestInvalidInput(t *testing.T) {
	h := newFakeHome(t)
	h.setAccount("a", 1)
	tests := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"bogus"}},
		{"no command", nil},
		{"capture without name", []string{"capture"}},
		{"path traversal name", []string{"capture", "../evil"}},
		{"swap missing profile", []string{"swap", "nope"}},
		{"restore missing dir", []string{"restore", filepath.Join(h.home, "nodir")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := h.run(tt.args...); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestMacOSNotSupported(t *testing.T) {
	h := newFakeHome(t)
	og := goos
	goos = "darwin"
	t.Cleanup(func() { goos = og })
	_, err := h.run("backup")
	if err == nil || !strings.Contains(err.Error(), "not supported in spike on macOS") {
		t.Fatalf("got %v", err)
	}
}

func TestClaudeConfigDirHonored(t *testing.T) {
	h := newFakeHome(t)
	cfg := filepath.Join(h.home, "custom")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	h.write(filepath.Join(cfg, ".credentials.json"), map[string]any{"claudeAiOauth": map[string]any{"accessToken": dummyPrefix + "X", "expiresAt": 1}})
	h.write(filepath.Join(cfg, ".claude.json"), map[string]any{"oauthAccount": map[string]any{"emailAddress": "c@example.com"}})
	out := h.mustRun("capture", "c")
	if !strings.Contains(out, "c@example.com") {
		t.Fatalf("custom config dir not used:\n%s", out)
	}
}
