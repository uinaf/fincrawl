# fincrawl

Local-first support conversation archive CLI for Intercom workspaces, including
conversation metadata that Intercom exposes for Fin participation.

`fincrawl` syncs support conversations into a private local SQLite archive,
searches them without going back through the Intercom UI, and exports portable
snapshots as compressed age-encrypted JSONL.

This repository is the generic crawler core. Tenant credentials, tenant config,
runtime state, crawl output, logs, reports, screenshots, plaintext snapshots,
encrypted tenant snapshots, and transcript-derived examples do not belong here.

## Install

Download a binary from the latest
[GitHub Release](https://github.com/uinaf/fincrawl/releases), or install from
source with Go:

```bash
go install github.com/uinaf/fincrawl/cmd/fincrawl@latest
```

To work from a checkout:

```bash
go run ./cmd/fincrawl version
./scripts/smoke
```

## Quick Start

Keep local state outside tracked paths:

```bash
export FINCRAWL_HOME=/tmp/fincrawl-home
fincrawl doctor --offline
```

Try the archive path with synthetic fixtures from a source checkout:

```bash
fincrawl sync --fixture testdata/synthetic
fincrawl search "billing refund" --fields provider_id,subject,updated_at
```

For live Intercom access, use a tenant-authorized read-only token from your
environment or ignored local env file:

```bash
fincrawl sync --entities
fincrawl sync --updated-since 2h --limit 50
fincrawl sync --updated-since 180d --updated-before 90d --limit 0
fincrawl search "login code expired" --fields provider_id,subject,updated_at
```

Use exact hydration when you already know a conversation ID:

```bash
fincrawl sync --conversation <intercom-conversation-id>
```

## Snapshots

Snapshots are compressed and encrypted before storage:

```bash
fincrawl publish \
  --recipient <age-recipient-or-ssh-public-key> \
  --out snapshots/local.jsonl.zst.age \
  --dry-run
```

Import validates and hydrates a local SQLite archive:

```bash
FINCRAWL_AGE_IDENTITY=<age-identity> \
  fincrawl import --in snapshots/local.jsonl.zst.age --dry-run
```

Plaintext archive output is local scratch data only. Tenant encrypted snapshots
still belong in tenant-controlled private storage, not in this repository.

## Commands

The CLI defaults to JSON output for agents and automation:

```bash
fincrawl describe --json
fincrawl describe search --json
fincrawl status --json
fincrawl guard --json
```

Common flows:

| Need | Command |
| --- | --- |
| Check local config | `fincrawl doctor --offline` |
| Sync metadata | `fincrawl sync --entities` |
| Sync a recent window | `fincrawl sync --updated-since 2h --limit 50` |
| Backfill a bounded historical window | `fincrawl sync --updated-since 180d --updated-before 90d --limit 0` |
| Hydrate one conversation | `fincrawl sync --conversation <id>` |
| Search local archive | `fincrawl search "<query>" --fields provider_id,subject,updated_at` |
| Filter search results | `fincrawl search "<query>" --state open --tag billing` |
| Find Fin-status matches | `fincrawl search "<query>" --fin-status resolved` |
| Export encrypted snapshot | `fincrawl publish --recipient <recipient> --out snapshots/local.jsonl.zst.age` |
| Import encrypted snapshot | `fincrawl import --identity <identity> --in snapshots/local.jsonl.zst.age` |
| Check repo guardrails | `fincrawl guard --json` |

## Docs

| Need | Read |
| --- | --- |
| Understand the CLI and storage design | [Architecture](docs/architecture.md) |
| See what exists and what is next | [Roadmap](docs/roadmap.md) |
| Keep tenant data out of the repo | [Tenant data boundary](docs/tenant-data-boundary.md) |
| Run bounded live Intercom checks | [Local live smoke](docs/runbooks/local-live-smoke.md) |
| Work on releases and CI | [Distribution](docs/distribution.md) |
| Implement Intercom sync safely | [Intercom API reference](docs/references/intercom-api.md) |
| Use fincrawl as an agent skill | [Agent guide](skills/fincrawl/SKILL.md) |
| Contribute changes | [Contributing](CONTRIBUTING.md) |
| Report a vulnerability | [Security](SECURITY.md) |

## Contributing

Run the local gate before sending changes:

```bash
./scripts/verify
```

See [Contributing](CONTRIBUTING.md) for setup, validation, commit style, and the
tenant-data contribution boundary.

## Security

Report vulnerabilities privately. See [Security](SECURITY.md).

## License

MIT. See [License](LICENSE).
