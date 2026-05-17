# Tenant Boundary

`fincrawl` is reusable infrastructure. Tenant data belongs to the tenant.

## Never Commit

- provider tokens, `.env` files, cloud credentials, or private key material
- tenant account IDs, workspace IDs, team names, admin names, or contact IDs
- transcript bodies, notes, ratings, tags, screenshots, summaries, or reports
- plaintext snapshots, encrypted tenant snapshots, logs, cache directories, or
  SQLite stores
- fixtures or examples copied from real support conversations, even when
  redacted

## Local Credential Loading

Credentials may come from environment variables or ignored local files such as
`.env.local`. Use redacted language only:

- good: "Intercom token is present"
- bad: raw token values
- bad: private 1Password item paths or vault names in committed docs

## File Writes

Before writing any file, decide whether it is generic or tenant-derived.

Generic files may live in `uinaf/fincrawl` when they contain only code, schemas,
synthetic fixtures, or generalized docs.

Tenant-derived files must stay in local ignored paths or tenant-controlled
private repos. This applies even when encrypted.

## Chat Output

Answer user questions from local search, but minimize sensitive detail. Prefer
conversation IDs and high-level summaries. Quote transcript text only when the
user asks for it and the current chat is an appropriate private surface.

