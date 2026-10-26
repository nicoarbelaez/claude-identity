package login

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
)

func TestArgs(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{"default", Options{}, []string{"auth", "login"}},
		{"console", Options{Console: true}, []string{"auth", "login", "--console"}},
		{"sso", Options{SSO: true}, []string{"auth", "login", "--sso"}},
		{"email", Options{Email: "a@example.com"}, []string{"auth", "login", "--email", "a@example.com"}},
		{"all", Options{Console: true, SSO: true, Email: "a@example.com"},
			[]string{"auth", "login", "--console", "--sso", "--email", "a@example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Args(tt.opts); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Args = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvSetsConfigDirAndReplacesExisting(t *testing.T) {
	base := []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/home/me/.claude", "HOME=/home/me"}
	got := Env(base, "/tmp/x")
	var dirs []string
	for _, kv := range got {
		if strings.HasPrefix(strings.ToUpper(kv), "CLAUDE_CONFIG_DIR=") {
			dirs = append(dirs, kv)
		}
	}
	if len(dirs) != 1 || dirs[0] != "CLAUDE_CONFIG_DIR=/tmp/x" {
		t.Fatalf("CLAUDE_CONFIG_DIR entries = %v", dirs)
	}
	if len(got) != 3 {
		t.Fatalf("other variables were not preserved: %v", got)
	}
	if base[1] != "CLAUDE_CONFIG_DIR=/home/me/.claude" {
		t.Fatal("Env mutated its input")
	}
}

func TestExecRunnerReportsMissingClaude(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := ExecRunner{}.Login(context.Background(), t.TempDir(), Options{})
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestFakeRunnerRecordsCallsAndRunsFn(t *testing.T) {
	dir := t.TempDir()
	f := &FakeRunner{Fn: func(configDir string, _ Options) error {
		return os.WriteFile(filepath.Join(configDir, "marker"), []byte("x"), 0o600)
	}}
	if err := f.Login(context.Background(), dir, Options{Console: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 1 || f.Calls[0].ConfigDir != dir || !f.Calls[0].Opts.Console {
		t.Fatalf("calls = %+v", f.Calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Fatal(err)
	}
}

func TestPathsInMatchesClaudeCodeLayout(t *testing.T) {
	dir := filepath.Join("x", "cfg")
	got := claudecfg.PathsIn(dir)
	if got.Credentials != filepath.Join(dir, ".credentials.json") || got.ClaudeJSON != filepath.Join(dir, ".claude.json") {
		t.Fatalf("PathsIn = %+v", got)
	}
}
