# Tenant Data Boundary

Status: draft

`fincrawl` is reusable infrastructure. A tenant may use it for local/manual
testing or later production archive jobs, but tenant-derived data stays outside
this repository.

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

Encryption recipients may be native `age1...` recipients or SSH public keys
accepted by age. Private age identities and private SSH keys must stay outside
the repo. Import dry-runs still need a private decrypt identity because they
validate encrypted record contents and counts.

## Fixture Rule

Committed fixtures must be obviously synthetic. Use invented names, invented
messages, invented IDs, and small deterministic examples that exercise parser,
sync, archive, and search behavior without resembling real support transcripts.
