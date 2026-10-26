// Package login starts Claude Code's own CLI login (`claude auth login`) in an
// isolated config directory. cid never implements OAuth or contacts Anthropic:
// it only launches the official login and later reads what Claude Code wrote.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNotInstalled means the `claude` executable was not found on PATH.
var ErrNotInstalled = errors.New("the `claude` command was not found on PATH")

// ErrTimeout means the login did not finish before the deadline.
var ErrTimeout = errors.New("the login timed out")

// DefaultTimeout is how long ExecRunner waits for the interactive browser login
// before killing `claude`. It is generous because the user must finish the
// sign-in in a browser, but bounded so a forgotten login cannot hang cid.
const DefaultTimeout = 10 * time.Minute

// Options selects how `claude auth login` is run.
type Options struct {
	Console bool   // --console: sign in with an Anthropic Console account
	SSO     bool   // --sso: force SSO login
	Email   string // --email: pre-fill the account email (relogin)
}

// Runner runs an interactive Claude Code login that stores its result under
// configDir (via CLAUDE_CONFIG_DIR).
type Runner interface {
	Login(ctx context.Context, configDir string, opts Options) error
}

// Streamer is implemented by runners whose child process streams can be
// redirected. The hook uses it so the login child never writes to the hook's
// stdout, which carries the hook JSON.
type Streamer interface {
	// WithStreams returns a runner that uses the given streams for the login
	// child. The receiver's own configuration is not modified.
	WithStreams(stdin io.Reader, stdout, stderr io.Writer) Runner
}

// Args returns the arguments passed to `claude` for opts.
func Args(opts Options) []string {
	args := []string{"auth", "login"}
	if opts.Console {
		args = append(args, "--console")
	}
	if opts.SSO {
		args = append(args, "--sso")
	}
	if opts.Email != "" {
		args = append(args, "--email", opts.Email)
	}
	return args
}

// Env returns base with CLAUDE_CONFIG_DIR set to configDir, replacing any
// existing value. base is not modified.
func Env(base []string, configDir string) []string {
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(strings.ToUpper(kv), "CLAUDE_CONFIG_DIR=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "CLAUDE_CONFIG_DIR="+configDir)
}

// ExecRunner runs the real `claude` binary. Nil streams default to the
// process terminal so the browser login can prompt the user.
type ExecRunner struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// Timeout bounds the login; zero means DefaultTimeout.
	Timeout time.Duration
	// command overrides the executable name ("claude"); tests only.
	command string
}

// WithStreams returns a copy of r that uses the given streams.
func (r ExecRunner) WithStreams(stdin io.Reader, stdout, stderr io.Writer) Runner {
	r.Stdin, r.Stdout, r.Stderr = stdin, stdout, stderr
	return r
}

// Login runs `claude auth login` with CLAUDE_CONFIG_DIR=configDir and waits
// for it. It returns ErrNotInstalled (wrapped) when `claude` is not on PATH and
// ErrTimeout (wrapped) when the login outlives the timeout.
func (r ExecRunner) Login(ctx context.Context, configDir string, opts Options) error {
	name := r.command
	if name == "" {
		name = "claude"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotInstalled, err)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, Args(opts)...)
	cmd.Env = Env(os.Environ(), configDir)
	// After the deadline kills claude, stop waiting on pipes a leftover child
	// may still hold, so the timeout always returns.
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.Stdin, r.Stdout, r.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("claude auth login: %w after %s", ErrTimeout, timeout)
		}
		return fmt.Errorf("claude auth login: %w", err)
	}
	return nil
}

// Call records one FakeRunner.Login invocation.
type Call struct {
	ConfigDir string
	Opts      Options
}

// FakeRunner is a Runner for tests: it records calls and runs Fn, which
// typically writes fixture credentials into the config dir it receives.
type FakeRunner struct {
	Fn    func(configDir string, opts Options) error
	Calls []Call

	// Noise is written to the redirected Stdout and Stderr during Login, like
	// the progress text of the real login.
	Noise string
	// Streamed reports that WithStreams was applied; InputRead is what the
	// redirected Stdin yielded during Login.
	Streamed  bool
	InputRead string

	stdin          io.Reader
	stdout, stderr io.Writer
}

// WithStreams records the streams and returns f itself, so calls stay visible.
func (f *FakeRunner) WithStreams(stdin io.Reader, stdout, stderr io.Writer) Runner {
	f.Streamed = true
	f.stdin, f.stdout, f.stderr = stdin, stdout, stderr
	return f
}

// Login records the call and returns Fn's result (nil when Fn is nil).
func (f *FakeRunner) Login(_ context.Context, configDir string, opts Options) error {
	f.Calls = append(f.Calls, Call{ConfigDir: configDir, Opts: opts})
	if f.stdin != nil {
		b, _ := io.ReadAll(f.stdin)
		f.InputRead = string(b)
	}
	for _, w := range []io.Writer{f.stdout, f.stderr} {
		if w != nil {
			_, _ = io.WriteString(w, f.Noise)
		}
	}
	if f.Fn == nil {
		return nil
	}
	return f.Fn(configDir, opts)
}
