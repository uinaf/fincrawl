# Architecture

Status: active

`fincrawl` is a local-first support conversation archive CLI for Intercom
workspaces, including conversation metadata that Intercom exposes for Fin
participation. The reusable repo owns generic crawling, storage, search,
artifact, guardrail, release, and agent-facing mechanics. Tenant credentials,
tenant config, runtime state, generated artifacts, logs, reports, screenshots,
and transcript-derived examples stay outside this repo.

## Current Surface

Core commands:

```bash
fincrawl doctor --offline
fincrawl metadata --json
fincrawl status --json
fincrawl sync --fixture testdata/synthetic
fincrawl sync --entities
fincrawl sync --updated-since 2h --limit 50
fincrawl sync --resume
fincrawl sync --conversation <id>
fincrawl search "billing refund" --json
fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out tmp/archive.jsonl.zst.age
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age
fincrawl import --identity <age-identity> --in snapshots/local.jsonl.zst.age
fincrawl guard --json
```

The CLI defaults to JSON output for agent use. Human text output is explicit.
Live provider credentials are optional and local; deterministic verification
uses synthetic fixtures.

## Package Shape

```text
cmd/fincrawl/          CLI entrypoint
internal/cli/          command parsing, JSON output, errors, Agent DX metadata
internal/config/       env and local config loading with redaction
internal/control/      machine-readable command descriptions and safe examples
internal/intercom/     provider API client boundary
internal/syncer/       fixture, entity, exact, and tail sync orchestration
internal/store/        SQLite schema, migrations, export, search, sync state
internal/archive/      canonical JSONL, zstd compression, age encryption
internal/guard/        preflight repository leak checks
internal/lock/         local write lock
```

`openclaw/crawlkit` is used where it fits local crawler mechanics. Provider API
shape, privacy rules, archive format, and support-search semantics stay in
`fincrawl`.

## Store

SQLite is the local archive source of truth. The schema stays migration-friendly
and small:

- `workspaces`
- `conversations`
- `conversation_parts`
- `contacts`
- `admins`
- `teams`
- `tags`
- `conversation_tags`
- `sync_state`
- `raw_blobs`
- `conversation_fts`

Search uses FTS with sanitized queries and a fallback path where needed. Raw
provider JSON is retained locally for replay and migration debugging, but it is
tenant data and must not be committed to this repo.

## Sync

Sync paths are deliberately separate:

- Fixture sync imports synthetic test data and exercises the real store/search
  path without credentials.
- Entity sync hydrates read-only provider metadata such as admins, teams, tags,
  and capped contacts when scopes allow.
- Exact sync hydrates one conversation by ID.
- Tail sync searches recently updated conversations, stores resumable sync
  state, then hydrates exact conversations.

Provider access must use supported APIs or official export paths only. Browser
scraping, undocumented endpoint crawling, credential-sharing workarounds, and
rate-limit bypasses are out of scope.

## Artifacts

Archive output is canonical JSONL streamed through zstd compression and explicit
age encryption. Supported artifact shape:

```text
*.jsonl.zst.age
```

Plaintext archives, databases, logs, snapshots, screenshots, reports, and
transcripts are ignored and blocked by `fincrawl guard`. Encrypted tenant
snapshots are still tenant data and belong outside this repo.

## Agent Use

Agents should load [the repo skill](../skills/fincrawl/SKILL.md) before using a
local archive. It describes binary discovery, read-only search, live sync
boundaries, portable store expectations, and current Agent DX conventions.
