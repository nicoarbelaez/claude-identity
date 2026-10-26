// Command cid switches between Claude Code accounts.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nicoarbelaez/claude-identity/internal/claudecfg"
	"github.com/nicoarbelaez/claude-identity/internal/cli"
	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/login"
	"github.com/nicoarbelaez/claude-identity/internal/pathenv"
	"github.com/nicoarbelaez/claude-identity/internal/profile"
	"github.com/nicoarbelaez/claude-identity/internal/swap"
	"github.com/nicoarbelaez/claude-identity/internal/vault"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run wires the real adapters (OS keyring, Claude Code files) and delegates
// to the CLI.
func run(args []string) int {
	lang := i18n.Detect(i18n.SystemEnv())
	deps, err := wire(lang)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T(lang, i18n.GenericError, err.Error()))
		return 1
	}
	return cli.Run(deps, args)
}

func wire(lang i18n.Lang) (cli.Deps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return cli.Deps{}, fmt.Errorf("locate home dir: %w", err)
	}
	profilesPath, err := profile.DefaultPath()
	if err != nil {
		return cli.Deps{}, err
	}
	paths := claudecfg.ResolvePaths(home)
	live, err := claudecfg.Open(runtime.GOOS, claudecfg.ExecRunner{}, paths)
	if err != nil {
		return cli.Deps{}, err
	}
	_, keychain := live.(*claudecfg.KeychainStore)
	store := profile.NewStore(profilesPath)
	lock := filepath.Join(filepath.Dir(profilesPath), "cid.lock")
	kv := vault.NewKeyringVault()
	exe, _ := os.Executable() // empty on failure: uninstall then skips the binary-specific steps
	return cli.Deps{
		Profiles: store,
		Swap:     swap.New(store, kv, live, lock),
		Lang:     lang,
		Login:    login.ExecRunner{},
		GOOS:     runtime.GOOS,
		Config: cli.Config{
			ProfilesPath: profilesPath,
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
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, nil
}
