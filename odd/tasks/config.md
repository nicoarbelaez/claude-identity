# config

## Objective
Add `cid config` read/write (`get`, `set`, `unset`) backed by a typed settings registry and a `config.json` file, and make cid's JSON files (`config.json`, `profiles.json`) survive hand edits and corruption.

## Problem
`cid config` only prints paths and the detected language. The language cannot be chosen persistently, and a hand-edited or truncated `profiles.json` makes every command fail with a parse error.

## Why
Users want a persistent, validated way to set options (starting with the interface language), usable from the shell and through `/account config ...`. A broken JSON file must never lock the user out of their accounts.

## Scope
In: `internal/settings` (registry, store), `internal/fsutil` (atomic write, `.bak`, recovery, JSON Schema validation), resilient `profiles.json`, `cid config [get|set|unset] [--json]`, i18n EN/ES, uninstall cleanup of the new files, README "Configuration", SKILL.md argument hint.
Out: any terminal UI, Claude Code menu or keyboard shortcut. `--json` is the stable contract for future front ends.

## Constraints
- Validation uses JSON Schema (`github.com/santhosh-tekuri/jsonschema/v6`, Apache-2.0) with schemas embedded via `embed`.
- A broken file never panics or aborts a command: move it aside to `<name>.corrupt-<timestamp>`, restore the valid `.bak`, otherwise start empty, print one stderr warning. Keyring entries are never deleted by recovery.
- Unknown keys in `config.json` are ignored with a warning; an invalid value falls back to the default with a warning.
- Tests use `t.TempDir()`; no real credentials, keyring or user config dir.
- Go; English code, comments and docs; neutral Spanish for ES strings; one-line messages only.

## Checklist
| ID | Status | Task | Route | Commit |
|----|--------|------|-------|--------|
| C1 | [x] | Settings registry (`language`) validated against an embedded JSON Schema, with a registry/schema sync test | bounded writer | 22347ab |
| C2 | [x] | Resilient storage: shared atomic write with `.bak`, recovery helper, schema-validated `profiles.json` and `config.json`, recovery warnings | bounded writer | 9f67778, 55bd7da |
| C3 | [x] | CLI: `cid config`, `get`, `set`, `unset`, `--json`; wiring in `main.go`; uninstall removes the new files | bounded writer | 96708c8 |
| C4 | [x] | Docs: README "Configuration" and usage table, SKILL.md hint, help text | bounded writer | 6080a91 |

## Acceptance criteria
- `cid config set language es` persists to `config.json`; `cid config get language` prints `es`; `cid config unset language` restores the default. Precedence is env (`CID_LANG`) > file > default.
- Unknown key or invalid value exits 2 with a one-line message that lists the allowed values.
- `cid config --json` and `cid config get <key> --json` match the documented shape.
- Truncated JSON, wrong types, duplicate names, invalid names, dangling `active`, a valid `.bak`, an invalid `.bak`, unknown config keys and invalid enum values are all handled without failing the command.
- `/account config set language es` works through the plugin hook.

## Checks
`go build ./...` (also GOOS=linux and darwin), `go vet ./...`, `go test ./...`, `gofmt -l .`, golangci-lint, `go mod tidy` and `go mod verify`. Test-first: each task starts with failing tests (RED) before the implementation.

## Progress
All tasks done. Library: github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 (Apache-2.0, released 2026-08-06, last push 2026-10-18). Uninstall also removes config.json, .bak and .corrupt-* files.
