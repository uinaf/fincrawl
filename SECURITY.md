# Security

`fincrawl` handles support archive mechanics, so security reports may involve
credentials, tenant identifiers, conversation metadata, or transcript content.
Do not disclose those details in public issues, pull requests, commits, logs, or
screenshots.

## Reporting

Use GitHub private vulnerability reporting. It must be enabled before this
repository is made public. If private reporting is unexpectedly unavailable,
open a minimal public issue asking for a private disclosure channel and include
no exploit details, secrets, tenant identifiers, or transcript text.

## Sensitive Data Rules

Never include the following in a report artifact unless a maintainer has
explicitly provided a private channel and asked for the minimum needed excerpt:

- Intercom tokens, API keys, bearer tokens, cloud credentials, or private age or
  SSH identities.
- Real 1Password item paths, vault names, or injected environment files.
- Tenant account IDs, workspace IDs, user/contact IDs, admin names, team names,
  tags, provider cursors, or conversation URLs.
- Conversation subjects, transcript bodies, summaries, notes, ratings,
  screenshots, generated snapshots, SQLite stores, logs, or reports.

Prefer synthetic reproductions. If a live tenant reproduction is unavoidable,
keep it local and describe the vulnerability class without copying tenant data
into the repo.

## Supported Surface

Security review should focus on:

- CLI credential loading and redaction.
- Provider API request construction and rate-limit handling.
- SQLite archive storage, import, search, and raw JSON retention.
- zstd + age artifact generation and import.
- Commit guardrails for generated artifacts, secret-looking values, provider
  URLs, and transcript-like content.
- Agent-facing skill guidance that may cause unsafe reads or writes.
