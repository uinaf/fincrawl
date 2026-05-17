# Bootstrap MVP Plan

Status: active

## Summary

Bootstrap `fincrawl` as a Go local-first support archive CLI. The first slice
implements synthetic fixture sync, SQLite + FTS search, Intercom sync shape,
canonical JSONL output, zstd+age encrypted artifact generation, and commit
guardrails. Local encrypted snapshot publish/import is implemented for local
store portability; Git-backed publish/push/subscribe remains designed but out
of scope for this slice.

The current bootstrapped repo has the first local archive path, live
conversation hydration, read-only entity hydration, resumable tail sync state,
privacy-safe `status --json`, DB-backed encrypted `publish`, encrypted
`import`, guardrails, and release automation. Tenant-controlled stores can wrap
`publish` and `import` locally for real encrypted artifacts, but remote
push/pull/subscriber automation stays deferred until the local loop has run for
a while.

## Next Slice

Prioritize crawl readiness and the remaining local usability work around
Intercom entity hydration, search quality, and local tenant-store ergonomics:

1. Prove the real local encrypted loop in tenant-controlled storage: publish an
   encrypted snapshot, configure a local decrypt identity outside Git, import
   into a scratch `FINCRAWL_HOME`, and search from that imported SQLite archive
   without live Intercom access.
2. Commit and push the generic `uinaf/fincrawl` implementation separately from
   any tenant-store wrappers, docs, or encrypted tenant artifacts.
3. Commit tenant-store helper scripts and docs separately. Decide tenant-side
   whether the first encrypted snapshot is kept as a local proof artifact or
   committed to tenant-controlled private storage.
4. Run the first bounded real crawl locally: hydrate entities, run a short
   updated-since window with a conservative limit, publish an encrypted
   snapshot, and confirm search still works from local SQLite.
5. Keep `sync --entities` as the explicit local command for admins, teams,
   tags, and capped contacts/users where API scopes are available.
6. Normalize more conversation entity references during exact hydration and
   tail sync without requiring every optional scope to be present.
7. Enrich search ranking and result display from the stored entity tables after
   enough synthetic edge cases exist.
8. Add a local periodic crawl/publish routine in tenant-controlled storage,
   guarded by dry-run defaults and explicit execute flags.
9. Expand live smoke commands to prove the read-only scope set locally, while
   keeping all tenant output in ignored/private runtime state.

Remote encrypted publish/subscribe follows this local slice. Build it only
after local search from hydrated SQLite is useful enough for agents and humans
without live Intercom access.

## Current Local Operation State

The local flow is:

```bash
fincrawl sync --updated-since 2h --limit 50
fincrawl publish --recipient <age-recipient> --out snapshots/latest.jsonl.zst.age
FINCRAWL_HOME=<scratch-home> fincrawl import --identity <age-identity> --in snapshots/latest.jsonl.zst.age
FINCRAWL_HOME=<scratch-home> fincrawl search "login code expired" --json
```

`publish` may run with a public age recipient or SSH public key. `import`
requires a private age identity or supported SSH private key via
`FINCRAWL_AGE_IDENTITY` or `--identity`; even `--dry-run` decrypts the snapshot
so it can validate records and report counts. Real tenant snapshots and
identities must live only in tenant-controlled private storage.

Before broad tenant crawling, complete this readiness sequence:

```bash
# In tenant-controlled storage, not in uinaf/fincrawl:
# 1. Add FINCRAWL_AGE_IDENTITY to the ignored local env file.
scripts/import-local snapshots/latest.jsonl.zst.age
FINCRAWL_IMPORT_EXECUTE=1 FINCRAWL_HOME=/tmp/fincrawl-import-test scripts/import-local snapshots/latest.jsonl.zst.age
FINCRAWL_HOME=/tmp/fincrawl-import-test scripts/fincrawl search "login code expired" --json

# Then run the first bounded crawl locally:
scripts/fincrawl sync --entities
scripts/fincrawl sync --updated-since 2h --limit 50
FINCRAWL_PUBLISH_EXECUTE=1 scripts/publish-local
```

The bounded crawl may use a tenant token, but all generated state, logs, and
snapshots remain in tenant-controlled private storage. Do not copy command
output or real search snippets back into `uinaf/fincrawl` docs or fixtures.

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
fincrawl sync --entities
fincrawl sync --entities --contacts --limit 10
fincrawl search "billing refund" --json
fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out <tmp>.jsonl.zst.age
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age
fincrawl import --in snapshots/local.jsonl.zst.age
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
5. Canonical archive writer and importer: serialize deterministic JSONL from
   fixture or store records, stream through zstd and age, and verify decrypt
   plus decompress/import round trips without plaintext intermediates.
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
- Intercom client dependency stance: the legacy official Go SDK is not a good
  default for the current bearer-token, versioned REST API shape. Keep the
  small internal client while the endpoint surface is tiny, and evaluate
  OpenAPI-generated Go code from Intercom's official OpenAPI repository before
  adding many more endpoints. Third-party Go SDKs may be useful references, but
  should not become core dependencies until they are reviewed for API version
  coverage, pagination, retry behavior, raw JSON access, and maintenance risk.
- Intercom entities: list and store admins, teams, tags, and minimal contacts or
  users when scopes allow. Treat unavailable optional scopes as degraded
  capability with clear diagnostics, not as permission pressure to grant broad
  write scopes. Never write entity examples copied from live tenant data.
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
- Search enrichment: index synthetic participant names, tags, assignees, state,
  rating, and Fin-like fields. JSON results should be useful for agents while
  staying privacy-aware: no credential source details, no opaque page cursors,
  and no provider cursor state.
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

`sync --entities` hydrates read-only supporting entities before or alongside
conversation sync. Admins, teams, and tags are included by default; contacts are
explicit and capped with `--contacts --limit N`. The command tolerates
unavailable optional scopes by returning warnings and importing the entity types
that are available.

`archive` writes encrypted artifacts from synthetic fixtures only. It must not
create plaintext JSONL or plaintext compressed files on disk, including
temporary files.

`publish` writes encrypted artifacts from the local SQLite archive. It reads
normalized tables and raw blobs, emits canonical JSONL, compresses with zstd,
and encrypts with age. It requires an explicit `.jsonl.zst.age` relative output
path and accepts `FINCRAWL_AGE_RECIPIENT`.

`import` reads a `.jsonl.zst.age` snapshot, decrypts it with
`FINCRAWL_AGE_IDENTITY` or an explicit `--identity`, then hydrates local SQLite
without live Intercom access. Use `--dry-run` to validate the snapshot and count
records without writing local state. Dry-run still requires the decrypt identity
because it validates the encrypted contents rather than only checking the path.

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

Entity fixtures should include fake admins, teams, tags, and contacts/users with
stable fake IDs and display names. They should exercise missing optional entity
lookups as well as complete entity joins.

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

## Next-Slice Non-Goals

- Attachment byte download or storage.
- Intercom writeback or mutation scopes.
- Broad company enrichment or analytics warehouse export.
- Git-backed publish/subscribe implementation before entity-backed local search
  is useful.
- Real tenant examples, even if encrypted or heavily redacted.

## Test Plan

Add tests that use only synthetic data:

- Intercom pagination and exact hydration with `httptest`.
- Incremental search state with timestamp collisions, overlapping lookback, and
  interrupted-run resume behavior.
- Intercom rate-limit handling for 429 responses and low remaining-budget
  headers.
- Fixture sync into SQLite and FTS-backed `search --json`.
- Entity hydration with fake admins, teams, tags, contacts/users, and missing
  optional scopes.
- Search enrichment for participant, tag, assignee, rating, state, and Fin-like
  fields.
- Canonical JSONL stability with deterministic fake records.
- zstd+age artifact output by generating a temp age identity,
  decrypting/decompressing, and comparing JSONL bytes.
- Encrypted publish/import round trip from one synthetic SQLite archive into a
  fresh local SQLite archive.
- Store lock behavior for write commands and read-only DB behavior for read
  commands.
- FTS query sanitization and LIKE fallback behavior.
- Guard failures for fake plaintext archive names, fake transcript-like files,
  fake secret strings, and fake tenant-like identifiers.

Run before handoff:

```bash
./scripts/verify
./scripts/release-check
```

For ad-hoc local work, `mise` wraps the same scripts and common Go commands:

```bash
mise trust mise.toml
mise tasks
mise run fixture-loop
mise run test
mise run guard
mise run verify
```

For local tenant-store proof, run the tenant-controlled wrapper scripts or the
equivalent CLI commands with ignored/private env files. Keep generated
encrypted snapshots out of `uinaf/fincrawl`.

## Acceptance Criteria

- A fresh checkout can run offline verification without live credentials.
- Synthetic fixture sync produces local searchable conversations.
- Search works from SQLite without live Intercom access.
- Archive output is encrypted and compressed with the `.jsonl.zst.age` shape.
- Local publish/import can move encrypted synthetic SQLite state without live
  Intercom access.
- Local tenant-controlled publish/import can move a real encrypted snapshot into
  a scratch `FINCRAWL_HOME` once the private decrypt identity is configured
  outside Git.
- Guardrails fail before tenant data, plaintext archives, or secrets can be
  committed.
- Docs link to [Tenant data boundary](../tenant-data-boundary.md) and the
  [Intercom archive MVP](../specs/intercom-archive-mvp.md).

## Current Local-Slice Acceptance Criteria

- Synthetic entity sync writes admins, teams, tags, and minimal contacts/users
  into SQLite.
- Conversation sync stores searchable participant rows and behaves clearly when
  optional entity scopes are unavailable.
- `search --json` returns richer local results from SQLite without live
  Intercom access.
- `status --json` remains privacy-safe and read-only.
- Local encrypted `publish` works from the hydrated SQLite archive.
- Local encrypted `import` works after a decrypt identity is configured outside
  Git, and imported archives are searchable without live Intercom access.
- Live/manual smoke can verify read-only entity scopes using ignored/private
  runtime state only.
- `./scripts/verify` and `fincrawl guard` pass without real credentials or
  tenant data.
