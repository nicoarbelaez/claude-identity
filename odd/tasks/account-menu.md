# account-menu

## Objective
`/account` with no arguments opens an interactive menu inside Claude Code that follows the native Settings look: tabs, a keyboard-driven account list, a detail view, and a Config tab. `/account config` opens the Config tab.

## Problem
`/account` only runs text commands. There is no way to browse accounts, see their state, or change settings visually.

## Why
Browsing and acting on accounts should feel like Claude Code's own `/config` and `/usage` screens, not like typing commands.

## Scope
In:
- A Claude Code mod (plugin `modules`): a pane with tabs, the Accounts list, a detail view with actions, and a Config tab generated from `cid config --json`.
- 5-hour and weekly bars for the active account, taken from the session's `rateLimits`.
- Shared UI pieces (`Tabs`, `Row`, `Bar`, `Hint`) drawn in theme colors.
- `cid plugin install|remove` as a CLI alternative to installing from Claude Code.

Out:
- A Usage tab or usage history (dropped by the user).
- Any terminal UI.
- Calling Anthropic endpoints or reading tokens.

## Constraints
- `/account <args>` and the `cid` CLI keep working unchanged. The menu only takes over `/account` with no arguments and `/account config`.
- The mod is a thin view: it reads `cid list --json` and `cid config --json`, and acts through `cid` (argv, no shell).
- Same design language as Claude Code: theme keys, a dim hint line, `›` on enum values.
- Adding a tab means adding one file to a tab registry. A new CLI setting appears in Config with no mod change.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| M0 | [ ] | Spike: a prototype mod in the session dev-mods folder. It must prove three things: `Client` key handling (↑↓ → ← tab enter), opening the pane from `/account` with no args while args still reach the current skill/hook, and the minimum Claude Code version. | bounded writer | (outside repo) |
| M1 | [ ] | CLI: `cid plugin install|remove` (fixes a local-directory marketplace); install scripts and `cid uninstall` call it | | |
| M2 | [ ] | Mod shell inside `plugin/`: pane, tab registry, `Tabs`/`Row`/`Bar`/`Hint` | | |
| M3 | [ ] | Accounts tab, detail view and actions (switch, relogin with progress, remove), active-account bars | | |
| M4 | [ ] | Config tab generated from `cid config --json`; `/account config` opens it | | |
| M6 | [ ] | Docs, SKILL.md, fallback notes, release | | |

## Acceptance criteria
- `/account` opens the menu.
- ↑↓ moves through accounts, → opens the detail, ← goes back, Enter switches, Esc closes.
- `/account add x`, `/account list` and every other argument form behave exactly as today.
- The Config tab lists every setting from `cid config --json` and changes it through `cid config set`.

## Delivery
Strategy `ask-on-risk`. Forecast is more than 400 lines, so chained PRs, one per M task after M0.

## Status: paused (2026-10-26)
The manual test on Windows failed: keys never reached the pane, and only the close mark worked.
- Upstream blocker: [anthropics/claude-code#101914](https://github.com/anthropics/claude-code/issues/101914). A `Client` only gets keys after a click.
- The prototype is kept in `prototypes/account-menu-mod/` on branch `spike/account-menu-mod`. Its README covers resume conditions.
- M1–M6 are not started. The user chose to improve the text `/account` output instead (feature `account-list`).

## Progress
- Branch `feat/account-menu` created from main (`eab0300`).
- M0 prototype written in `~/.claude/dev-mods/<session>/account-menu`.
  - `plugin validate`, `tsc` and `plugin test` (3/3) pass on fakes.
  - Findings:
    - `Client` receives up/down/left/right/return/tab through `surface.onKey`. A `Client` has no `$`, so it posts actions to a `ui.message` hook that runs `cid`.
    - `command.run` with a `{ command: 'account' }` matcher can take the no-args case and pass `/account <args>` through with `next(e)`.
    - Types are from Claude Code 2.1.311, the tested floor. The API is early access.
  - Pending manual test: whether the keyboard reaches `Client` without a click, which command name fires for the plugin skill, and the redraw cost (two `cid` spawns per render; M2 should cache).
