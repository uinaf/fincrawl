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
