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

## Live Intercom Smoke

Live sync is intentionally manual and local. Use a read-only Intercom app token
from ignored `.env.local`; do not write tenant config or generated artifacts
inside this repository.

```bash
cp .env.local.example .env.local.tpl
op inject -i .env.local.tpl -o .env.local
chmod 600 .env.local
go run ./cmd/fincrawl doctor --offline --json
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --conversation <synthetic-or-approved-test-id> --json
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl search "<query>" --json
```

For a bounded recent crawl, prefer a short window and explicit limit:

```bash
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --updated-since 2h --limit 5 --json
```

When a limited or interrupted updated-since run leaves an active window in local
SQLite, fresh updated-since runs are refused until the active window is
continued with:

```bash
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --resume --json
```

The default Intercom API version is `2.15`. Set
`FINCRAWL_INTERCOM_BASE_URL` only for a regional Intercom API host and
`FINCRAWL_INTERCOM_VERSION` only when intentionally testing another supported
API version.

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
is needed. During bootstrap, releasable commit types advance the patch-only
`0.0.x` line. When a release is published, GoReleaser builds `fincrawl`
binaries for Linux, macOS, and Windows and appends them to the GitHub Release.
