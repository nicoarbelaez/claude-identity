# account-usage

## Objective
`/account` and `cid list` show every account as a numbered row: active marker, email, status, and 5-hour and weekly usage bars with the reset countdown and the reading's age. `/account <n>` switches. Switching also adopts claude-swap's local safety mechanisms: Claude Code's credential locks, guarded capture of the outgoing login, rollback, and an expiry buffer.

## Problem
The list shows only names, emails and status. Switching takes only cid's own lock, so it can race with a token refresh that Claude Code is doing at the same moment.

## Why
Seeing which account has quota left, then switching with one short command, is the main reason to use cid. The safety work makes switching reliable while Claude Code is running.

## Scope
In:
- **Usage cache** per profile, written atomically, validated by schema, with its own lock.
- **`cid statusline`**:
  - Reads Claude Code's status line JSON from stdin and records `rate_limits` for the active profile.
  - Then runs the wrapped status line command (`--` argv) with the same stdin and passes its output through.
  - `cid statusline install` wraps the user's existing `statusLine` with a backup; `cid statusline uninstall` restores it.
- **List rendering**:
  - Numbered rows and bars. Colors by threshold only on a TTY, and `NO_COLOR` turns them off.
  - The countdown is recomputed at render time. Windows whose reset has passed are dropped. An age note appears once the reading is old.
  - `--json` gains usage fields.
- **Switch safety**:
  - Hold Claude Code's lock directories (`~/.claude/.oauth_refresh.lock`, `~/.claude.lock`, `~/.claude.json.lock`) around the live write, proper-lockfile style.
  - Never capture wiped or foreign credentials into a profile.
  - Roll back the live write when verification fails.
  - Use a 5-minute expiry buffer in `Status`.
  - Retry renames on Windows sharing violations.
- README, SKILL.md and help text.

Out:
- Any network call or use of tokens: no `/api/oauth/usage`, no refresh.
- A terminal UI or a Claude Code mod.
- Auto-switching.

## Constraints
- **No network, no token values.** Usage comes only from the status line stdin. Identity comes from `oauthAccount.emailAddress`, never from tokens.
- **The status line must stay fast and fail open.** It never blocks Claude Code's own command; if cid fails, the wrapped command still runs and its output is unchanged. Recording is best effort with a short lock timeout.
- **No network I/O while Claude Code's locks are held.**
  - Lock semantics match proper-lockfile: `mkdir` atomicity, stale after 60 s (config lock 10 s), mtime touch while held, 9 s acquire timeout.
  - A timeout aborts the switch with one line, and nothing is written.
- **Reuse what exists**: `internal/fsutil` (atomic write, schema, recovery), `internal/settings` registry if a new setting is needed, `gofrs/flock`, the i18n catalog, the `Deps` injection. Avoid new dependencies unless clearly justified (`golang.org/x/term` for TTY detection is acceptable).
- One-line messages, no numbered steps. EN and ES.
- Tests use `t.TempDir()` and fakes only. They never touch the real `~/.claude`, the keyring or the real `settings.json`.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| U1 | [x] | `internal/usage`: per-profile cache (schema, atomic write, lock, recovery), render helpers (bar, countdown from `resets_at`, age note, drop passed windows) | bounded writer | 46b66ef, 02aafce |
| U2 | [x] | `cid statusline` record and passthrough, plus `install`/`uninstall` of the wrapper in user `settings.json` (backup, idempotent, keeps the user's command) | bounded writer | 5dd56af, 7f7e0e6 |
| U3 | [x] | `cid list` and `/account` rendering with numbered rows, bars and colors; `--json` usage fields; `/account <n>` switch confirmed | bounded writer | 47df9c7, 49329c4, 410301d, dd4f3ad, f561267, da89299 |
| U4 | [ ] | Switch safety: Claude Code lock directories, guarded capture (skip wiped/foreign), rollback on failed verify, 5-minute expiry buffer, Windows rename retry | bounded writer | |
| U5 | [ ] | Docs: README (usage, status line setup, safety notes), SKILL.md hint, help text | bounded writer | |

Route evidence: 2+ non-trivial files per task, so a writer. Planned slices: U1+U2, U3, U4, U5. That forecast is more than 400 lines, so chained PRs.

## Acceptance criteria
- With the wrapper installed, the active account's 5h/7d usage appears in `/account` and `cid list` after the first API response.
- Inactive accounts show their last reading with an age note, or no bars when nothing was ever recorded.
- The user's own status line output is unchanged with the wrapper on. Uninstall restores the exact previous `statusLine`.
- `/account 2` switches to profile 2 with no model turn.
- A switch while a Claude Code lock is held waits up to the timeout, then fails with one line and leaves everything unchanged.
- A wiped or foreign live credential is never stored into a profile.
- A failed verification restores the previous live login.

## Checks
`go build ./...` (host, linux, darwin), `go vet ./...`, `go test ./...`, `gofmt -l .`, `go mod tidy` with no diff. Test-first.

## Delivery
`ask-on-risk`, with chained PRs into main.

## Progress
- Branch `feat/account-usage` created from `eab0300`.
- Research:
  - The status line stdin is the only non-network source of `rate_limits` (statusline docs).
  - Plugins cannot ship a `statusLine`.
  - claude-swap's lock protocol is `claude_locks.py:1-86`; its guarded capture is `switcher.py:6710-6856`.
- U1 done (`46b66ef`, `02aafce`):
  - `internal/usage` holds the schema-validated `usage.json` cache next to `profiles.json`, with its own `usage.lock` (2 s timeout; a timeout skips the record) and the render helpers (`Bar`, `Countdown`, `Age`, `Visible`, `LevelOf`).
  - `cid remove` forgets that profile's reading; `cid uninstall` deletes `usage.json`, its `.bak`/`.corrupt-*` siblings and `usage.lock`.
- U2 done (`5dd56af`, `7f7e0e6`):
  - `cid statusline [-- cmd...]` records `rate_limits` only when the live `oauthAccount` email matches the active profile's stored email, then runs the wrapped command by argv with the same stdin bytes. The recording is wrapped in `recover` and every error is dropped, so it fails open.
  - `statusline install` and `uninstall` edit `settings.json` as an order-preserving raw object. The previous `statusLine` text is saved byte for byte in `statusline.backup.json`, and `settings.json.bak` holds the file as it was before each change.
  - Install refuses commands that use shell syntax (`$ \` | ; & < > ( ) * ? [ ] { } ~ \`), because a split-and-requote round trip would change their meaning.
  - `cid uninstall` also restores the previous status line, so the user is never left with a wrapper that points at a deleted binary.
  - The hook rejects `statusline`.
- U3 done (`47df9c7`, `49329c4`, `410301d`, `dd4f3ad`, `f561267`, `da89299`):
  - `cid` and `cid list` share one renderer (`internal/cli/list.go`): a marker, the index, aligned name and email columns, then a note with the status (only when not ok) and the reading's age, then one line per visible window (`5h`, `week`) with bar, percent and "resets <countdown>". Windows past their reset are dropped.
  - The hint to run `cid statusline install` is printed once, under the active row, only when the active profile never had a reading and the wrapper is not installed (or settings.json cannot be read, in which case nothing is guessed).
  - Colors come from `Deps.Color`, set in main by `cli.UseColor(os.Stdout)` (TTY via golang.org/x/term, no `NO_COLOR`, no `TERM=dumb`; on Windows also a VT hint such as `WT_SESSION`/`TERM`). The hook forces `Color=false`, and a test checks its output has no ANSI.
  - `cid list --json` adds `usage` ({fetchedAt, ageSeconds, windows[{kind, percentUsed, resetsAt}]}, visible windows only) and leaves the key out when nothing was recorded. Documented in the README JSON section only.
  - `usage.AgeOf` returns the age as a number and a unit so the age note is translated through the catalog. `UsagePort` gained `All()`.
  - `/account <n>` and `/account <name>` switch inside the hook with a block decision (no model turn); a numeric test was added next to the existing name one.
  - Review follow-ups from U1+U2: (1) the wrapped command now runs first while the recording runs alongside it, waited for at most 750 ms (tests: order, and a locked store changes neither output nor exit code); (2) `cid uninstall` keeps `statusline.backup.json` and warns in one line when settings.json is unreadable; (3) the backup's `hadStatusLine` is a pointer so a missing field is not read as "there was none", and the zero value of the restore result is now `restoreFailed`; (4) the quoting comment is narrowed to POSIX shells and a test round-trips the words through a real `sh` (skipped when none).
