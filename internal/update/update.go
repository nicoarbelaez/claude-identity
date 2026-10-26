// Package update checks for new cid releases, replaces the running binary and
// refreshes the account-switch plugin. Network and process access sit behind
// small interfaces so callers can test without either.
package update

import (
	"context"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Updater finds and installs cid releases.
type Updater interface {
	// Latest returns the newest stable release version (without a leading "v").
	Latest(ctx context.Context) (string, error)
	// UpdateTo replaces the running executable with the given release after
	// verifying its checksum. On error the executable is left untouched.
	UpdateTo(ctx context.Context, version string) error
}

// Plugin refreshes the account-switch Claude Code plugin.
type Plugin interface {
	Refresh(ctx context.Context) error
}

// IsDev reports whether v is a development or unknown build, which can neither
// be compared to a release nor replaced.
func IsDev(v string) bool {
	_, err := semver.NewVersion(strings.TrimSpace(v))
	return err != nil
}

// Newer reports whether latest is a higher version than current. Anything that
// is not a valid version yields false.
func Newer(current, latest string) bool {
	c, err := semver.NewVersion(strings.TrimSpace(current))
	if err != nil {
		return false
	}
	l, err := semver.NewVersion(strings.TrimSpace(latest))
	if err != nil {
		return false
	}
	return l.GreaterThan(c)
}
