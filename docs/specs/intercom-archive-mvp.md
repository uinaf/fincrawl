# Intercom Archive MVP

Status: active draft

## Purpose

`fincrawl` is a reusable crawler/archive tool for support conversation systems,
starting with Intercom and Fin. It makes support conversations searchable,
syncable across machines, and usable by agents without forcing every lookup
through a live provider UI, provider MCP, or repeated live API calls.

Live/manual tenant proof may be run locally when credentials are available, but
the core project is not tenant-owned. Keep the generic crawler reusable and keep
tenant-derived state under tenant-controlled storage.

## Ownership Boundary

`uinaf/fincrawl` owns the generic tool: CLI, schemas, sync mechanics, local
SQLite archive, search, encryption pipeline, fake fixtures, tests, and docs.

Tenant-controlled storage owns tenant-specific credentials, config, crawl state,
encrypted snapshots, plaintext scratch data, logs, reports, screenshots, and any
transcript-derived examples. See [Tenant data boundary](../tenant-data-boundary.md)
for the durable rule.

## Product Shape

The target product behaves like a local-first support archive:

```bash
fincrawl init
fincrawl doctor
fincrawl metadata --json
fincrawl status --json
fincrawl sync --updated-since 30d
fincrawl sync --resume
fincrawl sync --conversation <id>
fincrawl search "billing refund" --json
fincrawl conversations --fin --unresolved
fincrawl publish --push
fincrawl subscribe <encrypted-store>
```

Daily use is cheap:

- One scheduled writer refreshes Intercom and publishes encrypted snapshots.
- Agents and humans subscribe to the private encrypted store.
- Read commands pull/import when stale, then query local SQLite.
- Exact misses can hydrate one conversation from Intercom when credentials are
  present.
- Unsupported or liveness-sensitive operations remain explicit live API calls.

## Data Scope

The MVP stores:

- Conversations and conversation parts.
- Minimal contacts needed for search and display.
- Admins, teams, assignees, tags, Fin participation, Fin status metadata,
  ratings, and resolution state when the provider exposes them.
- Raw provider JSON for replay and migration debugging.
- FTS over subject, body, parts, tags, assignees, and normalized participant
  names.

The next implementation slice should make this scope useful before adding more
distribution machinery: hydrate provider entities, normalize them into SQLite,
and expose them through local search/status output without leaking opaque
provider cursors or tenant identifiers.

The MVP defers:

- Attachment bytes.
- Broad contact or company enrichment.
- Writing tags, notes, or comments back to Intercom.
- Automated Fin quality scoring.
- Training exports.
- Public or cross-team sharing.

## Privacy And Security

Everything tenant-derived is private by default. Archive artifacts must be
compressed, then encrypted before Git-backed storage. Preferred artifact shapes:

```text
*.jsonl.zst.age
*.tar.zst.age
```

Use explicit age-encrypted artifacts. Public recipients may be committed when
appropriate; private decrypt keys stay outside GitHub, ideally in local secret
storage. Do not use `git-crypt` as the primary mechanism unless that decision is
revisited.

Preflight guardrails must block plaintext archive artifacts, transcript-like
files, tenant identifiers, and secrets before commit.

## Provider Compliance Boundary

`fincrawl` sync must use supported provider APIs or official provider export
paths only. Browser/UI scraping, automated provider UI crawling, undocumented
endpoint crawling, credential-sharing workarounds, and rate-limit bypasses are
out of scope.

For Intercom, use the official REST API for tail sync and exact hydration, and
prefer official cloud export for broad historical backfill when a tenant can
enable it. All live calls must use tenant-authorized credentials, request only
the data needed for the archive, honor provider scopes and rate limits, and keep
tenant-derived output under tenant-controlled storage.

## Architecture

Build a typed Go CLI/library skeleton around a local archive boundary:

```text
cmd/fincrawl/
internal/cli/
internal/config/
internal/control/
internal/intercom/
internal/lock/
internal/store/
internal/syncer/
internal/archive/
internal/crypto/
internal/search/
internal/tui/
```

Use `openclaw/crawlkit` as an implementation reference and dependency where its
mechanics fit: config paths, SQLite helpers, snapshots, git mirror mechanics,
sync state, output helpers, control/status payloads, and TUI primitives. Keep
provider-specific auth, API clients, object normalization, rate-limit policy,
and privacy rules in `fincrawl`.

Use the OpenClaw crawler family as architecture references:

- `discrawl` for provider client, syncer, store, search, share/import, and
  privacy-filter seams.
- `gitcrawl` for local-first agent cache behavior, exact hydration on miss,
  stale checks, and live fallback for liveness-sensitive operations.
- `telecrawl` for encrypted backup manifests and shard reuse decisions.
- `wacli` for store locking, read-only mode, bounded sync, and FTS query
  sanitization.

The first `fincrawl` archive writer should be local to this repo because the
required artifact shape is zstd plus age. `crawlkit/backup` is still useful as a
reference for age identities, recipients, manifests, and shard reuse, but it is
currently gzip plus age.

Use existing Intercom connector implementations as sync-strategy references, not
as product-shape references. Airbyte and Singer-style connectors generally solve
warehouse extraction, not local SQLite archives or encrypted tenant stores, but
their cursor handling is useful.

SQLite starts small and migration-friendly:

- `workspaces`
- `conversations`
- `conversation_parts`
- `contacts`
- `admins`
- `teams`
- `tags`
- `conversation_tags`
- `conversation_part_attachments`
- `sync_state`
- `raw_blobs`
- `conversation_fts`

Entity tables are provider-neutral where possible. Provider IDs are preserved in
provider-specific columns for replay and exact hydration, but command output
should prefer display names and stable local IDs unless a user explicitly asks
for provider IDs.

## Sync Strategy

Use three sync paths.

### API Tail Sync

Routine sync searches recently updated conversations, then hydrates exact
conversations:

```bash
fincrawl sync --updated-since 2h
fincrawl publish --push
```

Overlap windows slightly so late updates, edits, closes, assignment changes, and
rating updates are not missed.

For Intercom, prefer the conversations search endpoint for incremental tail
sync. Query on `updated_at`, sort deterministically, page with
`pages.next.starting_after`, and use a bounded lookback window. Store enough sync
state to recover from interrupted runs and timestamp collisions: at minimum the
successful high-water mark plus the last processed provider ID or page cursor.
Bounded or interrupted active windows resume explicitly with
`fincrawl sync --resume`; completing the window clears active state and advances
the high-water mark. Starting a fresh tail window while active state exists is
rejected unless an explicit abandon or force path is added later.

Treat conversation parts as hydration data. Search/list endpoints identify
changed conversations; retrieving a single conversation returns its
`conversation_parts` payload and supports `display_as=plaintext` for messages.
Store the raw provider JSON alongside normalized searchable rows.

Hydrate supporting entities through read-only provider APIs when scopes are
available:

- Admins and teams for assignee display, filtering, and future ownership
  analysis.
- Tags for searchable labels and conversation classification.
- Contacts/users only to the minimal extent needed for search and display.

Entity sync should tolerate unavailable scopes. Missing optional scopes should
produce explicit diagnostics and partial local search, not force broad
permissions or fail unrelated conversation sync.

Intercom rate limiting should be budget-aware. Honor 429 responses, read
`X-RateLimit-*` headers when present, throttle successful responses when
remaining budget is low, and keep detail hydration concurrency configurable.

### Exact Hydration

Refresh one conversation by ID:

```bash
fincrawl sync --conversation <id>
```

Use this for search misses, webhook follow-up, debugging, and agent workflows
that already know a conversation URL or provider ID.

### Historical Backfill

For broad history, prefer the provider's official cloud export path when the
tenant can enable it. API content export can be a fallback, but repeated export
jobs should not become the foundation unless live proof shows that path is
cleanest.

For Intercom specifically, cloud export can provide historical and periodic
conversation data to S3 or GCS. The API tail path still matters for exact
hydration, local development, and tenants that cannot enable cloud export.

## External References

Read [Intercom API reference](../references/intercom-api.md) before changing
Intercom sync code. It collects the public API docs and connector references
that inform the sync strategy.

## Store Sync

The tenant store is private and portable:

- Generated encrypted snapshot files only.
- One scheduled writer.
- Read-mostly subscribers.
- No manual edits.
- No secrets.
- No local runtime config.
- No plaintext transcript data in Git.

MVP store layout:

```text
manifest.json
snapshots/*.jsonl.zst.age
README.md
```

Subscriber flow:

```bash
fincrawl subscribe <store-url>
fincrawl search "login code expired"
```

Publish/subscribe should come after local entity hydration is useful enough to
carry across machines. The first publishable store should contain a manifest and
encrypted snapshot artifacts only; runtime config, credentials, plaintext
archives, local SQLite files, logs, and tenant reports remain outside the store.

The subscriber path should import encrypted snapshots into local SQLite without
requiring live Intercom credentials. Exact live hydration remains optional when
credentials are present and a caller explicitly requests it.

## Verification

Generic repo verification:

```bash
go test ./...
go test -race ./...
go run ./cmd/fincrawl doctor --offline
go run ./cmd/fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out <tmp>.jsonl.zst.age
```

Live/manual proof with a real tenant token is allowed only locally:

- Token comes from env or a 1Password-compatible local flow.
- No real output is committed.
- Logs redact token source and tenant identifiers.
- Generated tenant artifacts go only to tenant-controlled private storage.

## Done For MVP

The MVP is done when:

- `fincrawl doctor` validates config and redacts credential source.
- Synthetic fixture sync writes canonical JSONL.
- Local SQLite search works without live Intercom access.
- Artifact pipeline compresses and encrypts output.
- Preflight blocks plaintext archives, transcript-like data, secrets, and tenant
  identifiers.
- Docs state the tenant/data boundary clearly.
- CI runs without real credentials or real tenant data.

See [Bootstrap MVP plan](../plans/bootstrap-mvp.md) for the first implementation
slice.
