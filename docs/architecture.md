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
fincrawl sync --updated-since 180d --updated-before 90d --limit 0
fincrawl sync --resume
fincrawl sync --conversation <id>
fincrawl search "billing refund" --json
fincrawl show <id> --fields provider_id,subject,tags,snippet
fincrawl search "billing refund" --state open --tag billing
fincrawl search "login" --fin-status resolved
fincrawl archive --fixture testdata/synthetic --recipient <age-recipient> --out tmp/archive.jsonl.zst.age
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age
fincrawl import --identity <age-identity> --in snapshots/local.jsonl.zst.age
fincrawl store verify <tenant-store-root>
fincrawl subscribe <tenant-store-root> --dry-run
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
internal/tenantstore/  generic encrypted tenant-store manifest checks
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

Search uses FTS with sanitized queries, result scores, compact field masks, and
a fallback path where needed. `show` resolves either local IDs or provider IDs;
conversation parts are opt-in and text snippets are sanitized before output. Raw
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

Tail sync widens Intercom search bounds by one second while keeping strict
provider operators. This makes user-facing updated-since / updated-before
windows inclusive at Intercom's second-level `updated_at` precision. Adjacent
windows can intentionally overlap on the boundary; SQLite upserts keep repeated
hydration idempotent.

Provider access must use supported APIs or official export paths only. Browser
scraping, undocumented endpoint crawling, credential-sharing workarounds, and
rate-limit bypasses are out of scope.

## Artifacts

Archive output is canonical JSONL streamed through zstd compression and explicit
age encryption. Supported artifact shape:

```text
*.jsonl.zst.age
*.tar.zst.age
```

Plaintext archives, databases, logs, snapshots, screenshots, reports, and
transcripts are ignored and blocked by `fincrawl guard`. Encrypted tenant
snapshots are still tenant data and belong outside this repo.

Tenant stores can be checked locally with `fincrawl store verify <path>`. The
verifier reads `manifest.json`, requires manifest snapshots to point at existing
compressed age-encrypted artifacts, and rejects plaintext archives, SQLite
stores, runtime state, logs, reports, screenshots, and transcripts.
When the store path is a Git checkout, ignored local scratch paths are skipped
so a developer's runtime state does not block verification of the committable
store contents. Manifest and referenced snapshot paths are still checked
directly and must not be symlinks.

`fincrawl subscribe <path>` is a local one-shot subscriber flow. It verifies a
tenant-controlled store, reads the manifest, and imports listed
`.jsonl.zst.age` snapshots into local SQLite. It does not clone, pull, push,
schedule jobs, or persist tenant-specific subscription config.

## Agent Use

Agents should load [the repo skill](../skills/fincrawl/SKILL.md) before using a
local archive. It describes binary discovery, read-only search, live sync
boundaries, portable store expectations, and current Agent DX conventions.
