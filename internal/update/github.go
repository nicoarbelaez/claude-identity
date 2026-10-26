package update

import (
	"context"
	"errors"
	"fmt"

	"github.com/creativeprojects/go-selfupdate"
)

const (
	repoOwner = "nicoarbelaez"
	repoName  = "claude-identity"
	// checksumsFile is the checksum asset GoReleaser attaches to each release.
	checksumsFile = "checksums.txt"
)

// ErrNoRelease means no release has an asset for this platform.
var ErrNoRelease = errors.New("no release found for this platform")

// GitHubUpdater is the Updater backed by GitHub Releases.
type GitHubUpdater struct {
	up   *selfupdate.Updater
	repo selfupdate.Repository
}

// NewGitHubUpdater returns an Updater for the cid repository that validates
// every download against the release checksums.txt. It does not touch the
// network until used.
func NewGitHubUpdater() (*GitHubUpdater, error) {
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, err
	}
	up, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    src,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: checksumsFile},
	})
	if err != nil {
		return nil, err
	}
	return &GitHubUpdater{up: up, repo: selfupdate.NewRepositorySlug(repoOwner, repoName)}, nil
}

// Latest returns the newest stable release version.
func (g *GitHubUpdater) Latest(ctx context.Context) (string, error) {
	rel, found, err := g.up.DetectLatest(ctx, g.repo)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNoRelease
	}
	return rel.Version(), nil
}

// UpdateTo replaces the running executable with the given release.
func (g *GitHubUpdater) UpdateTo(ctx context.Context, version string) error {
	rel, found, err := g.up.DetectVersion(ctx, g.repo, version)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w (version %s)", ErrNoRelease, version)
	}
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return err
	}
	return g.up.UpdateTo(ctx, rel, exe)
}
