# Agent Guide

`fincrawl` is a generic uinaf crawler/archive CLI. Keep tenant credentials,
tenant identifiers, real transcript data, generated snapshots, logs, reports,
screenshots, and transcript-derived examples out of this repository.

## Fast Start

```bash
./scripts/smoke
./scripts/verify
```

Use `FINCRAWL_HOME=<scratch-dir>` for local CLI runs that should not touch the
default user state. The smoke script already does this.

## Checks

- `./scripts/smoke` exercises the real CLI offline with synthetic fixtures.
- `./scripts/verify` runs module tidy checking, tests, vet, race tests, smoke,
  guardrails, and whitespace checks.
- `./scripts/release-check` validates the GoReleaser config used by CI.
- `go run ./cmd/fincrawl guard --json` blocks committable plaintext archives,
  generated artifacts, secret-looking values, real 1Password references,
  provider URLs, and transcript-like data outside synthetic fixtures.

## Hooks

Enable the repo hook path when working locally:

```bash
git config core.hooksPath .git-hooks
```

The pre-push hook runs tests, vet, smoke, and guardrails.
The commit-msg hook enforces Conventional Commit subjects for semantic-release.
