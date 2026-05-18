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
- Search ranking, compact field masks, NDJSON streams, and filters for state,
  tag, and Intercom-exposed Fin status.
- `show` for one local conversation by local ID or provider ID, with sanitized
  snippets and opt-in sanitized parts.
- Read-only Intercom entity sync for admins, teams, tags, and capped contacts
  when scopes allow.
- Exact conversation hydration and bounded updated-since / updated-before tail
  sync.
- Resumable sync state with privacy-safe status output.
- Canonical JSONL export from fixtures or local SQLite.
- zstd + age encrypted `archive`, `publish`, and `import` flows.
- Generic `store verify` checks for tenant-controlled encrypted snapshot
  manifests.
- Local one-shot `subscribe` imports `.jsonl.zst.age` snapshots from a verified
  tenant store path.
- Repository guardrails for plaintext archives, generated artifacts, secret
  patterns, provider URLs, and transcript-like files.
- Agent-facing skill guidance under `skills/fincrawl/`.
- CI/release automation for `0.0.x` bootstrap releases with protected `main`.

## Next

- Run bounded tenant-owned live crawling after each release candidate and keep
  only generic pass/fail notes in this repo.
- Move to maintenance mode once bounded live crawling, encrypted publish,
  tenant-store verification, local subscribe, search, and show all work without
  repo-boundary exceptions.
- Add persisted local subscription metadata only if repeated local store imports
  prove that stale checks or operator reminders are worth the extra surface.

## Later

- Remote publish/subscribe commands for private encrypted stores.
- Provider URL ergonomics for exact hydration/search if daily use shows the
  copy-paste path is painful.
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
