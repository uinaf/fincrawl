# Roadmap

Status: active

This roadmap lists the current product direction. It is intentionally concise;
completed bootstrap notes and tenant-specific operating details do not belong in
this repo.

## Available Now

- Go CLI with JSON-first command output and structured errors.
- Offline `doctor`, `metadata`, `status`, `describe`, and `guard` commands.
- Synthetic fixture sync for deterministic tests.
- SQLite archive with FTS-backed search.
- Read-only Intercom entity sync for admins, teams, tags, and capped contacts
  when scopes allow.
- Exact conversation hydration and bounded updated-since tail sync.
- Resumable sync state with privacy-safe status output.
- Canonical JSONL export from fixtures or local SQLite.
- zstd + age encrypted `archive`, `publish`, and `import` flows.
- Repository guardrails for plaintext archives, generated artifacts, secret
  patterns, provider URLs, and transcript-like files.
- Agent-facing skill guidance under `skills/fincrawl/`.
- CI/release automation for `0.0.x` bootstrap releases.

## Next

- Choose a public license and complete the
  [open-source readiness](runbooks/open-source-readiness.md) checklist.
- Enable or confirm private vulnerability reporting before public visibility.
- Tighten local search result ranking and result fields from stored entity data.
- Improve import/search ergonomics for read-only subscribers.
- Add a small, generic tenant-store wrapper contract without committing tenant
  config or artifacts here.
- Add focused tests for more provider pagination, rate-limit, and resume edge
  cases.

## Later

- Remote publish/subscribe commands for private encrypted stores.
- Official cloud export ingestion for broad historical backfills.
- Attachment metadata and optional attachment-byte handling.
- Additional support providers behind the same archive boundary.
- TUI workflows if the CLI shape proves useful enough to justify them.

## Non-Goals

- Provider UI scraping or undocumented endpoint crawling.
- Write-back operations such as adding notes, tags, comments, or status changes.
- Training exports.
- Public or cross-tenant sharing of support data.
- Committed real tenant fixtures, summaries, screenshots, snapshots, logs, or
  transcript-derived examples.
