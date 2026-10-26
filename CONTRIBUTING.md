# Contributing

Thanks for your interest in `claude-identity`.

## Ground rules

- **Test-driven development.** Write a failing test first, make it pass, then refactor. Pull requests that change behavior must include tests.
- **Conventional Commits** (`feat:`, `fix:`, `chore:`, `docs:`, `test:`, `refactor:`, ...).
- **Signed commits are required.** Unsigned commits will be rejected by repository rules.
- **Pull requests only.** No direct pushes to `main`. CI must be green and a code owner must approve.
- Keep changes small and focused.

## Local checks

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

## Security

Never commit tokens, credentials, or real account data, including in tests and fixtures. Report vulnerabilities privately; see [SECURITY.md](SECURITY.md).
