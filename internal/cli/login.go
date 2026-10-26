package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"runtime"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
)

// extractFlags removes the named boolean flags (as -name or --name) from args
// wherever they appear, since the flag package stops parsing at the first
// positional argument. It returns which flags were set and the remaining args.
func extractFlags(args []string, names ...string) (set map[string]bool, rest []string) {
	set = make(map[string]bool, len(names))
	rest = make([]string, 0, len(args))
outer:
	for _, arg := range args {
		for _, n := range names {
			if arg == "--"+n || arg == "-"+n {
				set[n] = true
				continue outer
			}
		}
		rest = append(rest, arg)
	}
	return set, rest
}

func (a *app) goos() string {
	if a.GOOS != "" {
		return a.GOOS
	}
	return runtime.GOOS
}

// addCurrent captures the login that is live in Claude Code and marks it active.
func (a *app) addCurrent(name string, force bool) int {
	res, err := a.Swap.Add(name, force)
	if err != nil {
		return a.fail(err, name)
	}
	a.printAdded(res)
	return exitOK
}

func (a *app) printAdded(res swap.AddResult) {
	msg := i18n.ProfileAdded
	if res.Replaced {
		msg = i18n.ProfileReplaced
	}
	a.printf(a.Stdout, "%s\n", a.t(msg, res.Profile.Name))
}

// addWithLogin runs Claude Code's own login in an isolated config dir and
// saves the result as profile name without touching the live login.
func (a *app) addWithLogin(name string, force bool, opts login.Options) int {
	if !profile.ValidName(name) {
		return a.fail(profile.ErrInvalidName, name)
	}
	return a.withIsolatedLogin(opts, name, a.t(i18n.AddFallback, name), func(src swap.Source) int {
		res, err := a.Swap.AddFrom(name, src, force)
		if err != nil {
			return a.fail(err, name)
		}
		a.printAdded(res)
		a.printf(a.Stdout, "%s\n", a.t(i18n.LoginNotSwitched, res.Profile.Name))
		return exitOK
	})
}

// withIsolatedLogin creates a private temp config dir, runs the login in it,
// and calls save with a source reading the new credentials. The dir is always
// removed. Every failure to obtain a login prints a localized one-line cause
// plus one short retry sentence and exits 1. name is the profile being saved.
func (a *app) withIsolatedLogin(opts login.Options, name, retry string, save func(swap.Source) int) int {
	failWith := func(msg string) int {
		a.printf(a.Stderr, "%s\n%s\n", msg, retry)
		return exitRuntime
	}
	if a.goos() == "darwin" {
		// The macOS Keychain item for a non-default config dir is not known yet;
		// the message already says what to do instead, so no retry sentence.
		a.printf(a.Stderr, "%s\n", a.t(i18n.LoginUnsupportedOS, name))
		return exitRuntime
	}
	if a.Login == nil {
		// A wiring bug in main, not a missing `claude`.
		return failWith(a.t(i18n.LoginRunnerMissing))
	}

	// MkdirTemp creates the directory with mode 0700, so no chmod is needed.
	dir, err := os.MkdirTemp("", "cid-login-")
	if err != nil {
		return a.fail(err, "")
	}
	defer func() {
		removeAll := a.RemoveAll
		if removeAll == nil {
			removeAll = os.RemoveAll
		}
		if err := removeAll(dir); err != nil {
			a.printf(a.Stderr, "%s\n", a.t(i18n.LoginTempDirLeft, dir))
		}
	}()

	// Ctrl+C cancels the login (killing claude) instead of terminating cid, so
	// the deferred cleanup above still runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if !a.Quiet {
		a.printf(a.Stdout, "%s\n", a.t(i18n.LoginStarting))
	}
	if err := a.Login.Login(ctx, dir, opts); err != nil {
		if errors.Is(err, login.ErrNotInstalled) {
			return failWith(a.t(i18n.LoginClaudeMissing))
		}
		if errors.Is(err, login.ErrTimeout) {
			return failWith(a.t(i18n.LoginTimedOut))
		}
		return failWith(a.t(i18n.LoginFailed, err.Error()))
	}

	src := claudecfg.NewFileStore(claudecfg.PathsIn(dir))
	if _, err := src.Read(); errors.Is(err, claudecfg.ErrNoCredentials) {
		return failWith(a.t(i18n.LoginNoCredentials))
	}
	return save(src)
}
