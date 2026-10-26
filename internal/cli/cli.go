// Package cli is the command-line adapter of cid. It parses arguments, calls
// the profile store and swap service through small ports, and prints
// localized messages. It never handles token values.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/credentials"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
	"github.com/nicoarbelaez/claude-identity/internal/version"
)

// Exit codes.
const (
	exitOK      = 0
	exitRuntime = 1
	exitUsage   = 2
)

// ProfileReader is the read side of the profile store.
type ProfileReader interface {
	List() ([]profile.Profile, error)
	Active() (profile.Profile, error)
	Resolve(arg string) (profile.Profile, error)
}

// Swapper is the write side: operations that touch live credentials, the
// vault, and profiles.json under the cross-process lock.
type Swapper interface {
	Switch(arg string) (swap.Result, error)
	Add(name string, force bool) (swap.AddResult, error)
	AddFrom(name string, src swap.Source, force bool) (swap.AddResult, error)
	Relogin(arg string, src swap.Source) (swap.ReloginResult, error)
	Remove(arg string) (profile.Profile, error)
	Status(name string, active bool) credentials.Status
}

// Config describes where cid and Claude Code keep their data (for `cid config`).
type Config struct {
	ProfilesPath string
	ClaudeDir    string
	// Keychain is true when Claude Code credentials live in the macOS Keychain.
	Keychain bool
}

// Deps are the injected collaborators of Run.
type Deps struct {
	Profiles ProfileReader
	Swap     Swapper
	Lang     i18n.Lang
	Config   Config
	// Uninstall holds the collaborators of `cid uninstall`.
	Uninstall UninstallDeps
	Stdin     io.Reader // read only by the internal `hook` command
	Stdout    io.Writer
	Stderr    io.Writer
	// Login runs Claude Code's login in an isolated config dir (cid add/relogin).
	Login login.Runner
	// GOOS is the operating system; empty means runtime.GOOS. Tests set it.
	GOOS string
}

// Run executes one cid invocation and returns the process exit code:
// 0 success, 1 runtime error, 2 usage error.
func Run(d Deps, args []string) int {
	a := &app{Deps: d}
	if len(args) == 0 {
		return a.list(nil)
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		return a.list(rest)
	case "switch":
		return a.switchTo(rest)
	case "add":
		return a.add(rest)
	case "remove":
		return a.remove(rest)
	case "relogin":
		return a.relogin(rest)
	case "current":
		return a.current(rest)
	case "config":
		return a.config(rest)
	case "uninstall":
		return a.uninstall(rest)
	case "version":
		return a.version(rest)
	case "hook": // internal: plugin UserPromptExpansion hook, not in the usage text
		return a.hook()
	case "help", "-h", "--help":
		a.printf(a.Stdout, "%s\n", a.t(i18n.Usage))
		return exitOK
	}
	return a.bare(args)
}

type app struct{ Deps }

func (a *app) t(key i18n.Key, args ...any) string { return i18n.T(a.Lang, key, args...) }

func (a *app) printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func (a *app) usageError(msg string) int {
	if msg != "" {
		a.printf(a.Stderr, "%s\n", msg)
	}
	a.printf(a.Stderr, "%s\n", a.t(i18n.Usage))
	return exitUsage
}

// bare handles `cid <n|name>`: a profile reference acts as a switch, anything
// else is an unknown command.
func (a *app) bare(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return a.usageError(a.t(i18n.UnknownCommand, args[0]))
	}
	if _, err := a.Profiles.Resolve(args[0]); err != nil {
		if errors.Is(err, profile.ErrNotFound) {
			return a.usageError(a.t(i18n.UnknownCommand, args[0]))
		}
		return a.fail(err, args[0])
	}
	return a.doSwitch(args[0])
}

// parse parses flags for a command that accepts only --json (when withJSON)
// and the given number of positional arguments.
func parse(name string, args []string, withJSON bool, positional int) (asJSON bool, pos []string, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if withJSON {
		fs.BoolVar(&asJSON, "json", false, "")
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != positional {
		return false, nil, false
	}
	return asJSON, fs.Args(), true
}

func (a *app) list(args []string) int {
	asJSON, _, ok := parse("list", args, true, 0)
	if !ok {
		return a.usageError("")
	}
	ps, err := a.Profiles.List()
	if err != nil {
		return a.fail(err, "")
	}
	activeName, err := a.activeName()
	if err != nil {
		return a.fail(err, "")
	}
	if asJSON {
		out := make([]jsonProfile, 0, len(ps))
		for _, p := range ps {
			active := p.Name == activeName
			out = append(out, toJSON(p, active, a.Swap.Status(p.Name, active)))
		}
		return a.writeJSON(out)
	}
	if len(ps) == 0 {
		a.printf(a.Stdout, "%s\n", a.t(i18n.ListEmpty))
		return exitOK
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ProfileListHeader))
	for _, p := range ps {
		marker := " "
		active := p.Name == activeName
		if active {
			marker = "*"
		}
		status := a.statusLabel(a.Swap.Status(p.Name, active))
		line := fmt.Sprintf("%s %d  %s  %s  %s", marker, p.Index, p.Name, p.Email, status)
		a.printf(a.Stdout, "%s\n", strings.TrimRight(line, " "))
	}
	return exitOK
}

// statusLabel returns the localized, short label for a token status.
func (a *app) statusLabel(s credentials.Status) string {
	switch s {
	case credentials.StatusOK:
		return a.t(i18n.StatusOK)
	case credentials.StatusAccessExpired:
		return a.t(i18n.StatusAccessExpired)
	case credentials.StatusNeedsLogin:
		return a.t(i18n.StatusNeedsLogin)
	}
	return a.t(i18n.StatusUnknown)
}

// activeName returns the active profile name, or "" when none is recorded.
// Any error other than profile.ErrNoActive is returned to the caller.
func (a *app) activeName() (string, error) {
	p, err := a.Profiles.Active()
	if errors.Is(err, profile.ErrNoActive) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return p.Name, nil
}

func (a *app) current(args []string) int {
	asJSON, _, ok := parse("current", args, true, 0)
	if !ok {
		return a.usageError("")
	}
	p, err := a.Profiles.Active()
	if err != nil && !errors.Is(err, profile.ErrNoActive) {
		return a.fail(err, "")
	}
	none := err != nil
	if asJSON {
		if none {
			return a.writeJSON(nil)
		}
		return a.writeJSON(toJSON(p, true, a.Swap.Status(p.Name, true)))
	}
	if none {
		a.printf(a.Stdout, "%s\n", a.t(i18n.CurrentNone))
		return exitOK
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.CurrentActive, p.Name, p.Email))
	a.printf(a.Stdout, "%s\n", a.t(i18n.CurrentStatus, a.statusLabel(a.Swap.Status(p.Name, true))))
	return exitOK
}

// relogin refreshes a profile's login by running Claude Code's login in an
// isolated config dir for the profile's account. If the login cannot run, it
// prints the manual steps instead.
func (a *app) relogin(args []string) int {
	flags, rest := extractFlags(args, "console", "sso")
	_, pos, ok := parse("relogin", rest, false, 1)
	if !ok {
		return a.usageError("")
	}
	p, err := a.Profiles.Resolve(pos[0])
	if err != nil {
		return a.fail(err, pos[0])
	}
	who := p.Email
	if who == "" {
		who = p.Name
	}
	opts := login.Options{Console: flags["console"], SSO: flags["sso"], Email: p.Email}
	return a.withIsolatedLogin(opts, a.t(i18n.ReloginSteps, who, p.Name), func(src swap.Source) int {
		res, err := a.Swap.Relogin(p.Name, src)
		if err != nil {
			return a.fail(err, p.Name)
		}
		a.printf(a.Stdout, "%s\n", a.t(i18n.ReloginDone, res.Profile.Name))
		if res.LiveUpdated {
			a.printf(a.Stdout, "%s\n", a.t(i18n.ReloginLiveUpdated))
		}
		return exitOK
	})
}

func (a *app) switchTo(args []string) int {
	_, pos, ok := parse("switch", args, false, 1)
	if !ok {
		return a.usageError("")
	}
	return a.doSwitch(pos[0])
}

func (a *app) doSwitch(arg string) int {
	res, err := a.Swap.Switch(arg)
	if err != nil {
		return a.fail(err, arg)
	}
	if res.NoOp {
		a.printf(a.Stdout, "%s\n", a.t(i18n.AlreadyActive, res.To.Name))
		return exitOK
	}
	if res.From.Name != "" {
		a.printf(a.Stdout, "%s\n", a.t(i18n.SwitchedFromTo, res.From.Name, res.To.Name))
	} else {
		a.printf(a.Stdout, "%s\n", a.t(i18n.SwitchedTo, res.To.Name))
	}
	if res.RemoteControlWarning {
		a.printf(a.Stdout, "%s\n", a.t(i18n.RemoteControlWarn))
	}
	return exitOK
}

// add saves an account as a profile. By default it runs Claude Code's login in
// an isolated config dir; with --current it captures the live login instead.
func (a *app) add(args []string) int {
	// Flags may follow the name, which the flag package would not parse.
	flags, rest := extractFlags(args, "force", "current", "console", "sso")
	if len(rest) == 0 {
		return a.usageError(a.t(i18n.MissingName))
	}
	_, pos, ok := parse("add", rest, false, 1)
	if !ok {
		return a.usageError("")
	}
	if flags["current"] {
		if flags["console"] || flags["sso"] {
			return a.usageError("")
		}
		return a.addCurrent(pos[0], flags["force"])
	}
	return a.addWithLogin(pos[0], flags["force"], login.Options{Console: flags["console"], SSO: flags["sso"]})
}

func (a *app) remove(args []string) int {
	_, pos, ok := parse("remove", args, false, 1)
	if !ok {
		return a.usageError("")
	}
	p, err := a.Swap.Remove(pos[0])
	if err != nil {
		ref := pos[0]
		if errors.Is(err, swap.ErrRemoveActive) && p.Name != "" {
			ref = p.Name
		}
		return a.fail(err, ref)
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ProfileRemoved, p.Name))
	return exitOK
}

func (a *app) config(args []string) int {
	if _, _, ok := parse("config", args, false, 0); !ok {
		return a.usageError("")
	}
	backend := a.t(i18n.BackendFile)
	if a.Config.Keychain {
		backend = a.t(i18n.BackendKeychain)
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigProfilesPath, a.Config.ProfilesPath))
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigClaudeDir, a.Config.ClaudeDir))
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigBackend, backend))
	a.printf(a.Stdout, "%s\n", a.t(i18n.ConfigLanguage, string(a.Lang)))
	return exitOK
}

func (a *app) version(args []string) int {
	if _, _, ok := parse("version", args, false, 0); !ok {
		return a.usageError("")
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.VersionLine, version.String()))
	return exitOK
}

// jsonProfile is the stable machine-readable profile shape. It never carries
// credentials.
type jsonProfile struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Active bool   `json:"active"`
	Status string `json:"status"`
}

func toJSON(p profile.Profile, active bool, status credentials.Status) jsonProfile {
	return jsonProfile{Index: p.Index, Name: p.Name, Email: p.Email, Active: active, Status: string(status)}
}

func (a *app) writeJSON(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return a.fail(err, "")
	}
	a.printf(a.Stdout, "%s\n", b)
	return exitOK
}

// fail prints a localized, payload-free message for err and returns exit 1.
// arg is the profile reference the user typed, when relevant.
func (a *app) fail(err error, arg string) int {
	var msg string
	var exists *swap.ProfileExistsError
	var dup *swap.DuplicateAccountError
	var mismatch *swap.EmailMismatchError
	switch {
	case errors.As(err, &mismatch):
		msg = a.t(i18n.ReloginEmailMismatch, mismatch.Got, mismatch.Name, mismatch.Want, mismatch.Name)
	case errors.As(err, &exists):
		msg = a.t(i18n.ProfileExists, exists.Name, exists.Email)
	case errors.As(err, &dup):
		msg = a.t(i18n.AccountSaved, dup.Name)
	case errors.Is(err, profile.ErrNotFound):
		msg = a.t(i18n.UnknownProfile, arg)
	case errors.Is(err, profile.ErrInvalidName):
		msg = a.t(i18n.InvalidName)
	case errors.Is(err, swap.ErrRemoveActive):
		msg = a.t(i18n.RemoveActive, arg)
	case errors.Is(err, swap.ErrUnknownLiveAccount):
		msg = a.t(i18n.LiveAccountUnknown)
	case errors.Is(err, swap.ErrLockTimeout):
		msg = a.t(i18n.LockTimeout)
	case errors.Is(err, claudecfg.ErrNoCredentials):
		msg = a.t(i18n.NoLiveLogin)
	default:
		msg = a.t(i18n.GenericError, err.Error())
	}
	a.printf(a.Stderr, "%s\n", msg)
	return exitRuntime
}
