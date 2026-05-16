# fincrawl

`fincrawl` is a reusable local-first crawler/archive tool for support
conversation systems, starting with Intercom and Fin.

The project is the generic crawler core: CLI, schemas, local archive mechanics,
search, encryption pipeline, fake fixtures, tests, and docs. Tenant credentials,
runtime config, crawl state, logs, reports, screenshots, plaintext snapshots,
encrypted snapshots, and transcript-derived examples do not belong in this repo.

## Start Here

- [Intercom archive MVP](docs/specs/intercom-archive-mvp.md) describes the
  product shape, data scope, security model, and done criteria.
- [Bootstrap MVP plan](docs/plans/bootstrap-mvp.md) describes the first Go
  implementation slice.
- [Intercom API reference](docs/references/intercom-api.md) collects public API
  links and extraction patterns for agents implementing sync.
- [Tenant data boundary](docs/tenant-data-boundary.md) is the reusable policy
  for credentials, real artifacts, manual tenant testing, and committed
  fixtures.

## Development Boundary

Committed tests and fixtures must be fake or synthetic. Real tenant crawl state
and generated artifacts belong in tenant-controlled private storage outside this
repository, even when encrypted.

## Current Offline Slice

The bootstrapped CLI runs without live Intercom credentials:

```bash
./scripts/smoke
./scripts/verify
```

Use `FINCRAWL_HOME=tmp/fincrawl-home` to keep local CLI smoke state inside the
ignored repo `tmp/` directory. The archive command above uses a synthetic public
age recipient for smoke tests only. The CLI may read ignored `.env.local` for
`FINCRAWL_AGE_RECIPIENT`; it does not shell out to 1Password.

## Agent Workflow

`./scripts/smoke` runs a fast offline CLI proof against synthetic fixtures.
`./scripts/verify` runs the full local gate: module tidy check, tests, vet, race
tests, smoke, guardrails, and whitespace checks.
`./scripts/release-check` validates the GoReleaser release config.

Enable the repo hook path if you want the local pre-push gate:

```bash
git config core.hooksPath .git-hooks
```

The pre-push hook runs tests, vet, smoke, and guardrails. The commit-msg hook
enforces Conventional Commit subjects for semantic-release.

## Release

Pushes and pull requests run the verify job in GitHub Actions. Pushes to `main`
also run semantic-release; Conventional Commit messages decide whether a release
is needed. When a release is published, GoReleaser builds `fincrawl` binaries
for Linux, macOS, and Windows and appends them to the GitHub Release.
