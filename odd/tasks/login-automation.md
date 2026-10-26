# login-automation

## Objective
Let `cid add <name>` and `cid relogin <n|name>` start the Claude Code login themselves, so users no longer run `/login` first, and capture the result into a profile without touching the live Claude Code login.

## Problem
`cid add` captures whatever account is currently logged in to Claude Code, so the user must run `/login` first. `cid relogin` only prints manual steps.

## Why
Adding or refreshing an account should be one command. Running Claude Code's own login in an isolated config dir avoids disturbing the running session and avoids implementing OAuth.

## Scope
In: `internal/login` runner (exec + fake), `swap.Service` save-from-source and relogin methods, `cid add` / `cid add --current` / `cid relogin` flows, plugin hook guidance, i18n (en/es), README, help text.
Out: implementing OAuth, calling Anthropic endpoints, token refresh by us, macOS isolated login (Keychain item name for a custom config dir is unknown; treated as unsupported for now).

## Constraints
- Run `claude auth login [--console] [--sso] [--email <email>]` with `CLAUDE_CONFIG_DIR=<private temp dir>`, terminal attached, then read the credentials from that dir with the existing file store.
- Temp dir is created 0700 and always removed (defer, also on error and interrupt).
- Never read or print token values; tests never touch real `~/.claude` or the OS keyring.
- Hook (`cid hook`) has no terminal and a 20s timeout: it must never start a login.
- Go; Windows, macOS, Linux; English code, comments, docs; neutral Spanish for ES strings.

## Open spike items (unverified, a user spike is running)
- S1: does `claude auth login` write into `CLAUDE_CONFIG_DIR` (credentials file and `.claude.json` with `oauthAccount`)?
- S2: does `claude auth login` work without a TTY on stdin (only matters if we ever pipe it; we attach the terminal)?
- S3: macOS Keychain item name for a non-default config dir (blocks macOS support).
If S1 fails, the isolated flow reports "no credentials" and falls back to the manual steps; the code paths stay valid.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| L1 | [x] | Isolated login runner (`internal/login`: Options, exec runner, fake, `claudecfg.PathsIn`) | inline | 3a560c5 |
| L2 | [x] | `cid add` with auto login (`swap.AddFrom`, `--console/--sso/--current`, darwin and missing-claude fallback) | bounded writer | c5888f9 |
| L3 | [x] | `cid relogin` with auto login (`swap.Relogin`, email match, live store refresh when active) | bounded writer | 586565c |
| L4 | [x] | Hook guidance: block `add` (without `--current`) and `relogin`, keep `add --current` | bounded writer | 75d920d |
| L5 | [x] | Docs: README Usage table + "Adding accounts", help text en/es, SKILL.md if needed | bounded writer | this commit |

## Acceptance criteria
- `cid add <name>` saves the freshly logged-in account as a profile without changing the live login or the active profile.
- Same name + same email refreshes; different email needs `--force`; an account saved under another name is refused.
- `cid add <name> --current` behaves exactly as before.
- `cid relogin` refuses to save when the logged-in email differs from the profile email; overwrites stored credentials; refreshes the live store only when the profile is active.
- Temp dir removed in every outcome.
- Missing `claude`, failed login, empty temp dir or darwin: localized message, manual fallback steps, exit 1.
- `cid hook` never starts a login; `add --current` still works from the hook.

## Checks
`go build ./...` (also GOOS=linux and GOOS=darwin), `go vet ./...`, `go test ./...`, `gofmt -l .`, golangci-lint. TDD: RED (compile or assertion failure) before GREEN for every task.

## Progress
- L1-L5 done, each with RED observed first (compile or assertion failure) and GREEN after the implementation. Tests use fake runners and temp dirs only; the real keyring and `~/.claude` are never touched.
- Design: `login.Runner` interface (exec + `FakeRunner`); `swap.Service.AddFrom` shares the Add guard (`addFrom`) and neither marks active nor touches live; `swap.Service.Relogin` checks the email, overwrites stored credentials and writes the live store only for the active profile; the login itself runs outside the cross-process lock (only the save is locked).
- `cid add` without `--current` is now the isolated login; existing live-capture tests were updated to pass `--current`.
- Unverified, depends on the spike: S1 (`claude auth login` honors `CLAUDE_CONFIG_DIR`), S2 (no-TTY use), S3 (macOS). Until S1 is confirmed, a real run may end in the "no credentials" fallback.

## Next step
Run the user spike results (S1/S2) against a real `cid add` on Windows/Linux, then decide on macOS support (S3).
