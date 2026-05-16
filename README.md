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
go test ./...
go run ./cmd/fincrawl doctor --offline --json
go run ./cmd/fincrawl sync --fixture testdata/synthetic --json
go run ./cmd/fincrawl search login --json
go run ./cmd/fincrawl archive --fixture testdata/synthetic --recipient age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl --out tmp/fincrawl-smoke.jsonl.zst.age --json
go run ./cmd/fincrawl guard
```

Use `FINCRAWL_HOME=tmp/fincrawl-home` to keep local CLI smoke state inside the
ignored repo `tmp/` directory. The archive command above uses a synthetic public
age recipient for smoke tests only. The CLI may read ignored `.env.local` for
`FINCRAWL_AGE_RECIPIENT`; it does not shell out to 1Password.
