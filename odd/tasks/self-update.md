# self-update

## Objective
`cid version` tells the user when a newer release exists, and `cid update` updates both the CLI binary and the `account-switch` plugin in one command.

## Problem
Upgrading means re-running the install script and reinstalling the plugin by hand.

## Why
Updating should be one command, with no reinstall and no extra setup.

## Scope
In:
- Latest-release check in `cid version`.
- `cid update`: verified binary swap via `github.com/creativeprojects/go-selfupdate`, then a plugin refresh through the `claude plugin` CLI.
- i18n EN/ES, README and SKILL.md updates.
- The hook rejects `update`.

Out:
- A new config key or env var.
- A background or cached update notifier.
- A hook notice.
- Package managers.
- Attestation verification inside the binary. The checksum is verified; `gh attestation verify` stays in the install scripts.

## Constraints
- No new configuration. The check runs only when the user runs `cid version`. Timeout: 3 s.
- Network failure, rate limit or a dev build: print the version line only, with no error and exit 0.
- Reuse the existing architecture: `Deps` injection in `internal/cli`, the `internal/version` package, the i18n catalog and the `login.Runner`-style exec (no shell).
- Checksum validation against the release `checksums.txt`. Asset names follow the GoReleaser pattern `cid_<ver>_<os>_<arch>.{tar.gz,zip}`.
- Plugin refresh:
  - Runs `claude plugin marketplace update nicoarbelaez-tools`, then `claude plugin update account-switch@nicoarbelaez-tools`.
  - If `claude` is missing or a step fails: one stderr warning. The binary update still counts as a success.
- Messages are one line each, with no numbered steps (`TestNoMessageHasNumberedSteps`).
- Tests never hit the network or replace the real test binary; they use fakes.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| U1 | [x] | `cid version` shows "newer version available" when a newer release exists | bounded writer | c3fa219, 73d90e7, 2c02429, e160b54 |
| U2 | [x] | `cid update` replaces the binary after checksum validation and reports old→new or "already up to date" | bounded writer | c3fa219, 73d90e7, 2c02429, e160b54 |
| U3 | [x] | `cid update` refreshes the marketplace and plugin via the `claude` CLI; the hook rejects `update` | bounded writer | c3fa219, 73d90e7, 2c02429, e160b54 |
| U4 | [x] | Docs: README (Update section, usage table), help text, SKILL.md | bounded writer | 2c02429, e75ddba |

Route evidence: 2+ non-trivial files (new package, CLI, i18n, main wiring, docs) trigger the writer.

## Acceptance criteria
- `cid version` on an older build prints the version line and one line naming the new version and `cid update`.
- `cid version` offline prints only the version line, exit 0.
- `cid update` on the latest version says it is up to date and still refreshes the plugin.
- A checksum mismatch aborts with one line, and the binary is left unchanged.
- `/account update` is rejected by the hook.

## Checks
`go build ./...` (also GOOS=linux and darwin), `go vet ./...`, `go test ./...`, `gofmt -l .`, `go mod tidy` with no diff, `go mod verify`. Test-first where a deterministic test exists.

## Delivery
Strategy: `ask-on-risk`. Forecast is about 400 authored lines, so one PR.

## Progress
- Branch `feat/self-update` created from `2e16b99`.
- U1-U3: `internal/update` (Updater/Plugin/CommandRunner interfaces, GitHub updater with `checksums.txt` validation, `claude plugin` refresh), `cid update`, release notice in `cid version`, hook rejects `update`. Tests use fakes only. RED observed first (undefined symbols) before implementing.
- U4: README "Updating" section and usage table row; usage text updated in EN and ES. SKILL.md unchanged: its hint lists no terminal-only commands (`update` is rejected by the hook like `uninstall`).
- Checks: build (host, linux, darwin), vet, test, gofmt, `go mod tidy` with no diff, `go mod verify` all pass.
