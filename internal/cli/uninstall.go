package cli

import (
	"bufio"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/pathenv"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
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
	// Only a directory that holds nothing but cid was added to PATH by the
	// installer; a shared one (e.g. ~/go/bin) must stay on PATH.
	pathDir := a.installDir()
	if !onlyCid(pathDir) {
		pathDir = ""
	}

	if !*yes {
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPlan, len(ps), cfgDir))
		if u.PathEnv != nil && pathDir != "" {
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPlanPath, pathDir))
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
	vaultFailed := false
	for _, p := range ps {
		err := u.Vault.Delete(p.Name)
		if err != nil && !errors.Is(err, vault.ErrNotFound) {
			fail(p.Name, err)
			vaultFailed = true
			continue
		}
		a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallProfile, p.Name))
	}
	// A status line still wrapped by cid would point at a binary that is going
	// away, so put the previous one back while its saved copy still exists.
	keepStatuslineBackup := false
	if a.Config.ClaudeDir != "" {
		switch out, err := a.restoreStatusline(); {
		case err != nil && errors.Is(err, errBadSettings):
			// Not restorable now: keep the saved copy so the user can restore later.
			if _, statErr := os.Stat(a.backupPath()); statErr == nil {
				keepStatuslineBackup = true
				a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineBackupKept, a.settingsPath(), a.backupPath()))
			}
		case err != nil:
			fail(a.settingsPath(), err)
			keepStatuslineBackup = true
		case err == nil && out == restored:
			a.printf(a.Stdout, "%s\n", a.t(i18n.StatuslineRemoved, a.settingsPath()))
		case err == nil && out == restoreNoBackup:
			a.printf(a.Stderr, "%s\n", a.t(i18n.StatuslineNoBackup))
		}
	}
	// Keep the profiles file (and its backups) while keyring entries remain,
	// so running uninstall again can find and retry them.
	var files []string
	if !vaultFailed {
		files = append(files, fileFamily(a.Config.ProfilesPath)...)
	}
	files = append(files, fileFamily(a.Config.ConfigPath)...)
	if a.Config.ProfilesPath != "" {
		cache := usage.PathFor(a.Config.ProfilesPath)
		files = append(files, fileFamily(cache)...)
		files = append(files, usage.LockFor(cache))
	}
	if !keepStatuslineBackup && a.Config.ProfilesPath != "" {
		files = append(files, a.backupPath())
	}
	files = append(files, u.LockPath)
	for _, path := range files {
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
	if u.PathEnv != nil && pathDir != "" {
		removed, err := u.PathEnv.RemoveDir(pathDir)
		switch {
		case err != nil:
			fail("PATH", err)
		case removed:
			a.printf(a.Stdout, "%s\n", a.t(i18n.UninstallPathRemoved, pathDir))
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

// fileFamily returns path plus its .bak and .corrupt-* siblings (nil for "").
func fileFamily(path string) []string {
	if path == "" {
		return nil
	}
	out := []string{path, fsutil.BackupPath(path)}
	if m, err := filepath.Glob(path + ".corrupt-*"); err == nil {
		out = append(out, m...)
	}
	return out
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

// onlyCid reports whether dir exists and contains only the cid binary.
func onlyCid(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if n := strings.ToLower(e.Name()); n != "cid" && n != "cid.exe" {
			return false
		}
	}
	return true
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
