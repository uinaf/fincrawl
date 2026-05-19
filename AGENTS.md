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
- `./scripts/verify` runs module tidy checking, tests with coverage report
  (enforced at `FINCRAWL_COVERAGE_FLOOR=80` by default; target is 90),
  vet, race tests, smoke, guardrails, release-check, lint, and whitespace
  checks. This is the canonical one-shot. Override the floor via
  `FINCRAWL_COVERAGE_FLOOR=N ./scripts/verify` when ratcheting.
- `./scripts/lint` runs `staticcheck`, `gofumpt -l`, `govulncheck`, and
  `gosec` with pinned versions. Auto-installs the tools on first run.
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

## Docs

- [Architecture](docs/architecture.md): CLI, storage, sync, artifact, and
  agent-use shape.
- [Tenant data boundary](docs/tenant-data-boundary.md): what must stay out of
  this repo.
- [Distribution](docs/distribution.md): CI, release, and versioning contract.
- [Roadmap](docs/roadmap.md): current product surface and next work.
- [Local live smoke](docs/runbooks/local-live-smoke.md): bounded live Intercom
  proof.
- [Agent skill](skills/fincrawl/SKILL.md): how agents should use installed
  archives.

`CLAUDE.md` is a symlink to this file; keep one authored agent guide.
