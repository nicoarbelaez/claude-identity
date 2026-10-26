---
name: account
description: Switch, list, add or remove Claude Code accounts through the cid CLI.
argument-hint: "[n|name] | list | add <name> | remove <n|name> | current | config | version"
disable-model-invocation: true
allowed-tools: Bash(cid) Bash(cid *)
model: haiku
effort: low
---

The instant path for `/account` is the plugin's `UserPromptExpansion` hook (`cid hook`), which answers successful commands without a model turn. This body runs only when `cid hook` could not start or `cid` failed.

Output of `cid $ARGUMENTS`:

!`cid $ARGUMENTS 2>&1 || true`

Show the output above verbatim. If it contains a Remote Control warning, keep it. If it is an error, add one or two sentences on how to fix the command, based on the usage text. If it is empty or says "command not found", tell the user in one sentence to install `cid` with the install command from the project README (https://github.com/nicoarbelaez/claude-identity#installation). Do not run any tool.
