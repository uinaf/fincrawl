# Store Pattern

The intended shape mirrors a local-first archive pattern:

- reusable CLI/library in `uinaf/fincrawl`
- tenant-controlled private store repo for encrypted artifacts
- local decrypted SQLite/cache for agent reads

Agents should query the local SQLite through `fincrawl search`, not inspect
encrypted artifacts directly.

## Current Boundary

At this stage, `fincrawl` has local sync, local search/show, guardrails,
encrypted archive output, a generic tenant-store verifier, and a local one-shot
tenant-store subscriber. `archive` writes encrypted snapshots from synthetic
fixtures, `publish` writes encrypted snapshots from the local SQLite archive,
`import` hydrates local SQLite from one encrypted snapshot, and `subscribe`
hydrates local SQLite from snapshots listed by a verified local store manifest.

Do not invent remote clone, pull, push, or schedule mechanics until
`metadata --json` or `fincrawl --help` shows they exist.

For archive writes, validate first:

```bash
fincrawl archive --fixture testdata/synthetic --recipient age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl --out tmp/snapshot.jsonl.zst.age --dry-run
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age --dry-run
fincrawl import --in snapshots/local.jsonl.zst.age --dry-run
fincrawl store verify <tenant-store-root>
fincrawl subscribe <tenant-store-root> --dry-run
```

## Tenant Store Expectations

A tenant store repo should contain only tenant-controlled private material, such
as:

```text
README.md
AGENTS.md
manifest.json
snapshots/*.jsonl.zst.age
```

It should not contain plaintext JSONL, SQLite DBs, WAL/SHM files, logs,
screenshots, transcript fixtures, credentials, or local config.

`fincrawl store verify <tenant-store-root>` expects manifest snapshots to point
at existing `.jsonl.zst.age` or `.tar.zst.age` files with relative paths. It
rejects plaintext archive artifacts, local databases, runtime state, logs,
reports, screenshots, and transcripts.

`fincrawl subscribe <tenant-store-root>` verifies the same store and imports
listed `.jsonl.zst.age` snapshots into local SQLite. It does not pull from a
remote, persist subscription config, or write tenant artifacts into the generic
repo.

Use tenant-controlled repo docs/config for concrete store paths. The generic
`fincrawl` skill must stay tenant-neutral.
