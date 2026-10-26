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

## Installation (planned)

Signed release binaries from GitHub Releases, `go install github.com/nicoarbelaez/claude-identity/cmd/cid@latest`, and the Claude Code plugin marketplace. An install script may follow.

## Security note

This tool handles OAuth tokens. It stores them only in your OS keyring and never sends them anywhere. Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
