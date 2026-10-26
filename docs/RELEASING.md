# Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please-action) and [GoReleaser](https://goreleaser.com). Nobody edits versions or tags by hand.

## How it works

1. Merge Conventional Commits into `main` (squash-merge PR titles must follow the convention).
2. `.github/workflows/release.yml` runs on every push to `main`. release-please opens or updates a single **Release PR** containing the next semver bump, the `CHANGELOG.md` entries, and the new version in `plugin/.claude-plugin/plugin.json`.
3. **To cut a release, merge the Release PR.** release-please then creates the tag `vX.Y.Z` and the GitHub Release.
4. In the same workflow run, the `goreleaser` job checks out that tag, builds six targets (windows/darwin/linux x amd64/arm64), uploads the archives and `checksums.txt` to the existing release (`release.mode: keep-existing` keeps the notes release-please wrote), and attests build provenance for the checksums.

The release and the build share one workflow so the build runs right after the release is created.

## Commit types and version bumps

The project is pre-1.0 (`bump-minor-pre-major: true`), so breaking changes bump the minor version, not the major.

| Commit | Bump before 1.0 | Bump from 1.0 |
| --- | --- | --- |
| `fix:` | patch | patch |
| `feat:` | minor | minor |
| `feat!:` or a `BREAKING CHANGE:` footer | minor | major |
| `docs:`, `chore:`, `ci:`, `test:`, `refactor:` | no release | no release |

The first `feat:` after the initial `0.0.0` baseline produces `0.1.0`.

## Version sources

- Git tag `vX.Y.Z` is the source of truth.
- The `cid` binary gets its version from GoReleaser through `-ldflags -X .../internal/version.Version`; no Go constant is edited.
- `plugin/.claude-plugin/plugin.json` `$.version` is bumped by release-please (`extra-files`), so the plugin version always matches the release.

## How users update

- Binary: re-run the install script (see the README), or `go install github.com/nicoarbelaez/claude-identity/cmd/cid@latest`.
- Plugin: run `/plugin marketplace update nicoarbelaez-tools`, then update the plugin. To receive updates automatically, enable auto-update for the `nicoarbelaez-tools` marketplace in the `/plugin` manager.

## Verifying downloads

The install scripts always verify the archive SHA-256 against `checksums.txt` and abort on mismatch. If the GitHub CLI is installed they also run `gh attestation verify` against this repository; set `CID_VERIFY_ATTESTATION=0` to skip that step. To verify manually:

```sh
gh attestation verify cid_X.Y.Z_linux_amd64.tar.gz --repo nicoarbelaez/claude-identity
```

## Release GitHub App

release-please authenticates with a short-lived token from a dedicated GitHub App, minted by [`actions/create-github-app-token`](https://github.com/actions/create-github-app-token). This keeps the repository setting "Allow GitHub Actions to create and approve pull requests" disabled, and because the Release PR is authored by the App, CI runs on it like on any other pull request.

The App needs only these repository permissions, installed on this repository alone:

- Contents: read and write
- Pull requests: read and write

The workflow reads two values:

| Name | Kind | Value |
| --- | --- | --- |
| `RELEASE_APP_CLIENT_ID` | Actions variable | The App's Client ID |
| `RELEASE_APP_PRIVATE_KEY` | Actions secret | A private key generated for the App (`.pem` contents) |

To rotate the key, generate a new private key in the App settings, update the secret with `gh secret set RELEASE_APP_PRIVATE_KEY < key.pem`, delete the old key in the App settings, and delete the local `.pem` file.
