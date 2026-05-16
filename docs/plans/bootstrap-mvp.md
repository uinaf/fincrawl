# Bootstrap MVP Plan

Status: draft

## Summary

Bootstrap `fincrawl` as a Go local-first support archive CLI. The first slice
implements synthetic fixture sync, SQLite + FTS search, Intercom sync shape,
canonical JSONL output, zstd+age encrypted artifact generation, and commit
guardrails. Git-backed publish/subscribe is designed but not implemented in this
slice.

## Implementation Plan

Create a Go module `github.com/uinaf/fincrawl` on Go `1.26.2`. Use
`github.com/openclaw/crawlkit@v0.5.2`, `github.com/alecthomas/kong`,
`modernc.org/sqlite`, `filippo.io/age`, and Go-native zstd support. Do not use
older forked crawlkit module paths.

Add CLI commands:

```bash
fincrawl doctor --offline
fincrawl metadata --json
fincrawl status --json
fincrawl sync --fixture testdata/synthetic
fincrawl sync --updated-since 2h
fincrawl sync --resume
fincrawl sync --conversation <id>
fincrawl search "billing refund" --json
fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out <tmp>.jsonl.zst.age
fincrawl guard
```

## Build Order

Implement the first slice in this order:

1. Module and CLI shell: initialize the Go module, wire Kong, add command
   structs, shared output helpers, and `doctor --offline`, `metadata --json`,
   and `status --json` stubs that do not require credentials.
2. Guardrails first: add `.gitignore` coverage for local databases, plaintext
   archives, compressed plaintext archives, decrypted artifacts, logs, and
   scratch output. Keep `.env.local` ignored and use `.env.local.example` only
   as a placeholder `op inject` template. Implement `fincrawl guard` before any
   fixture data lands so the repo protects itself from day one.
3. Store foundation: add migrations, SQLite open modes, write locking,
   synthetic workspace rows, raw blob storage, and fixture sync into normalized
   tables.
4. Search: add FTS population, FTS query sanitization, LIKE fallback, and
   `search --json` over synthetic fixture data.
5. Canonical archive writer: serialize deterministic JSONL from fixture or
   store records, stream through zstd and age, and verify decrypt plus
   decompress round trips without plaintext intermediates.
6. Intercom client shape: implement the provider boundary with `httptest`
   coverage for search pagination, exact hydration, rate limiting, and
   resumable incremental state. Live credentials remain optional and local.
   Use official APIs only; do not add browser scraping, UI automation, or
   undocumented endpoint clients.
7. Closeout checks: run formatting, vet, tests, race tests when feasible, CLI
   smoke commands, and `fincrawl guard`.

Implement these subsystems:

- Config and diagnostics: resolve local paths, detect credentials from env or a
  local secret-backed env flow, and report credential presence without exposing
  values or tenant-specific item paths. Use crawlkit config/control shapes for
  `doctor`, `metadata --json`, and `status --json`.
- CLI runtime: keep `cmd/fincrawl` thin, use Kong for parsing, and route command
  execution through `internal/cli`.
- Intercom provider: use conversations search for `updated_at` incremental tail
  sync, page with `starting_after`, hydrate exact conversations by ID for
  conversation parts, retain raw JSON, normalize into the local store, and keep
  provider-specific behavior out of crawlkit. Make the Intercom API version
  configurable with a pinned current default chosen at implementation time.
  Implement only supported REST API/export flows.
- Sync state: persist a successful high-water mark plus enough resume state to
  avoid skipped rows when a run dies halfway through or multiple conversations
  share the same timestamp. Use a small configurable lookback window for routine
  tail sync.
- Rate limiting: honor 429 responses, consume `X-RateLimit-*` headers when the
  provider returns them, throttle successful responses as remaining budget gets
  low, and keep detail hydration concurrency bounded and configurable.
- Store and search: create small migration-friendly SQLite tables, maintain FTS
  content for conversations and parts, and support JSON search output without
  live credentials. Use read-only SQLite opens for read commands, an exclusive
  local lock for write commands, sanitized FTS queries, and a LIKE fallback when
  FTS is unavailable.
- Archive: emit deterministic canonical JSONL from synthetic fixtures or local
  store records, then stream through zstd compression and age encryption to
  `.jsonl.zst.age` without writing plaintext intermediates. Implement this
  writer in `fincrawl` for the first slice; use `crawlkit/backup` only as a
  reference because its current shard format is gzip plus age.
- Guardrails: ignore local outputs and implement `fincrawl guard` to scan
  tracked, staged, and untracked files for plaintext archive artifacts,
  transcript-like data, tenant identifiers, and secret-looking values.

## Command Semantics

`doctor --offline` checks local config shape, dependency availability, writable
state directories, age recipient parsing, SSH recipient parsing where supported,
and credential presence by source only. It must never print token values, secret
item paths, tenant names, or account identifiers.

Credential lookup should read environment variables such as
`FINCRAWL_AGE_RECIPIENT` and `FINCRAWL_INTERCOM_TOKEN`. Local operators may
populate ignored `.env.local` with `op inject`; the CLI should not shell out to
1Password itself in the first slice.

`FINCRAWL_HOME` may point runtime state at an isolated local directory for
smoke tests and agent worktrees. This keeps CLI verification out of global user
state without changing XDG settings used by developer tooling.

`status --json` reports archive counts and privacy-safe sync-state visibility:
provider, cursor kind, high-water mark, active window timestamps, resume
availability, and booleans for whether opaque provider markers are present. It
must not print provider conversation IDs, page cursors, credential sources, or
tenant names.

`sync --fixture` imports only synthetic data and is the main deterministic
development path. It should exercise the same store and search code paths as
provider sync.

`sync --updated-since` uses the Intercom search shape described in
[Intercom API reference](../references/intercom-api.md). It persists an active
sync window before provider reads, writes hydrated conversations as they arrive,
and leaves resumable state when `--limit` stops before the window completes. It
is allowed to fail with a clear missing-credential diagnostic when no local
token is present.

`sync --resume` continues the active Intercom updated-since window from
`sync_state`. It reuses the saved page cursor and last processed provider ID so
an interrupted or intentionally bounded run does not skip rows in the active
window. A completed window clears active state and advances the high-water mark.
Fresh `sync --updated-since` runs are refused while active state exists.

`sync --conversation` hydrates one provider conversation by ID and writes the
same normalized rows and raw blobs as incremental sync. It is the exact-refresh
path for cache misses and debugging.

`archive` writes encrypted artifacts only. It must not create plaintext JSONL or
plaintext compressed files on disk, including temporary files.

`guard` is both a developer command and a future CI/pre-commit command. It scans
the Git index and worktree candidates, including untracked files, before commit.

## Data Contracts

Use stable provider-neutral IDs internally and preserve provider IDs in
provider-specific columns. Store raw provider JSON in `raw_blobs` with a content
hash and a type label so migrations can replay older payloads.

Canonical JSONL records should be deterministic:

- one JSON object per line
- sorted keys
- stable ordering by record type, provider ID, and timestamp
- UTC timestamps
- explicit schema version
- no local absolute paths
- no credential source details

Synthetic fixtures should cover provider edge cases without copying provider
payloads. Include same-timestamp updates, empty bodies, edited parts, closed and
reopened conversations, assignee changes, tags, ratings, and Fin-like metadata
with fake names and fake IDs.

Architecture references to internalize before writing code:

- [Intercom API reference](../references/intercom-api.md): public Intercom docs,
  connector patterns, cursor state, rate limits, and synthetic fixture cases.
- `discrawl`: provider client + syncer + store + share/import + privacy filter
  seams.
- `gitcrawl`: exact hydration on cache miss, stale checks, live fallback, and
  agent-facing local cache behavior.
- `telecrawl`: encrypted manifest/shard reuse and restore verification.
- `wacli`: store lock, read-only mode, bounded sync, and FTS query
  sanitization.

## First-Slice Non-Goals

- Attachment byte download or storage.
- Broad contact/company enrichment.
- Intercom writeback.
- Browser/UI scraping, automated provider UI crawling, undocumented endpoint
  crawling, credential-sharing workarounds, or rate-limit bypasses.
- Automated Fin quality scoring.
- Training exports.
- Git-backed publish, push, subscribe, and mirror reconciliation.
- Public or cross-team sharing workflows.
- Real tenant fixtures, logs, screenshots, reports, plaintext snapshots, or
  encrypted snapshots in this repo.

## Test Plan

Add tests that use only synthetic data:

- Intercom pagination and exact hydration with `httptest`.
- Incremental search state with timestamp collisions, overlapping lookback, and
  interrupted-run resume behavior.
- Intercom rate-limit handling for 429 responses and low remaining-budget
  headers.
- Fixture sync into SQLite and FTS-backed `search --json`.
- Canonical JSONL stability with deterministic fake records.
- zstd+age artifact output by generating a temp age identity,
  decrypting/decompressing, and comparing JSONL bytes.
- Store lock behavior for write commands and read-only DB behavior for read
  commands.
- FTS query sanitization and LIKE fallback behavior.
- Guard failures for fake plaintext archive names, fake transcript-like files,
  fake secret strings, and fake tenant-like identifiers.

Run before handoff:

```bash
GOWORK=off go mod tidy
git diff --exit-code -- go.mod go.sum
GOWORK=off go vet ./...
GOWORK=off go test -count=1 ./...
GOWORK=off go test -race ./...
go run ./cmd/fincrawl doctor --offline
go run ./cmd/fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out <tmp>.jsonl.zst.age
go run ./cmd/fincrawl guard
```

## Acceptance Criteria

- A fresh checkout can run offline verification without live credentials.
- Synthetic fixture sync produces local searchable conversations.
- Search works from SQLite without live Intercom access.
- Archive output is encrypted and compressed with the `.jsonl.zst.age` shape.
- Guardrails fail before tenant data, plaintext archives, or secrets can be
  committed.
- Docs link to [Tenant data boundary](../tenant-data-boundary.md) and the
  [Intercom archive MVP](../specs/intercom-archive-mvp.md).
