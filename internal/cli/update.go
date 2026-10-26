package cli

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/nicoarbelaez/claude-identity/internal/i18n"
	"github.com/nicoarbelaez/claude-identity/internal/update"
	"github.com/nicoarbelaez/claude-identity/internal/version"
)

const (
	// versionCheckTimeout bounds the release check in `cid version`.
	versionCheckTimeout = 3 * time.Second
	// updateTimeout bounds each network step of `cid update`.
	updateTimeout = 2 * time.Minute
)

// UpdateDeps are the collaborators of `cid update`.
type UpdateDeps struct {
	Updater update.Updater // finds and installs releases; nil disables update checks
	Plugin  update.Plugin  // refreshes the plugin; nil skips that step
}

// announceUpdate prints one extra line after the version when a newer release
// exists. Dev builds, offline use, rate limits and parse errors print nothing.
func (a *app) announceUpdate() {
	cur := version.String()
	if a.Update.Updater == nil || update.IsDev(cur) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()
	latest, err := a.Update.Updater.Latest(ctx)
	if err != nil || !update.Newer(cur, latest) {
		return
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.UpdateAvailable, latest))
}

// update replaces the cid binary with the latest release, then refreshes the
// account-switch plugin. The exit code reflects only the binary step.
func (a *app) update(args []string) int {
	if _, _, ok := parse("update", args, false, 0); !ok {
		return a.usageError("")
	}
	cur := version.String()
	if update.IsDev(cur) {
		a.printf(a.Stderr, "%s\n", a.t(i18n.UpdateDevBuild))
		return exitRuntime
	}
	if a.Update.Updater == nil {
		a.printf(a.Stderr, "%s\n", a.t(i18n.UpdateFailed, "no updater configured"))
		return exitRuntime
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if code := a.updateBinary(ctx, cur); code != exitOK {
		return code
	}
	if a.Update.Plugin != nil {
		pctx, cancel := context.WithTimeout(ctx, updateTimeout)
		defer cancel()
		if err := a.Update.Plugin.Refresh(pctx); err != nil {
			a.printf(a.Stderr, "%s\n", a.t(i18n.UpdatePluginWarn, reason(err)))
		} else {
			a.printf(a.Stdout, "%s\n", a.t(i18n.UpdatePluginDone))
		}
	}
	return exitOK
}

func (a *app) updateBinary(ctx context.Context, cur string) int {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	latest, err := a.Update.Updater.Latest(ctx)
	if err != nil {
		a.printf(a.Stderr, "%s\n", a.t(i18n.UpdateFailed, reason(err)))
		return exitRuntime
	}
	if !update.Newer(cur, latest) {
		a.printf(a.Stdout, "%s\n", a.t(i18n.UpdateUpToDate, cur))
		return exitOK
	}
	if err := a.Update.Updater.UpdateTo(ctx, latest); err != nil {
		a.printf(a.Stderr, "%s\n", a.t(i18n.UpdateFailed, reason(err)))
		return exitRuntime
	}
	a.printf(a.Stdout, "%s\n", a.t(i18n.UpdateDone, cur, latest))
	return exitOK
}

// reason renders err on one line without a trailing period, so it fits
// inside a sentence.
func reason(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	return strings.TrimRight(s, ".")
}
