# claude-identity

Keep several Anthropic accounts for Claude Code and hot-swap between them inside a running session, without losing context.

## Why

Claude Code supports one logged-in account at a time. Switching normally means logging out, logging in again, and often restarting the session. `claude-identity` stores the credentials of inactive accounts in your operating system keyring and swaps the active one in place, so the same `~/.claude` directory and the same conversation keep working.

## How it works

- A single `~/.claude` directory stays in use.
- Credentials of inactive accounts (the credentials file on Windows/Linux, the `Claude Code-credentials` Keychain item on macOS, plus the `oauthAccount` block of `~/.claude.json`) live in the OS keyring: Windows Credential Manager, macOS Keychain, or Linux Secret Service.
- When a token expires, Claude Code refreshes it. If that fails, run `cid relogin <name>` (or `/account relogin <name>` in Claude Code): it starts the Claude Code login for that account and refreshes the profile.
- `cid` never calls Anthropic OAuth endpoints itself.

## Plugin commands

The repository is also a Claude Code plugin marketplace. The plugin provides:

- `/account` to list accounts
- `/account 2` or `/account <name>` to switch
- `/account add <name>` to log in through the browser and save the account as a profile
- `/account relogin <n|name>` to refresh a profile's login
- `/account add <name> --current` to capture the login that is live in Claude Code (run `/login` first)
- `/account config` to configure language and options
- `/account auto ...` to set up automatic switching

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

1. Run `cid add a` (or `/account add a` in Claude Code) and sign in with your first account in the browser.
2. Run `cid add b` and sign in with your second account.
3. Switch with `/account 1` or `/account b`.

To try the plugin from a local checkout without installing, run `claude --plugin-dir ./plugin`.

## Updating

```sh
cid update
```

One command updates both parts. It downloads the latest release from GitHub, checks it against the release `checksums.txt`, replaces the `cid` binary, then runs `claude plugin marketplace update nicoarbelaez-tools` and `claude plugin update account-switch@nicoarbelaez-tools`. Restart Claude Code or run `/reload-plugins` to load the new plugin.

- `cid version` prints the version and, when a newer release exists, one extra line pointing to `cid update`. It checks only when you run it, with a 3-second timeout, and prints nothing extra if you are offline or the check fails.
- If the checksum does not match or the download fails, `cid update` exits `1` and leaves the binary unchanged.
- The plugin step needs `claude` on your `PATH`. If it fails, `cid update` prints one warning and its exit code still reflects only the binary update.
- The marketplace must have been added from GitHub (`/plugin marketplace add nicoarbelaez/claude-identity`), not from a local directory; a local marketplace has nothing to fetch.
- Development builds cannot be updated. `cid update` is not available through the plugin hook; run it in a terminal.

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
/account add isa     log in in the browser and save the account as "isa"
/account relogin isa refresh the login of "isa"
/account config       show paths, backend, language and settings
/account config set language es   save the interface language
```

`list`, `remove`, `current` and `version` are forwarded too. Claude never runs these on its own (`disable-model-invocation: true`); you must type them.

### Instant responses

The plugin registers a `UserPromptExpansion` hook that runs `cid hook` (no shell) whenever you type `/account`. The hook runs `cid` in-process and shows its exact output to you, without a model turn, so the answer is immediate and costs no tokens. It only answers when `cid` succeeds. Arguments are limited to profile names or numbers and `--json`.

If `cid` fails (unknown command or profile, bad arguments, runtime error) or the arguments are not valid, the hook does not block the prompt. It passes the error to the model as context, together with a note that `cid` already ran, and the `account` skill explains how to fix the command. `cid` runs exactly once. If the hook cannot start (for example `cid` is not on `PATH`), the skill tells you to install `cid`. This requires Claude Code 2.1.275 or later.

Security: arguments never reach a shell. The hook runs `cid` directly (exec form, no shell) and rejects any token that is not a profile name or number, or `--json`. The skill body never runs your command; its only shell line is the fixed `cid help`.

## Usage

### Adding accounts

`cid add <name>` runs Claude Code's own login (`claude auth login`) in a private temporary config directory, so your current Claude Code login and the active profile are not touched. It opens the normal browser sign-in, then saves the new account as a profile and deletes the temporary directory (also on error or Ctrl+C). `cid` never implements OAuth or contacts Anthropic itself. Add `--console` for an Anthropic Console account or `--sso` for SSO.

`cid relogin <n|name>` does the same for an existing profile, pre-filling its email. It refuses to save if you sign in with a different account, overwrites the profile's stored login, and, when the profile is the active one, also refreshes the live Claude Code login.

Requirements and fallbacks: `claude` must be on your `PATH`, and macOS is not supported for the isolated login yet. When the login cannot run or produces no credentials, `cid` prints the cause and one short retry sentence, and exits with code 1. On macOS use `/login` and then `cid add <name> --current`.

Inside Claude Code, `/account add` and `/account relogin` run the same login from the plugin hook: the browser opens, the hook waits up to 10 minutes and then shows the result without a model turn. If it fails, the assistant explains the cause in a sentence and can retry when you ask.

### Commands

| Command | What it does |
|---------|--------------|
| `cid` / `cid list [--json]` | List profiles (index, name, email, token status) with 5h and week usage bars when a reading exists; the active one is marked with `●`. See [Usage per account](#usage-per-account). |
| `cid <n\|name>` / `cid switch <n\|name>` | Hot-swap to a profile. Remote Control sessions disconnect after a swap. |
| `cid add <name> [--force] [--console] [--sso]` | Log in through Claude Code in an isolated config directory and save the account as a profile. The live login and the active profile are not changed. Re-run it to refresh a profile. It refuses to replace a profile that holds a different account unless you pass `--force`, and never saves an account that is already stored under another name. |
| `cid add <name> --current [--force]` | Save the current Claude Code login as a profile and mark it active (run `/login` first). Same guards as above. |
| `cid remove <n\|name>` | Remove a profile and its keyring entry. The active profile cannot be removed. |
| `cid relogin <n\|name> [--console] [--sso]` | Log in again through Claude Code as the profile's account and refresh its saved login (and the live login if the profile is active). Refuses a different account. If the login cannot run, it prints the cause in one line. |
| `cid current [--json]` | Show the active profile and its token status. |
| `cid config [--json]` | Show the file locations, the credential backend, the language and every setting with its value and source. See [Configuration](#configuration). |
| `cid config get <key> [--json]` | Print one setting value. Exit `2` for an unknown key. |
| `cid config set <key> <value>` | Validate and save a setting in `config.json`. Exit `2` for an unknown key or an invalid value. |
| `cid config unset <key>` | Remove a setting from `config.json` so its default applies again. |
| `cid statusline [install\|uninstall]` | Record plan usage from Claude Code's status line input. `install` wraps your current status line and `uninstall` restores it exactly. See [Usage per account](#usage-per-account). |
| `cid auto [on\|off\|add\|remove\|principal\|reserve\|defaults]` | Switch accounts automatically when the active one nears its limits. See [Automatic switching](#automatic-switching). |
| `cid update` | Update the `cid` binary (checksum verified) and the `account-switch` plugin to the latest release. See [Updating](#updating). |
| `cid version` | Show the version, and a note when a newer release exists. |

The text output of `cid list` and `cid current` is meant for people and may change between versions; scripts should use `--json`. `--json` prints stable objects with `index`, `name`, `email`, `active` and `status` (never tokens). `cid list --json` adds a `usage` object to each profile that has a recorded reading (the key is left out when there is none): `{"fetchedAt": "2026-03-04T09:58:30Z", "ageSeconds": 90, "windows": [{"kind": "five_hour", "percentUsed": 62, "resetsAt": "2026-03-04T12:47:00Z"}]}`. `kind` is `five_hour` or `seven_day`, and `windows` lists only the windows whose reset is still ahead (it can be empty). Exit codes: `0` success, `1` runtime error, `2` usage error.

### Token status

The status column (and the `status` JSON field) is one of `ok`, `access_expired` (Claude Code refreshes it on next use), `needs_login` (no refresh token; run `cid relogin`) or `unknown`. It is computed locally from the expiry time and the presence of a refresh token, without contacting Anthropic, so it cannot prove a token is still accepted. A saved copy can also go stale if the same account is used on another machine; if a switch then asks you to sign in again, run `cid relogin <name>`.

## Usage per account

`cid list` shows how much of each account's plan you have used:

```
Accounts

 ●  1  work      me@example.com   active
       5h     ████████████████████░░░░░░░░░░░░    62%   resets 2h 47m
       week   ██████░░░░░░░░░░░░░░░░░░░░░░░░░░    18%   resets 4d 21h

 ○  2  personal  you@example.com  3h ago
       5h     ████████░░░░░░░░░░░░░░░░░░░░░░░░    25%   resets 1h 2m
       week   ████████████████████████████░░░░    88%   resets 1d 4h
```

Each account gets a 5-hour bar and a weekly bar, the time until each window resets, and, when the reading is older than a few minutes, how long ago it was taken (`3h ago`). Windows that have already reset are not shown. Bars turn yellow and red as they fill when the terminal supports color (`NO_COLOR` turns it off).

**Where the data comes from.** Claude Code passes `rate_limits` to its status line command on stdin. It is present only for Pro and Max accounts, and only after the first response of a session. `cid statusline` reads that input and stores the numbers for the active account. `cid` never calls Anthropic and never reads token values for this.

**Setup.**

```sh
cid statusline install     # wrap the current status line
cid statusline uninstall   # put it back exactly as it was
```

`install` wraps your existing status line command: it still runs with the same input and its output is unchanged. `uninstall` restores the previous setting byte for byte. `cid uninstall` also unwraps it. While the active account has no reading and the wrapper is not installed, `cid list` prints a one-line hint to run `cid statusline install`.

**Windows.** If the previous status line command contains backslashes or shell metacharacters, `install` refuses and changes nothing. Use forward slashes in that command (for example `C:/tools/line.exe`) and run `install` again.

## Automatic switching

`cid auto` switches the live account for you when the active one gets close to its limit, so a long session does not stop at the 5-hour or weekly wall. It works from the usage readings that `cid statusline` already records, makes no network call and never reads a token.

**When.** The status line checks the active account on every refresh. If its 5-hour usage reaches the 5h line (90% by default) or its weekly usage reaches the week line (95%), a switch is queued. A `UserPromptSubmit` hook applies it just before your next message is sent, never in the middle of a response. The status line then shows `⇄ work → personal (5h 91%)` for about two minutes, and the switch is kept in a short log that `cid auto` prints.

**Which account.** Only enrolled accounts that are signed in and under both of their lines are candidates.

- The account whose weekly window resets soonest goes first, so weekly quota is spent before it renews. A tie goes to the principal, then to the account with the most room, then to the lowest index.
- The principal is also the account the rule returns to: once its weekly reset is no later than the active account's and it is under its lines again (for example after its 5-hour window reset), `cid` moves back to it even though nothing is near a limit.
- A reserve account is used only when no other account is under its lines.
- When every enrolled account is past its weekly line, `cid` equalizes: it moves to the account with the most room left, provided that room is larger than the active account's and the account is below 100%.
- To avoid flapping, a switch needs the target to have at least 5 points more headroom than the active account (the return to the principal and equalizing are exempt), and two switches are at least 5 minutes apart. An active account at 100% of a window skips both rules.

**Setup.**

```sh
cid statusline install        # the status line feeds and triggers auto-switching
cid auto add 1                # enroll accounts by index or name (defaults: 5h 90%, week 95%)
cid auto add personal --5h 85 --week 90
cid auto principal work       # wins ties and is the account to return to
cid auto reserve spare        # used only when nobody else is under its lines
cid auto on                   # nothing switches until you turn it on
cid auto                      # accounts, lines, pending switch, recent switches
cid auto defaults --5h 85     # change the default lines
cid auto off                  # stop switching at once
```

`cid auto remove <n|name>` takes an account out. The same commands work as `/account auto ...`, and `cid auto --json` prints the state for scripts. The plugin hook is a single `cid hook` entry, so `cid` must be on `PATH`; with nothing pending it reads one small file and exits.

**Limits.** The rule trusts `cid`'s own readings, so it assumes this machine is the only place these accounts are used. Usage on another machine or app is not seen, and the cache then undercounts. If a switch fails, the prompt still goes through and the status line says so.

## Switching safely

- `cid` waits for Claude Code's own credential locks (`.oauth_refresh.lock`, `.claude.lock`, `.claude.json.lock`) before it reads or writes a login, so a switch does not collide with a token refresh. If a lock stays busy, the switch fails with one line and changes nothing.
- It refuses to store an empty or unknown login into a profile.
- If verification fails after a switch, it restores the previous login.
- A login counts as expired 5 minutes before its real expiry time, so you are not switched into a token that dies right away.

## Configuration

```
cid config                     show paths and every setting (value and source)
cid config get language        print one value
cid config set language es     validate and save
cid config unset language      back to the default
```

The same commands work in the plugin, for example `/account config set language es`.

| Setting | Values | Default | Environment override | Meaning |
|---------|--------|---------|----------------------|---------|
| `language` | `auto`, `en`, `es` | `auto` | `CID_LANG` | Interface language. `auto` follows the detection order in [Language](#language). |

A setting is resolved in this order: the environment variable, then `config.json`, then the default. `cid config` shows which one applied in the `(source)` column (`environment`, `file` or `default`).

**Files.** Settings live in `config.json`, next to `profiles.json`, in `os.UserConfigDir()/cid` (`%AppData%\cid` on Windows, `~/.config/cid` on Linux, `~/Library/Application Support/cid` on macOS) or in `$CID_CONFIG_DIR` when set. It is a flat JSON object such as `{"language": "es"}`. Both files are validated against an embedded JSON Schema every time they are read.

**Hand edits and recovery.** Both files are written atomically, and each successful write refreshes a `.bak` copy of the last good state. If you edit a file by hand and break it, cid keeps working:

- A file that is not valid JSON, has the wrong types, a profile name that is not allowed or repeated, or a repeated index is moved to `<file>.corrupt-<timestamp>`. cid then restores the `.bak` copy when that is valid, or starts from an empty file, and prints one warning line on stderr with the location of the moved file.
- An `active` marker that names a profile that does not exist is cleared, with a warning.
- In `config.json`, an unknown key is ignored with a warning (and kept when cid rewrites the file), and an invalid value is ignored so the default applies, also with a warning.
- Recovery never deletes keyring entries. If profiles seem to be missing after a recovery, re-add them with `cid add`, or copy the entries you need from the `.corrupt-*` file.

**JSON contract.** `cid config --json` and `cid config get <key> --json` are the stable interface for tools:

```json
{
  "paths": {"profiles": "...", "config": "...", "claude_dir": "..."},
  "settings": [
    {"key": "language", "value": "es", "source": "file", "default": "auto", "kind": "enum",
     "allowed": ["auto", "en", "es"], "env": "CID_LANG", "description": "..."}
  ]
}
```

`source` is `env`, `file` or `default`. `get --json` prints one object of the `settings` array. `cid uninstall` also removes `config.json` and the `.bak` and `.corrupt-*` copies.

## Language

`cid` speaks English and Spanish. Unless `language` is set to `en` or `es` (see [Configuration](#configuration)), the language is picked in this order, and the first match wins:

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
