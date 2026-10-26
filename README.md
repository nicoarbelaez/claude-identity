# claude-identity

Keep several Anthropic accounts for Claude Code and hot-swap between them inside a running session, without losing context.

> Status: **pre-alpha**. Nothing is usable yet. The interface below is a plan, not a promise.

## Why

Claude Code supports one logged-in account at a time. Switching normally means logging out, logging in again, and often restarting the session. `claude-identity` stores the credentials of inactive accounts in your operating system keyring and swaps the active one in place, so the same `~/.claude` directory and the same conversation keep working.

## How it works (planned)

- A single `~/.claude` directory stays in use.
- Credentials of inactive accounts (the credentials file on Windows/Linux, the `Claude Code-credentials` Keychain item on macOS, plus the `oauthAccount` block of `~/.claude.json`) live in the OS keyring: Windows Credential Manager, macOS Keychain, or Linux Secret Service.
- When a token expires, Claude Code refreshes it. If that fails, `cid` guides you through `/login` and recaptures the new credentials into the profile.
- `cid` never calls Anthropic OAuth endpoints itself.

## Planned usage

The repository is also a Claude Code plugin marketplace. The plugin provides:

- `/account` to list accounts
- `/account 2` or `/account <name>` to switch
- `/account add <name>` to capture the current login as a profile
- `/account config` to configure language and options

The same operations are available from the CLI binary `cid`. Supported platforms: Windows, macOS, Linux. Languages: English and Spanish.

## Installation

### CLI binary

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.ps1 | iex
```

The scripts download the release archive for your OS and architecture, verify its SHA-256 against `checksums.txt` (aborting on mismatch), and install `cid` to `~/.local/bin` (Windows: `%LOCALAPPDATA%\Programs\cid`). If the GitHub CLI is installed, they also run `gh attestation verify`. Pin a version with `CID_VERSION=0.1.0`. See [docs/RELEASING.md](docs/RELEASING.md).

With Go:

```sh
go install github.com/nicoarbelaez/claude-identity/cmd/cid@latest
```

### Claude Code plugin

Install the Claude Code plugin (Claude Code 2.1.275 or later):

```
/plugin install account-switch --marketplace nicoarbelaez/claude-identity
```

Or add the marketplace first, then install:

```
/plugin marketplace add nicoarbelaez/claude-identity
/plugin install account-switch@nicoarbelaez-tools
```

The plugin needs the `cid` binary on your `PATH` (see [CLI binary](#cli-binary)). To try it from a local checkout without installing, run `claude --plugin-dir ./plugin`.

#### Using the plugin

The plugin provides one skill, `account`, that forwards its arguments to `cid` and prints the result. Plugin skills are namespaced, so the full name is `/account-switch:account`. The bare `/account` also works as long as no other command uses that name.

```
/account              list profiles
/account 2            switch to the second profile
/account isa          switch to the profile named isa
/account add isa      save the current login as "isa"
/account config       show paths, backend and language
```

`list`, `remove`, `current` and `version` are forwarded too. Claude never runs these on its own (`disable-model-invocation: true`); you must type them.

#### Instant responses

The plugin registers a `UserPromptExpansion` hook that runs `cid hook` (no shell) whenever you type `/account`. The hook runs `cid` in-process and shows its exact output to you, without a model turn, so the answer is immediate and costs no tokens. Arguments are limited to profile names or numbers and `--json`; anything else is rejected with a usage message.

This requires `cid` on your `PATH` and Claude Code 2.1.275 or later. If the hook cannot start (for example `cid` is not on `PATH`), the prompt proceeds and the `account` skill runs `cid $ARGUMENTS` through a small model turn instead.

Security: only the skill fallback inserts `$ARGUMENTS` literally into a shell command (`cid $ARGUMENTS`). Because only you can invoke the skill, only the typing user can inject anything, and `cid` validates profile names against `^[a-z0-9][a-z0-9._-]{0,31}$`. Do not paste untrusted text after `/account`.

## Usage

Log in to Claude Code with the account you want to save, then:

| Command | What it does |
|---------|--------------|
| `cid` / `cid list [--json]` | List profiles (index, name, email); the active one is marked with `*`. |
| `cid <n\|name>` / `cid switch <n\|name>` | Hot-swap to a profile. Remote Control sessions disconnect after a swap. |
| `cid add <name> [--force]` | Save the current Claude Code login as a profile and mark it active. Re-run it to refresh a profile. It refuses to replace a profile that holds a different account unless you pass `--force`, and never saves an account that is already stored under another name. |
| `cid remove <n\|name>` | Remove a profile and its keyring entry. The active profile cannot be removed. |
| `cid current [--json]` | Show the active profile. |
| `cid config` | Show the profiles file, the Claude config directory, the credential backend and the language. |
| `cid version` | Show the version. |

`--json` prints stable objects with `index`, `name`, `email` and `active` (never tokens). Exit codes: `0` success, `1` runtime error, `2` usage error.

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
