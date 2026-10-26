---
name: account
description: Switch, list, add or remove Claude Code accounts through the cid CLI.
argument-hint: "[n|name] | list | add <name> | remove <n|name> | current | config | version"
disable-model-invocation: true
allowed-tools: Bash(cid help)
model: haiku
effort: low
---

The `account-switch` plugin answers `/account` through its `UserPromptExpansion` hook (`cid hook`), which runs `cid` itself and shows the result without a model turn. This body only runs when the hook could not answer. It never runs the user's command.

You typed: /account $ARGUMENTS

Help text of `cid`:

!`cid help 2>&1 || true`

- If the hook context says `cid` failed, explain the error and how to fix the command in one or two sentences, using the help text above.
- If the help text above is empty or says "command not found", tell the user in one sentence to install `cid` following the Installation section of https://github.com/nicoarbelaez/claude-identity#installation.
- Never run `cid` or any other tool.
