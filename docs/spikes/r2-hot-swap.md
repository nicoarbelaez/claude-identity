# Spike R2: does a running Claude Code session re-read swapped credentials?

Question: when another process replaces the OAuth credentials on disk, does an
already running `claude` session pick them up (no restart, no context loss)?
Secondary: does the running session write its old credentials back (see
anthropics/claude-code#23906)?

The kit is `spike/r2` (Go, stdlib only). It touches only the `claudeAiOauth`
key of `.credentials.json` and the `oauthAccount` key of `~/.claude.json`, and
never prints token values. Windows and Linux only; macOS (Keychain) is not
supported in the spike.

## Safety

- Profiles in `~/.cid-spike/profiles/` hold OAuth tokens in plaintext. This is
  acceptable for the spike only. Delete `~/.cid-spike` when finished.
- Never commit `~/.cid-spike`, backups, or any file from it.
- Take a backup first (step 0). `restore` puts both files back byte-for-byte.
- Use two accounts you are allowed to log in with. Do not paste token values
  anywhere; the kit prints only email, display name, and expiry.
- `CLAUDE_CONFIG_DIR`, if set, is honored (files are then read from that dir).

All commands below are PowerShell, run from the repository root.

## Steps

### 0. Backup

Close every Claude Code session first.

```powershell
go run ./spike/r2 backup
```

Note the printed `backup: <dir>` path.

### 1. Capture account A

You are currently logged in as account A.

```powershell
go run ./spike/r2 capture a
```

### 2. Capture account B

```powershell
claude
# inside: /login  -> sign in as account B, then exit
go run ./spike/r2 capture b
go run ./spike/r2 swap a
claude auth status
```

`claude auth status` must show account A. If not, stop and `restore`.

### 3. Experiment 1: live re-read

Terminal 1:

```powershell
claude
# /status           -> note the email (expect A)
# send a trivial prompt, e.g. "say hi"
```

Terminal 2 (while the session in Terminal 1 stays open):

```powershell
go run ./spike/r2 swap b
go run ./spike/r2 watch 120
```

Back in Terminal 1 (session S), while `watch` runs:

```text
send a trivial prompt, then run /status
```

Record: does S show B, still show A, or fail with an auth error? Did `watch`
print `WRITE-BACK`?

### 4. Experiment 2: write-back on exit

Keep `watch 120` running (or start `go run ./spike/r2 watch 120` again right
after the swap), then exit S (`/exit`). Afterwards:

```powershell
go run ./spike/r2 status
```

Record whether `watch` reported a write-back and which key differed
(`claudeAiOauth` and/or `oauthAccount`), and which profile `status` matches.

### 5. Experiment 3: fallback (resume after swap)

```powershell
go run ./spike/r2 swap b
claude --continue
# /status  -> which account? did it resume the same conversation?
```

### 6. Cleanup

If anything breaks:

```powershell
go run ./spike/r2 restore <backup-dir>
claude auth status
```

When done (this deletes plaintext tokens):

```powershell
Remove-Item -Recurse -Force "$HOME\.cid-spike"
```

## Results

| # | Question | Observed | Notes |
|---|----------|----------|-------|
| 1a | After `swap b`, does running S show account B on next prompt/`/status`? | | |
| 1b | Did S error (auth/401) or keep working? | | |
| 1c | Was conversation context preserved in S? | | |
| 2a | Did `watch` report WRITE-BACK while S was open? | | |
| 2b | Did exiting S write old credentials back? Which key(s)? | | |
| 2c | Account matched by `status` after S exited | | |
| 3a | `swap b` then `claude --continue`: account shown | | |
| 3b | Same conversation resumed? | | |
| 4 | Claude Code version (`claude --version`), OS | | |

Decision input for T1: if 1a is "yes" and 2b is "no", in-session hot swap by
file replacement is viable. If 1a is "no" but 3a/3b succeed, fall back to
swap plus `claude --continue`. If 2b is "yes", the swap engine must also guard
against write-back.
