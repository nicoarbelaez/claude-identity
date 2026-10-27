# account-menu mod (paused prototype)

An interactive `/account` menu drawn as a Claude Code mod. It has Accounts and Config tabs, a detail view, and an active-account usage bar, all backed by `cid --json`.

The prototype is paused. It is not shipped and not part of the plugin.

## Why it is paused
- The list is drawn in a `Client` element. A `Client` only receives keys after a mouse click. On Windows in our test, neither keys nor clicks worked; only the pane's close mark did.
- Tracked upstream in [anthropics/claude-code#101914](https://github.com/anthropics/claude-code/issues/101914), which is open and has no maintainer reply as of 2026-10-26.
- Pane keyboard is limited to Tab, Shift+Tab, Enter and Esc ([keybindings docs](https://code.claude.com/docs/en/keybindings), Pane context). There is no native → for "open details".
- The mods API is marked early access.

## Resume when
#101914 ships, meaning `$.ui.focus` accepts a `Client` key or `Client` gets `autoFocus`. Alternatively, rework the list as `Button` rows plus a `Select` for config. Those use the native focus ring and are usable today, at the cost of no ←/→.

## What the spike proved
- `surface.onKey` delivers `up/down/left/right/return/tab` to a focused `Client`. A `Client` has no `$`, so it `surface.post()`s actions to a `ui.message` hook that runs `cid` (argv, no shell) and answers `{ props }`.
- `command.run` with a `{ command: 'account' }` matcher can take `/account` with no args. Anything else goes through `next(e)` to the existing skill and hook unchanged.
- The usage bar reads `$.session.usage().rateLimits` (`five_hour`, `seven_day`). This covers the active account only.
- Tested on Claude Code 2.1.311.

## Try it
1. Copy this folder, `.claude-plugin/` included, to the session dev-mods folder. The `plugin-authoring` skill names that folder.
2. Run `/account-menu`.

Tests: `claude plugin test prototypes/account-menu-mod`.

## Known gaps
- Two `cid` spawns on every redraw. Cache them, or reload only after an action.
- Relogin and Remove are drawn but not wired.
