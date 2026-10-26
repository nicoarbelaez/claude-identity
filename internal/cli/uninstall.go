package cli

import (
	"bufio"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/pathenv"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

// UninstallDeps are the collaborators `cid uninstall` needs beyond the
// profile store. None of them can reach Claude Code's live credentials.
type UninstallDeps struct {
	Vault      vault.Vault
	PathEnv    pathenv.Editor // nil when there is no persistent PATH to edit
	LockPath   string
	Executable string // path of the running binary
	GOOS       string // selects the delete command hint
}

// uninstall removes everything cid created: stored credentials of every
// profile, the profiles and lock files, and (on Windows) the user PATH entry.
// It never touches Claude Code's own live credentials. Steps are independent:
// a failure is reported and the rest still run, then the exit code is 1.
func (a *app) uninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return a.usageError("")
	}
	u := a.Uninstall

	// A broken profiles file must not block the rest of the cleanup.
	ps, listErr := a.Profiles.List()
	cfgDir := filepath.Dir(a.Config.ProfilesPath)
	binDir := a.installDir()

	if !*yes {
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPlan, len(ps), cfgDir))
		if u.PathEnv != nil && binDir != "" {
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPlanPath, binDir))
		}
		if !a.confirm() {
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallAborted))
			return exitOK
		}
	}

	failed := false
	fail := func(what string, err error) {
		failed = true
		a.printf(a.Stderr, "%s\n", a.t(i18n.UninstallStepFailed, what, err.Error()))
	}
	if listErr != nil {
		fail(a.Config.ProfilesPath, listErr)
	}
	for _, p := range ps {
		err := u.Vault.Delete(p.Name)
		if err != nil && !errors.Is(err, vault.ErrNotFound) {
			fail(p.Name, err)
			continue
		}
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallProfile, p.Name))
	}
	for _, path := range []string{a.Config.ProfilesPath, u.LockPath} {
		if path == "" {
			continue
		}
		switch err := os.Remove(path); {
		case err == nil:
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallFileRemoved, path))
		case !errors.Is(err, os.ErrNotExist):
			fail(path, err)
		}
	}
	if empty, err := isEmptyDir(cfgDir); err == nil && empty {
		if err := os.Remove(cfgDir); err != nil {
			fail(cfgDir, err)
		} else {
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallFileRemoved, cfgDir))
		}
	}
	if u.PathEnv != nil && binDir != "" {
		removed, err := u.PathEnv.RemoveDir(binDir)
		switch {
		case err != nil:
			fail("PATH", err)
		case removed:
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPathRemoved, binDir))
		}
	}

	if failed {
		a.printf(a.Stderr, "%s\n", a.t(i18n.UninstallIncomplete))
	} else {
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallDone))
	}
	if u.Executable != "" {
		hint := a.t(i18n.UninstallDeleteCmd, deleteCommand(u.GOOS, u.Executable))
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallManual, u.Executable, hint))
	}
	if failed {
		return exitRuntime
	}
	return exitOK
}

// installDir is the resolved directory of the running binary, or "".
func (a *app) installDir() string {
	exe := a.Uninstall.Executable
	if exe == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// confirm reads one line from Stdin; only an explicit yes (English or
// Spanish) confirms.
func (a *app) confirm() bool {
	a.printf(a.Stdout, "%s", a.t(i18n.UninstallPrompt))
	if a.Stdin == nil {
		return false
	}
	line, _ := bufio.NewReader(a.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "s", "si", "sí":
		return true
	}
	return false
}

func deleteCommand(goos, path string) string {
	if goos == "windows" {
		return `Remove-Item "` + path + `"`
	}
	return `rm "` + path + `"`
}

func isEmptyDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
