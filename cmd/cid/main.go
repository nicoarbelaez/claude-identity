// Command cid switches between Claude Code accounts.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/cli"
	"github.com/nicoarbelaez/claude-identity/internal/fsutil"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
	"github.com/nicoarbelaez/claude-identity/internal/pathenv"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/settings"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
	"github.com/nicoarbelaez/claude-identity/internal/update"
	"github.com/nicoarbelaez/claude-identity/internal/usage"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run wires the real adapters (OS keyring, Claude Code files) and delegates
// to the CLI.
func run(args []string) int {
	profilesPath, err := profile.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T(i18n.Detect(i18n.SystemEnv()), i18n.GenericError, err.Error()))
		return 1
	}
	configPath := filepath.Join(filepath.Dir(profilesPath), "config.json")

	// config.json decides the language, so notices raised while reading it are
	// held back until the language is known.
	lang := i18n.EN
	known := false
	var pending []fsutil.Notice
	printNotice := cli.NoticeWriter(os.Stderr, func() i18n.Lang { return lang })
	notify := func(n fsutil.Notice) {
		if known {
			printNotice(n)
			return
		}
		pending = append(pending, n)
	}
	sett := settings.NewStore(configPath, os.Getenv).WithNotifier(notify)
	lang = resolveLang(sett)
	known = true
	for _, n := range pending {
		printNotice(n)
	}

	deps, err := wire(lang, profilesPath, configPath, sett, notify)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T(lang, i18n.GenericError, err.Error()))
		return 1
	}
	return cli.Run(deps, args)
}

// resolveLang applies the language setting (CID_LANG > config.json > auto).
// Only "auto", or an unreadable store, falls back to the detection chain.
func resolveLang(s *settings.Store) i18n.Lang {
	if v, err := s.Get("language"); err == nil {
		switch v.Value {
		case "en":
			return i18n.EN
		case "es":
			return i18n.ES
		}
	}
	return i18n.Detect(i18n.SystemEnv())
}

func wire(lang i18n.Lang, profilesPath, configPath string, sett *settings.Store, notify func(fsutil.Notice)) (cli.Deps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return cli.Deps{}, fmt.Errorf("locate home dir: %w", err)
	}
	paths := claudecfg.ResolvePaths(home)
	live, err := claudecfg.Open(runtime.GOOS, claudecfg.ExecRunner{}, paths)
	if err != nil {
		return cli.Deps{}, err
	}
	_, keychain := live.(*claudecfg.KeychainStore)
	store := profile.NewStore(profilesPath).WithNotifier(notify)
	lock := filepath.Join(filepath.Dir(profilesPath), "cid.lock")
	kv := vault.NewKeyringVault()
	var updater update.Updater // nil turns off only the update check and cid update
	if gh, err := update.NewGitHubUpdater(); err == nil {
		updater = gh
	}
	exe, _ := os.Executable() // empty on failure: uninstall then skips the binary-specific steps
	return cli.Deps{
		Profiles: store,
		Swap:     swap.New(store, kv, live, lock),
		Usage:    usage.NewStore(usage.PathFor(profilesPath)).WithNotifier(notify),
		Lang:     lang,
		Settings: sett,
		Login:    login.ExecRunner{},
		GOOS:     runtime.GOOS,
		Config: cli.Config{
			ProfilesPath: profilesPath,
			ConfigPath:   configPath,
			ClaudeDir:    filepath.Dir(paths.Credentials),
			Keychain:     keychain,
		},
		Uninstall: cli.UninstallDeps{
			Vault:      kv,
			PathEnv:    pathenv.New(),
			LockPath:   lock,
			Executable: exe,
			GOOS:       runtime.GOOS,
		},
		Update: cli.UpdateDeps{
			Updater: updater,
			Plugin:  update.PluginRefresher{Runner: update.ExecRunner{}},
		},
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, nil
}
