# auto-switch

## Objective
`cid auto` switches the live Claude Code account by itself when the active account crosses its 5-hour or weekly line. It picks the target by a deterministic rule that spends weekly quota before it resets.

## Problem
With several accounts, the user switches by hand when one runs low. They also cannot tell which account's weekly quota is about to be wasted.

## Why
Use every account as much as possible, without interruptions and without thinking about it.

## Decisions (user, 2026-10-27)
- **Trigger:** the `cid statusline` wrapper. There is no daemon and no network.
- **Apply:** between turns, from a `UserPromptSubmit` plugin hook, before the next prompt is sent. A response in flight is never cut.
- **Strategy:** soonest weekly reset first, with a principal tie-break and a return to the principal after its 5h reset. When every account is past its weekly line, equalize by headroom.
- **Notice:** a status line line `⇄ <from> → <to> (<window> <pct>%)` shown for a while. It is also logged.
- **Defaults:** 5h 90%, week 95%.
- **Roles:**
  - Principal: wins tie-breaks and is the account the rule returns to.
  - Reserve: used only when no other candidate is under its lines.
- **Equalize:** happens only when all enrolled accounts are past their weekly line.
- **Config:** `cid auto` commands, also reachable as `/account auto ...`.

## Rule
1. **Trigger.** The active account's 5h% ≥ its 5h line, or its week% ≥ its week line.
2. **Eligible candidates.** The account is enrolled, is not active, its login status is not needs_login or unknown, and it is under both its lines. Usage is projected from the cache:
   - A 5h window whose `resetsAt` has passed counts as 0%.
   - A weekly window whose `resetsAt` has passed counts as 0%.
   - An account with no recorded usage counts as 0%, but ranks after accounts that have data.
3. **Pick** among the non-reserve eligible candidates:
   - The earliest weekly `resetsAt` wins. Unknown resets sort last.
   - Then the principal.
   - Then the most headroom, where headroom = min(5h line − 5h%, week line − week%).
   - Then the lowest index.
4. **Return to the principal.** Even when the trigger has not fired, switch to the principal when all of these hold:
   - The principal is not active.
   - The principal is eligible.
   - The principal's weekly reset is no later than the active account's.
   - The active account is not the principal.
5. **Equalize.** When there is no non-reserve candidate and every enrolled account is past its weekly line, pick the most headroom by raw percentages among accounts under 100% in both windows. The reserve is included. If none qualifies, stay put and notify once.
6. **Reserve.** Used only when step 3 has no candidate.
7. **Anti-flap.**
   - A non-forced switch needs the target's headroom to be at least 5 points above the active account's.
   - There is a minimum of 5 minutes between switches.
   - The cooldown is bypassed when the active account is at ≥100% of a window.

## Scope
In:
- `internal/auto`: the policy as a pure function `Decide(state, now) → Decision`, plus a config store (schema, atomic write, lock) and a state store (pending switch, last switch time, last notice).
- `cid auto` commands, i18n EN/ES, and `/account auto` through the existing hook path.
- Integration:
  - `cid statusline` evaluates and records a pending switch after recording usage, best effort, and never delays the wrapped command.
  - `cid hook` handles `UserPromptSubmit` by applying a pending switch through the existing safe switch (U4 locks and rollback), with a short timeout. It never blocks the prompt; on failure it logs and continues.
  - The status line shows the notice line after the wrapped output, for about 2 minutes.
- Docs.

Out:
- Network calls.
- A daemon.
- Per-model windows.
- Multi-machine usage. The cache undercounts in that case; this is documented.

## Constraints
- Reuse: `internal/usage` (cache, render), `swap.Service.Switch` (locks, rollback), `fsutil` (atomic write, schema, recovery), `profile` store, i18n catalog, and the `Deps` injection.
- `Decide` is a pure function with table tests covering every rule above, including the user's example scenario.
- The `UserPromptSubmit` hook must be fast when nothing is pending. That means one small file read, then exit 0 with no output.
- Never read or print token values. Tests never touch the real `~/.claude`, the keyring or `settings.json`.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| A1 | [x] | `internal/auto`: config and state stores, the pure `Decide` with exhaustive table tests | bounded writer | 3960c5a, 93a2145 |
| A2 | [x] | `cid auto` commands (`on`, `off`, `add`, `remove`, `principal`, `reserve`, status), i18n, `/account auto` | bounded writer | 0336916, 51e6004 |
| A3 | [x] | statusline evaluation, the pending switch and the notice line; the `UserPromptSubmit` hook applies it; plugin `hooks.json` | bounded writer | 421e6d1, 98d9eb0, 7037c45, ec67b0a |
| A4 | [x] | Docs: README section, SKILL.md hint, help | bounded writer | e4badba |

Slices: PR 1 is A1 + A2, PR 2 is A3 + A4.

## Acceptance criteria
- In the user's example, account 1's week resets tomorrow and account 2's on Sunday:
  - The rule picks 1.
  - At 1's 5h 90% it moves to 2.
  - After 1's 5h reset it returns to 1.
  - When everything is past its weekly line, it equalizes.
- No switch happens mid-response. It is applied on the next prompt.
- `cid auto off` stops all automatic switching immediately.
- No switch happens more than once in 5 minutes unless the active account is at its limit.

## Checks
`go build ./...` (host, linux, darwin), `go vet ./...`, `go test ./...`, `gofmt -l .`, `go mod tidy` with no diff. Test-first.

## Progress
- Branch `feat/auto-switch` created from `1be076b` (v0.8.0).
- Research: claude-swap `autoswitch.py` (strategies, hysteresis, cooldown) and `pace.py`.
- A1 done (`3960c5a`, `93a2145`): `internal/auto` has the pure `Decide`, `ConfigStore` (auto.json) and `StateStore` (auto-state.json) sharing `auto.lock`. Clarifications of the Rule:
  - The return to the principal (rule 4) has no 5-point hysteresis, only the cooldown. The principal spends its weekly quota first, so it usually has less headroom than a fresh account; the lead would block the acceptance scenario.
  - Equalize (rule 5) needs the target to be strictly better in raw headroom, not +5: with every weekly line crossed, raw headroom is at most 5, so the lead is unreachable unless the active is at 100%.
  - A weekly window that has passed its reset projects to 0% and its next reset is unknown, so it sorts last. Among unknown resets, accounts with data rank before no-data ones, before the principal tie-break.
  - `Decide` ignores the `enabled` flag (the caller checks it). If the active account is not enrolled it never switches away.
  - `NoCandidate` is set whenever the active account fired and nobody can take over, even during the cooldown; the notifier de-duplicates with `lastNoCandidateAt` (A3).
- A2 done (`0336916`, `51e6004`): `cid auto [--json]`, `on|off`, `add`, `remove`, `principal`, `reserve`, `defaults`; `Deps.Auto` and `Deps.AutoState`. `cid remove` and `cid uninstall` clean the auto files. `/account auto ...` already passes the hook allow-list (tokens such as `--5h` match it), now covered by a test. Role commands require an enrolled account (exit 2); there is no command to clear a role yet.
- A3 done: `cid statusline` evaluates after a successful usage recording (inside the best-effort goroutine) and writes `pending` or a once-per-hour no-candidate notice; `cid hook` handles `UserPromptSubmit` silently (exit 0, no output) and applies the pending switch through `swap.Service.Switch` after re-validating it (still enabled, target enrolled, not older than 10 min, `from` is the live profile). The status line prints the notice after the wrapped output while `now < until` and clears an expired one lazily. State gained `history` (last 20, with an `error` field for failures). Deviations:
  - The evaluation also clears a pending switch when the new decision no longer holds, so a stale one is not applied.
  - A failed switch sets `lastSwitchAt` too, so a persistent failure cannot retry on every prompt (the cooldown is bypassed only at 100%).
  - There is no extra timeout around `Switch`: abandoning it mid-write could leave a half-done swap, so it relies on the Claude Code lock wait (at most about 9 s) and the hook timeout of 30 s.
  - `AutoStatePort` now has `Update`; `cid auto` and `--json` show the latest 5 history entries.
  - Risk: each evaluation reads the keyring status of every enrolled non-active account (local, no network); it only runs when the status line refresh carries `rate_limits` for the live account.
- A4 done: README section "Automatic switching" and table row, `/account auto ...` line, SKILL.md argument hint. Help text from A2 needed no change.
