# Contributing

`fincrawl` is a generic support-archive CLI. Contributions must keep the core
reusable and must not add tenant-derived data, credentials, local runtime state,
or transcript-derived examples.

## Setup

Use the repo scripts as the source of truth:

```bash
./scripts/smoke
./scripts/verify
```

`mise` is available as a task index for local convenience:

```bash
mise trust mise.toml
mise tasks
```

Use `FINCRAWL_HOME=<scratch-dir>` for manual CLI runs that should not touch the
default user state. Keep scratch homes under ignored paths such as `tmp/` or a
system temp directory.

## Data Boundary

Do not commit:

- Provider tokens, API keys, bearer tokens, `.env` files, or real 1Password item
  paths.
- Tenant account IDs, workspace IDs, user/contact IDs, admin names, team names,
  tags, or tenant-specific config.
- Real conversation bodies, subjects, summaries, ratings, notes, screenshots,
  reports, plaintext snapshots, encrypted tenant snapshots, logs, SQLite stores,
  or cache directories.
- Fixtures copied from real conversations, even if redacted.

Committed fixtures must be synthetic and intentionally small.

## Local Live Testing

Live Intercom testing is optional and local. Load credentials through ignored
environment files or direct environment variables. Do not paste command output
containing tenant data into commits, docs, issues, or pull requests.

See [Local live smoke](docs/runbooks/local-live-smoke.md) for the safe local
workflow.

## Verification

Before opening a pull request or committing a non-trivial change, run:

```bash
./scripts/verify
```

For release configuration changes, also run:

```bash
./scripts/release-check
```

For docs-only changes, at minimum check links and run the guard:

```bash
for f in CONTRIBUTING.md SECURITY.md README.md docs/architecture.md docs/roadmap.md docs/runbooks/open-source-readiness.md docs/runbooks/local-live-smoke.md docs/tenant-data-boundary.md docs/references/intercom-api.md skills/fincrawl/SKILL.md; do test -f "$f" || { echo "missing $f"; exit 1; }; done
go run ./cmd/fincrawl guard --json
```

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
on the `0.0.x` bootstrap line.

## Pull Requests

Keep pull requests scoped. Include:

- What changed.
- How it was verified.
- Whether tenant-data boundaries, local state, or generated artifacts are
  affected.
- Any remaining manual proof needed.
