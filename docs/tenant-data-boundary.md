# Tenant Data Boundary

Status: active

`fincrawl` is reusable infrastructure. A tenant may use it for local/manual
testing or later production archive jobs, but tenant-derived data stays outside
this repository.

This boundary is the same whether the generic repository is private or public.
Making `uinaf/fincrawl` open source does not change where tenant credentials,
state, encrypted snapshots, plaintext scratch data, logs, reports, screenshots,
summaries, or transcript-derived examples belong.

## Provider Access Boundary

Use supported provider APIs or official provider export paths only. Do not use
browser/UI scraping, automated provider UI crawling, undocumented endpoint
crawling, credential-sharing workarounds, or rate-limit bypasses.

Live/manual testing must use tenant-authorized credentials and must honor
provider scopes and rate limits.

## What This Repo May Contain

- Generic crawler code, schemas, migrations, and CLI contracts.
- Synthetic fixtures written by hand for tests.
- Example config that uses placeholder values only.
- Public age recipients or SSH public-key recipients when they are generic
  examples.
- Tests that run without real credentials or live tenant data.

## What This Repo Must Not Contain

- Intercom tokens, API keys, bearer tokens, cloud credentials, or `.env` files.
- Tenant account IDs, workspace IDs, team names, admin names, contact IDs, or
  tenant-specific config.
- Real transcript bodies, conversation URLs, summaries, ratings, tags, notes, or
  reports derived from tenant support data.
- Plaintext snapshots, encrypted tenant snapshots, logs, screenshots, local
  SQLite stores, cache directories, or generated artifacts.
- Fixtures copied from real conversations, even if redacted.

## Local Credential Loading

Live/manual testing may load credentials from environment variables or a local
1Password-compatible environment flow. The credential source should be reported
only in redacted form, such as "token present in environment", never as a raw
value or tenant-specific item path.

Use `op inject` for local environment files. Keep the committed
[example env template](../.env.local.example) generic with
`{{ op://<vault>/<item>/<field> }}` placeholders. Copy it to an ignored local
template, point that local template at real 1Password item fields, then inject
real values into ignored `.env.local`:

```bash
cp .env.local.example .env.local.tpl
op inject -i .env.local.tpl -o .env.local
chmod 600 .env.local
```

The committed template must contain placeholder `op://<vault>/<item>/<field>`
references only. Real 1Password item paths can reveal private workspace
structure and must stay local.

Repo verification and CI must run without live credentials. If a command needs
live credentials, it must fail clearly when they are absent and must not create
committable tenant artifacts.

## Artifact Rule

Archive artifacts are compressed, then encrypted before any Git-backed storage.
The preferred shapes are:

```text
*.jsonl.zst.age
*.tar.zst.age
```

Plaintext archive outputs are local-only scratch data and must be ignored or
blocked by preflight checks. Tenant encrypted artifacts are still tenant data and
belong in tenant-controlled private storage, not in `uinaf/fincrawl`.

Use `fincrawl store verify <tenant-store-root>` on tenant-controlled stores
before importing from them. The verifier expects `manifest.json` to reference
existing compressed age-encrypted snapshots with relative paths and rejects
plaintext archives, local databases, runtime state, logs, reports, screenshots,
and transcripts.

Use `fincrawl subscribe <tenant-store-root>` only for local one-shot imports from
a tenant-controlled store. The command verifies the store before import and
hydrates local SQLite from encrypted JSONL snapshots; it does not make the
generic repo a home for tenant store state, schedules, or remote credentials.

Encryption recipients may be native `age1...` recipients or SSH public keys
accepted by age. Private age identities and private SSH keys must stay outside
the repo. `import --dry-run` still needs a private decrypt identity because it
validates encrypted record contents and counts. `subscribe --dry-run` verifies
the store manifest and planned snapshots without decrypting or mutating local
SQLite.

## Fixture Rule

Committed fixtures must be obviously synthetic. Use invented names, invented
messages, invented IDs, and small deterministic examples that exercise parser,
sync, archive, and search behavior without resembling real support transcripts.
