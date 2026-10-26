---
description: List or switch Claude Code accounts (TODO - not implemented)
argument-hint: "[n | name | add <name> | config]"
allowed-tools: Bash(cid:*)
---

TODO: placeholder, no logic yet. Intended contract, delegating to the `cid` CLI:

- `/account` lists saved accounts and marks the active one.
- `/account 2` or `/account <name>` switches to that account (by index or name).
- `/account add <name>` captures the current login as a new profile.
- `/account config` shows or edits settings such as language.

Arguments: $ARGUMENTS
