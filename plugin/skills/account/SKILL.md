---
name: account
description: Switch, list, add or remove Claude Code accounts, and read or change cid settings, through the cid CLI.
argument-hint: "[n|name] | list | add <name> | relogin <n|name> | remove <n|name> | current | config [get <key> | set <key> <value> | unset <key>] | version"
disable-model-invocation: true
allowed-tools: Bash(cid help), Bash(cid add *), Bash(cid relogin *)
model: haiku
effort: low
---

The `account-switch` plugin answers `/account` through its `UserPromptExpansion` hook (`cid hook`), which runs `cid` itself (including the browser login of `add` and `relogin`) and shows the result without a model turn. This body only runs when the hook could not answer.

You typed: /account $ARGUMENTS

Help text of `cid`:

!`cid help 2>&1 || true`

- If the hook context says `cid` failed, explain the cause in one or two sentences and offer to retry. Never list steps.
- If the help text above is empty or says "command not found", tell the user in one sentence to install `cid` following the Installation section of https://github.com/nicoarbelaez/claude-identity#installation.
- Only when the user explicitly asks in this conversation to retry an add or a relogin, run `cid add <name>` or `cid relogin <name>` (same flags as before) with Bash and `run_in_background`, then report the one-line result.
- Never read, print or ask for tokens or credential files, and never run any other command.
- Answer in the user's language.
