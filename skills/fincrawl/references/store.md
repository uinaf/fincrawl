# Store Pattern

The intended shape mirrors a local-first archive pattern:

- reusable CLI/library in `uinaf/fincrawl`
- tenant-controlled private store repo for encrypted artifacts
- local decrypted SQLite/cache for agent reads

Agents should query the local SQLite through `fincrawl search`, not inspect
encrypted artifacts directly.

## Current Boundary

At this stage, `fincrawl` has local sync, local search, guardrails, and
encrypted archive output. `archive` writes encrypted snapshots from synthetic
fixtures, while `publish` writes encrypted snapshots from the local SQLite
archive and `import` hydrates local SQLite from an encrypted snapshot.

Do not invent remote `subscribe` or push mechanics until `metadata --json` or
`fincrawl --help` shows they exist.

For archive writes, validate first:

```bash
fincrawl archive --fixture testdata/synthetic --recipient age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl --out tmp/snapshot.jsonl.zst.age --dry-run
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age --dry-run
fincrawl import --in snapshots/local.jsonl.zst.age --dry-run
```

## Tenant Store Expectations

A tenant store repo should contain only tenant-controlled private material, such
as:

```text
README.md
AGENTS.md
manifest.json
snapshots/*.jsonl.zst.age
reports/*.json
```

It should not contain plaintext JSONL, SQLite DBs, WAL/SHM files, logs,
screenshots, transcript fixtures, credentials, or local config.

Use tenant-controlled repo docs/config for concrete store paths. The generic
`fincrawl` skill must stay tenant-neutral.
