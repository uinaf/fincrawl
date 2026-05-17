# Contributing

`fincrawl` is a generic support-archive CLI. Contributions must keep the core
reusable and must not add tenant-derived data, credentials, local runtime state,
or transcript-derived examples.

## Setup

Install Go using the version in `go.mod`, then run the smoke check:

```bash
./scripts/smoke
```

`mise` is available as a convenience task index:

```bash
mise trust mise.toml
mise tasks
```

Use `FINCRAWL_HOME=<scratch-dir>` for manual CLI runs that should not touch the
default user state. Keep scratch homes under ignored paths such as `tmp/` or a
system temp directory.

## Run Locally

Use the source checkout while developing:

```bash
go run ./cmd/fincrawl doctor --offline
go run ./cmd/fincrawl sync --fixture testdata/synthetic
go run ./cmd/fincrawl search Morgan --fields provider_id,subject,updated_at
```

Live Intercom testing is optional and local. Load credentials through ignored
environment files or direct environment variables. Do not paste command output
containing tenant data into commits, docs, issues, or pull requests.

See [Local live smoke](docs/runbooks/local-live-smoke.md) for the safe local
workflow.

## Validation

Run the full local gate before opening a pull request:

```bash
./scripts/verify
```

For release configuration changes, also run:

```bash
./scripts/release-check
```

For docs-only changes, at minimum check the linked doc surface and run the repo
guard:

```bash
for f in README.md CONTRIBUTING.md SECURITY.md docs/architecture.md docs/roadmap.md docs/distribution.md docs/runbooks/open-source-readiness.md docs/runbooks/local-live-smoke.md docs/tenant-data-boundary.md docs/references/intercom-api.md skills/fincrawl/SKILL.md; do test -f "$f" || { echo "missing $f"; exit 1; }; done
go run ./cmd/fincrawl guard --json
```

## Data Boundary

Committed fixtures must be synthetic and intentionally small.

Do not commit:

- Provider tokens, API keys, bearer tokens, `.env` files, or real 1Password item
  paths.
- Tenant account IDs, workspace IDs, user/contact IDs, admin names, team names,
  tags, or tenant-specific config.
- Real conversation bodies, subjects, summaries, ratings, notes, screenshots,
  reports, plaintext snapshots, encrypted tenant snapshots, logs, SQLite stores,
  or cache directories.
- Fixtures copied from real conversations, even if redacted.

Use [Tenant data boundary](docs/tenant-data-boundary.md) as the source of truth
when a change touches credentials, live sync, snapshots, logs, fixtures, or
agent-facing examples.

## Pull Requests

Keep pull requests scoped. Include:

- What changed.
- How it was verified.
- Whether tenant-data boundaries, local state, or generated artifacts are
  affected.
- Any remaining manual proof needed.

The pull request template asks for the same evidence in a compact form.

## Commit Style

Use Conventional Commit subjects:

```text
<type>(<scope>): <subject>
```

Examples:

```text
feat(sync): persist updated-since cursors
docs(security): clarify tenant artifact boundary
```

The release pipeline uses Conventional Commit metadata to decide patch releases
on the `0.0.x` bootstrap line. See [Distribution](docs/distribution.md) for the
release model.
