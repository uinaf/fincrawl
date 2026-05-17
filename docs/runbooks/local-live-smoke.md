# Local Live Smoke

Status: active

Use this runbook to prove read-only Intercom access on a local machine without
committing tenant data, logs, snapshots, reports, or runtime config to this
repository.

## Boundary

Live smoke uses tenant-authorized credentials and official Intercom APIs only.
All runtime state must stay in an ignored or temporary `FINCRAWL_HOME`.

Do not copy command output into checked-in docs or issues when it contains
conversation subjects, participant names, contact data, tenant identifiers,
provider cursors, or transcript text.

## Setup

Create `.env.local` through a local 1Password-compatible env flow. The committed
template must stay generic; real item paths belong only in ignored local files.

```bash
cp .env.local.example .env.local.tpl
op inject -i .env.local.tpl -o .env.local
chmod 600 .env.local
```

The CLI loads ignored `.env.local` automatically. It reports credential
presence only, never token values or 1Password item paths.

## Safe Default Smoke

The default script uses a temporary `FINCRAWL_HOME` and removes it on success,
runs offline diagnostics, hydrates read-only admins, teams, and tags, and checks
local status. It does not list contacts and does not hydrate conversations
unless explicitly requested.

```bash
./scripts/local-live-smoke
```

To keep runtime state for inspection, point it at an ignored temp path:

```bash
FINCRAWL_LIVE_HOME=/tmp/fincrawl-live-smoke ./scripts/local-live-smoke
```

## Contact Scope Smoke

Contact listing is intentionally opt-in because it can expose broad user data.
Use a small cap.

```bash
FINCRAWL_LIVE_CONTACT_LIMIT=10 ./scripts/local-live-smoke
```

Equivalent direct command:

```bash
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --entities --contacts --limit 10 --json
```

If the Intercom token does not have an optional read scope, `sync --entities`
returns a warning for that entity type and continues with the scopes that are
available.

## Conversation Smoke

Use exact hydration when you have an approved test conversation ID. Use a
temporary or ignored home.

```bash
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --conversation <approved-test-conversation-id> --json
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl search "<approved-test-query>" --json
```

For a bounded recent tail sync, keep the window and limit small:

```bash
FINCRAWL_LIVE_UPDATED_SINCE=2h FINCRAWL_LIVE_CONVERSATION_LIMIT=5 ./scripts/local-live-smoke
```

When a limited or interrupted updated-since run leaves an active window in local
SQLite, fresh updated-since runs are refused until the active window is
continued:

```bash
FINCRAWL_HOME=/tmp/fincrawl-live-smoke go run ./cmd/fincrawl sync --resume --json
```

`status --json` reports sync-state timestamps and resume availability. It shows
booleans for opaque provider markers and page cursors, not the raw provider IDs
or cursor values.

## Cleanup

Delete temporary homes when manual proof is done:

```bash
rm -rf /tmp/fincrawl-live-smoke
```

Before committing any follow-up work, run:

```bash
go run ./cmd/fincrawl guard --json
```
