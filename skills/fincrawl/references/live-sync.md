# Live Sync

Live sync touches the provider API and may store tenant-derived data in the
local archive. Use it only when the user requests it or when local data is stale
and the user approves refresh.

## Safe Order

Start with read-only metadata:

```bash
fincrawl sync --entities --dry-run
fincrawl sync --entities
```

Use contacts only when needed and keep it capped:

```bash
fincrawl sync --entities --contacts --limit 50 --dry-run
fincrawl sync --entities --contacts --limit 50
```

Use a short tail window for recent issues:

```bash
fincrawl sync --updated-since 2h --limit 50 --dry-run
fincrawl sync --updated-since 2h --limit 50
```

Use exact hydration when the user gives a conversation/provider ID:

```bash
fincrawl sync --conversation <provider-conversation-id> --dry-run
fincrawl sync --conversation <provider-conversation-id>
```

The dry run validates flags, parses windows and IDs, reports the planned write
target, and does not call the provider API.

Pass only the provider ID, not a URL. The CLI rejects path traversal,
whitespace, query strings, fragments, and percent-encoded path separators before
dispatching live requests.

## Stale Or Interrupted State

Check state first:

```bash
fincrawl status
```

If a tail sync says an active sync exists, resume instead of starting a new
window:

```bash
fincrawl sync --resume --dry-run
fincrawl sync --resume
```

## Local Live Smoke

Inside the source checkout, use the runbook script for bounded proof:

```bash
./scripts/local-live-smoke
```

Optional live proofs:

```bash
FINCRAWL_LIVE_CONTACT_LIMIT=10 ./scripts/local-live-smoke
FINCRAWL_LIVE_UPDATED_SINCE=2h FINCRAWL_LIVE_CONVERSATION_LIMIT=5 ./scripts/local-live-smoke
```

## Boundaries

- Never print raw tokens or credential source paths.
- Do not create screenshots, logs, reports, fixtures, or summaries containing
  tenant support data unless the user explicitly asks and the destination is
  tenant-controlled.
- Respect provider rate limits and scope-denied warnings.
- Do not bypass official APIs or automate provider UI scraping.
