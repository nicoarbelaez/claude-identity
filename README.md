# claude-identity

Keep several Anthropic accounts for Claude Code and hot-swap between them inside a running session, without losing context.

## Why

Claude Code supports one logged-in account at a time. Switching normally means logging out, logging in again, and often restarting the session. `claude-identity` stores the credentials of inactive accounts in your operating system keyring and swaps the active one in place, so the same `~/.claude` directory and the same conversation keep working.

## How it works

- A single `~/.claude` directory stays in use.
- Credentials of inactive accounts (the credentials file on Windows/Linux, the `Claude Code-credentials` Keychain item on macOS, plus the `oauthAccount` block of `~/.claude.json`) live in the OS keyring: Windows Credential Manager, macOS Keychain, or Linux Secret Service.
- When a token expires, Claude Code refreshes it. If that fails, `cid` guides you through `/login` and recaptures the new credentials into the profile.
- `cid` never calls Anthropic OAuth endpoints itself.

## Plugin commands

The repository is also a Claude Code plugin marketplace. The plugin provides:

- `/account` to list accounts
- `/account 2` or `/account <name>` to switch
- `/account add <name>` to capture the current login as a profile
- `/account config` to configure language and options

The same operations are available from the CLI binary `cid`. Supported platforms: Windows, macOS, Linux. Languages: English and Spanish.

## Installation

### 1. Install `cid`

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.ps1 | iex
```

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.sh | sh
```

Or with Go:

```sh
go install github.com/nicoarbelaez/claude-identity/cmd/cid@latest
```

The scripts download the release archive for your OS and architecture, verify its SHA-256 against `checksums.txt` (aborting on mismatch), and install `cid` to `%LOCALAPPDATA%\Programs\cid` (Windows) or `~/.local/bin` (macOS / Linux). If the GitHub CLI is installed, they also run `gh attestation verify`.

- Windows: the install directory is added to your user `PATH` automatically. Set `CID_NO_MODIFY_PATH=1` to skip that.
- macOS / Linux: if the directory is not on your `PATH`, the script prints the line to add for your shell.
- Pin a version with `CID_VERSION=<version>`; change the directory with `CID_INSTALL_DIR`.

See [docs/RELEASING.md](docs/RELEASING.md).

### 2. Verify

Open a new terminal and run:

```sh
cid version
```

### 3. Install the Claude Code plugin

In Claude Code (2.1.275 or later):

```
/plugin marketplace add nicoarbelaez/claude-identity
/plugin install account-switch@nicoarbelaez-tools
```

Then restart Claude Code so it picks up `cid` on your `PATH`. Plugin commands are namespaced as `/account-switch:account`; `/account` works when the name is not ambiguous.

### 4. First use

1. Run `/login` with your first account, then `/account add a`.
2. Run `/login` with your second account, then `/account add b`.
3. Switch with `/account 1` or `/account b`.

To try the plugin from a local checkout without installing, run `claude --plugin-dir ./plugin`.

## Uninstall

```sh
cid uninstall          # lists what it will remove and asks for confirmation
cid uninstall --yes    # no confirmation
```

`cid uninstall` removes every saved profile's stored credentials (OS keyring), the profiles file and the now-empty `cid` config folder, and, on Windows, the install directory from your user `PATH`. It never touches Claude Code's own login (`~/.claude/.credentials.json`, `~/.claude.json`, or the macOS `Claude Code-credentials` Keychain item), so you stay logged in with whichever account is live. If a step fails it keeps going, reports each failure and exits 1. It is not available through the plugin hook; run it in a terminal.

`cid` cannot delete its own running binary on Windows, so finish by hand:

1. Delete the binary. `cid uninstall` prints its path and the exact command (`rm "<path>"` on macOS and Linux, `Remove-Item "<path>"` in PowerShell on Windows).
2. Remove the plugin from Claude Code:

   ```
   /plugin uninstall account-switch@nicoarbelaez-tools
   /plugin marketplace remove nicoarbelaez-tools
   ```

## Using the plugin

The plugin provides one skill, `account`, whose command is answered by a hook that runs `cid` (see [Instant responses](#instant-responses)). Plugin skills are namespaced, so the full name is `/account-switch:account`. The bare `/account` also works as long as no other command uses that name.

```
/account              list profiles
/account 2            switch to the second profile
/account isa          switch to the profile named isa
/account add isa      save the current login as "isa"
/account config       show paths, backend and language
```

`list`, `remove`, `current` and `version` are forwarded too. Claude never runs these on its own (`disable-model-invocation: true`); you must type them.

### Instant responses

The plugin registers a `UserPromptExpansion` hook that runs `cid hook` (no shell) whenever you type `/account`. The hook runs `cid` in-process and shows its exact output to you, without a model turn, so the answer is immediate and costs no tokens. It only answers when `cid` succeeds. Arguments are limited to profile names or numbers and `--json`.

If `cid` fails (unknown command or profile, bad arguments, runtime error) or the arguments are not valid, the hook does not block the prompt. It passes the error to the model as context, together with a note that `cid` already ran, and the `account` skill explains how to fix the command. `cid` runs exactly once. If the hook cannot start (for example `cid` is not on `PATH`), the skill tells you to install `cid`. This requires Claude Code 2.1.275 or later.

Security: arguments never reach a shell. The hook runs `cid` directly (exec form, no shell) and rejects any token that is not a profile name or number, or `--json`. The skill body never runs your command; its only shell line is the fixed `cid help`.

## Usage

Log in to Claude Code with the account you want to save, then:

| Command | What it does |
|---------|--------------|
| `cid` / `cid list [--json]` | List profiles (index, name, email, token status); the active one is marked with `*`. |
| `cid <n\|name>` / `cid switch <n\|name>` | Hot-swap to a profile. Remote Control sessions disconnect after a swap. |
| `cid add <name> [--force]` | Save the current Claude Code login as a profile and mark it active. Re-run it to refresh a profile. It refuses to replace a profile that holds a different account unless you pass `--force`, and never saves an account that is already stored under another name. |
| `cid remove <n\|name>` | Remove a profile and its keyring entry. The active profile cannot be removed. |
| `cid relogin <n\|name>` | Print the steps to refresh a profile's login (run `/login` in Claude Code, then `cid add <name>`). It does not touch any token. |
| `cid current [--json]` | Show the active profile and its token status. |
| `cid config` | Show the profiles file, the Claude config directory, the credential backend and the language. |
| `cid version` | Show the version. |

The text output of `cid list` and `cid current` is meant for people and may change between versions; scripts should use `--json`. `--json` prints stable objects with `index`, `name`, `email`, `active` and `status` (never tokens). Exit codes: `0` success, `1` runtime error, `2` usage error.

### Token status

The status column (and the `status` JSON field) is one of `ok`, `access_expired` (Claude Code refreshes it on next use), `needs_login` (no refresh token; run `cid relogin`) or `unknown`. It is computed locally from the expiry time and the presence of a refresh token, without contacting Anthropic, so it cannot prove a token is still accepted. A saved copy can also go stale if the same account is used on another machine; if a switch then asks you to sign in again, run `cid relogin <name>`.

## Language

`cid` speaks English and Spanish. The language is picked in this order, and the first match wins:

1. `CID_LANG` environment variable (`en` or `es`, case-insensitive).
2. The Claude Code `language` setting (`settings.json`, overridden by `settings.local.json`, in `CLAUDE_CONFIG_DIR` or `~/.claude`). Values such as `Spanish`, `español` or `es` select Spanish; `English` or `en` select English; anything else is ignored.
3. `LC_ALL`, `LC_MESSAGES`, then `LANG` (for example `es_CO.UTF-8`).
4. The OS user locale (Windows UI language, macOS `AppleLocale`).
5. English.

Only the base language matters, so `es-MX` and `es_ES` both select Spanish. Force a language with `CID_LANG=es cid list`.

## Security note

This tool handles OAuth tokens. It stores them only in your OS keyring and never sends them anywhere. Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
