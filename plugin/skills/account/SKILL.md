---
name: account
description: Switch, list, add or remove Claude Code accounts through the cid CLI.
argument-hint: "[n|name] | list | add <name> | remove <n|name> | current | config | version"
disable-model-invocation: true
allowed-tools: Bash(cid) Bash(cid *)
model: haiku
effort: low
---

Output of `cid $ARGUMENTS`:

!`cid $ARGUMENTS 2>&1 || true`

Print the output above verbatim and add nothing else. If it contains a Remote Control warning, keep it. Do not run any tool.
