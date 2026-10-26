# claude-identity-v1

## Objective
Ship `claude-identity` (CLI `cid` plus Claude Code plugin) so users can hot-swap Anthropic accounts inside one running Claude Code session without losing context.

## Problem
Claude Code holds one logged-in account. Switching requires logout/login and often a new session, losing context.

## Why
Users with personal and work (or several paid) accounts need fast, safe switching. A single `~/.claude` plus an OS-keyring vault keeps history, settings and sessions shared.

## Scope
In: profile store, keyring vault, swap engine, CLI, `/account` plugin commands, expiry guidance, i18n (en/es), releases, repo security.
Out: calling Anthropic OAuth endpoints, token refresh by us, plaintext token storage.

## Constraints
Go; Windows, macOS, Linux; MIT; public English repo `claude-identity`; free GitHub plan; no secrets in repo; never send tokens anywhere.

## Requirements
- R1 Hot swap in a single `~/.claude`, no context loss.
- R2 Spike: verify a running session re-reads credentials changed by another process (first task, blocks design).
- R3 Last active account persists naturally.
- R4 Commands `/account`, `/account <n|name>`, `/account config`, `/account add <name>`, short alias, CLI `cid`. Plugin commands are namespaced (`/account-switch:account`); `cid setup` installs a user-level `/account` alias in `~/.claude/commands/`.
- R14 Plugin and marketplace names must not start with `claude-` (reserved by Claude Code). Plugin `account-switch`, marketplace `nicoarbelaez-tools`; repository name stays `claude-identity`.
- R5 Expiry: Claude Code refreshes; on failure guided `/login` plus automatic recapture.
- R6 OS keyring vault, no plaintext.
- R7 Go.
- R8 Windows + macOS + Linux.
- R9 i18n en/es (Claude Code settings, fallback OS locale).
- R10 Plugin marketplace plus signed GitHub Releases binary; install script or `go install`.
- R11 Free GitHub Actions CI/CD with TDD, 3-OS tests, lint, GoReleaser.
- R12 Strict repo security on free plan: ruleset (PR required, CODEOWNERS approval, green CI, signed commits, no force-push/delete); CodeQL; Dependabot; secret scanning + push protection; SHA-pinned actions; least-privilege GITHUB_TOKEN; manual approval for fork workflows.
- R13 Public repo `claude-identity`, English, MIT.

## Checklist
| ID | Status | Task | Reqs | Route | Commit |
|----|--------|------|------|-------|--------|
| T0 | [x] done | Scaffold, CI, release pipeline, plugin skeleton, repo security | R7 R10 R11 R12 R13 | delegated direct (2+ non-trivial files) | e30c252 |
| T0.1 | [x] done | Rename plugin/marketplace to pass `claude plugin validate` | R14 | inline (mechanical) | 75b96f2 (#1) |
| T1 | [x] done | Spike: does a running session re-read externally changed credentials? Blocks design | R2 | delegated direct (kit) + user-run experiment | 200dbf5 (#2), results in this PR |
| T2 | [x] done | Profile store + keyring vault (TDD) | R3 R6 | delegated direct | d34ae6a (#3) |
| T8a | [x] done | Release automation (release-please + GoReleaser) and verified install scripts | R10 R11 | delegated direct + independent verifier | #4 |
| T3 | [x] done | Swap engine: Windows/Linux file + macOS Keychain adapter | R1 R8 | delegated direct | 307f70b (#7) |
| T4 | [x] done | CLI commands | R4 | delegated direct (writer trigger: 2+ non-trivial files) | 1ee6c3a, 07665a2, 5664549, e1cd9f5, 93f816a |
| T5 | [x] done | Plugin `/account` commands | R4 R10 | delegated direct (writer trigger: plugin + docs) | 9dc782c, 5458ada. `cid setup` deferred pending a live check of bare `/account` |
| T6 | [ ] | Expiry detection + guided relogin and recapture | R5 | delegated direct | - |
| T7 | [x] done | i18n en/es | R9 | delegated direct | #6 |
| T8 | [ ] | Install script + first signed release | R10 R11 | delegated direct | - |
| T8b | [x] done | release-please initial-version 0.1.0 | R10 | inline (mechanical) | 609339a (#10) |

T0 closed: repo public, CI (3 OS + lint) and CodeQL green, ruleset `protect-main` active, secret scanning + push protection, Dependabot, private vulnerability reporting, fork-workflow approval, SSH commit signing.

## Acceptance criteria
- T1 result documented and design adjusted before T2 starts.
- Switching accounts mid-session keeps context and uses the new account on the next request.
- No token ever written in plaintext outside Claude Code's own credential store.
- CI green on 3 OSes; release produces attested binaries for 6 targets.

## Checks
`go build ./...`, `go vet ./...`, `go test -race ./...`, `gofmt -l .`, golangci-lint, CodeQL, JSON/YAML validity. TDD: RED before GREEN wherever a runnable deterministic test exists.

## Delivery strategy
`ask-on-risk`. Forecast is under about 400 authored lines per slice, except the T0 scaffold, which is larger because of boilerplate and docs.

## Progress
- T0: done. Evidence: commit e30c252 signed; CI and CodeQL runs green on main.
- T0.1: `claude plugin validate .` rejected the `claude-identity` plugin name as reserved; renamed.
- T1: done. User-run experiment on Claude Code 2.1.310 / Windows 11 (see `docs/spikes/r2-hot-swap.md`): a running session picked up swapped credentials on the next request with the same Session ID and no context loss, and did not write the old account back while open or on exit. R1 hot swap confirmed as the primary design; `claude --continue` relaunch stays only as a documented fallback. Untested: token-refresh race, macOS Keychain.
- T2: done. `internal/profile`, `internal/vault` (keyring with chunking for the 2560-byte Windows limit), `internal/credentials`. Open for T3: no cross-process lock on `profiles.json`.
- T8a: PR #4 verified (goreleaser check, snapshot names match install scripts, actionlint, shellcheck, PowerShell parse 0 errors); awaiting merge decision.

## Next step
T3 swap engine (key-level write-back check, file lock, Remote Control warning) and T7 i18n in parallel; then T4 CLI, T5 plugin commands, T6 expiry handling.
