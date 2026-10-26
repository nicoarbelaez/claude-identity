package update

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrClaudeMissing means the `claude` executable was not found on PATH.
var ErrClaudeMissing = errors.New("the `claude` command was not found on PATH")

const (
	marketplace = "nicoarbelaez-tools"
	pluginName  = "account-switch@" + marketplace
	// reasonMax bounds the command output quoted in an error.
	reasonMax = 200
)

// CommandRunner runs an executable without a shell and returns its combined
// output.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner is the CommandRunner backed by os/exec.
type ExecRunner struct{}

// Run executes name with args, capturing stdout and stderr together. It
// returns ErrClaudeMissing (wrapped) when name is not on PATH.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrClaudeMissing, err)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	// After the deadline kills the child, stop waiting on pipes a leftover
	// grandchild may still hold.
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// PluginRefresher updates the account-switch plugin through the `claude`
// CLI: first the marketplace listing, then the plugin itself.
type PluginRefresher struct {
	Runner CommandRunner
}

// Refresh runs `claude plugin marketplace update` and `claude plugin update`.
// The first failing step stops it; the error is a single line.
func (p PluginRefresher) Refresh(ctx context.Context) error {
	steps := [][]string{
		{"plugin", "marketplace", "update", marketplace},
		{"plugin", "update", pluginName},
	}
	for _, args := range steps {
		out, err := p.Runner.Run(ctx, "claude", args...)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrClaudeMissing) {
			return err
		}
		if reason := oneLine(out); reason != "" {
			return fmt.Errorf("claude %s: %w: %s", strings.Join(args[:2], " "), err, reason)
		}
		return fmt.Errorf("claude %s: %w", strings.Join(args[:2], " "), err)
	}
	return nil
}

// oneLine returns the first non-empty line of s, truncated to reasonMax.
func oneLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > reasonMax {
			line = string(r[:reasonMax])
		}
		return line
	}
	return ""
}
