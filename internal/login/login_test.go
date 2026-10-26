package login

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

// helperEnv switches the test binary into a fake `claude` (see TestMain).
const helperEnv = "CID_LOGIN_TEST_HELPER"

// TestMain lets the test binary stand in for `claude`: ExecRunner re-executes
// it with helperEnv set, so no real claude is ever invoked.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "ok":
		os.Exit(0)
	case "fail":
		os.Exit(3)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "env":
		os.Stdout.WriteString("dir=" + os.Getenv("CLAUDE_CONFIG_DIR") + " args=" + strings.Join(os.Args[1:], ","))
		os.Exit(0)
	}
	os.Exit(4)
}

// fakeClaude returns an ExecRunner whose "claude" is this test binary.
func fakeClaude(t *testing.T, mode string, out *bytes.Buffer) ExecRunner {
	t.Helper()
	t.Setenv(helperEnv, mode)
	return ExecRunner{command: os.Args[0], Stdin: strings.NewReader(""), Stdout: out, Stderr: out}
}

func TestExecRunnerPassesArgsAndConfigDir(t *testing.T) {
	var out bytes.Buffer
	r := fakeClaude(t, "env", &out)
	if err := r.Login(context.Background(), "/cfg/x", Options{SSO: true}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "dir=/cfg/x args=auth,login,--sso"; got != want {
		t.Fatalf("helper saw %q, want %q", got, want)
	}
}

func TestExecRunnerReturnsErrorOnNonZeroExit(t *testing.T) {
	var out bytes.Buffer
	err := fakeClaude(t, "fail", &out).Login(context.Background(), t.TempDir(), Options{})
	if err == nil || errors.Is(err, ErrNotInstalled) || errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want a plain exit error", err)
	}
}

func TestExecRunnerTimesOut(t *testing.T) {
	var out bytes.Buffer
	r := fakeClaude(t, "hang", &out)
	r.Timeout = 200 * time.Millisecond
	start := time.Now()
	err := r.Login(context.Background(), t.TempDir(), Options{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if time.Since(start) > 30*time.Second {
		t.Error("the hung login was not killed at the deadline")
	}
}

func TestExecRunnerCancelIsNotATimeout(t *testing.T) {
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	err := fakeClaude(t, "hang", &out).Login(ctx, t.TempDir(), Options{})
	if err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want a cancellation error that is not ErrTimeout", err)
	}
}

func TestDefaultTimeoutIsTenMinutes(t *testing.T) {
	if DefaultTimeout != 10*time.Minute {
		t.Fatalf("DefaultTimeout = %v", DefaultTimeout)
	}
}
