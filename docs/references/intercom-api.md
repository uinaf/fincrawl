# Intercom API Reference

Last checked: 2026-05-17

This is the repo-local reference for agents implementing Intercom support in
`fincrawl`. It records public API links and extraction patterns only. Do not add
tenant credentials, account identifiers, real response payloads, transcript
text, screenshots, logs, or generated artifacts here.

## Official Sources

- [Search conversations](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Conversations/searchConversations/)
- [Retrieve a conversation](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Conversations/retrieveConversation/)
- [List all conversations](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Conversations/listConversations/)
- [Conversation schema](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Conversations/Conversation/)
- [Conversation list item schema](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Conversations/ConversationListItem/)
- [List admins](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Admins/listAdmins/)
- [List teams](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Teams/listTeams/)
- [List tags](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Tags/listTags/)
- [List contacts](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Contacts/listContacts/)
- [Retrieve a contact](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Contacts/retrieveContact/)
- [Data export](https://developers.intercom.com/docs/references/rest-api/api.intercom.io/Data%20Export/)
- [Cloud conversation export](https://www.intercom.com/help/en/articles/7029721-export-conversations-data-to-amazon-s3)
- [Official OpenAPI repository](https://github.com/intercom/Intercom-OpenAPI)
- [Legacy Go SDK package](https://pkg.go.dev/github.com/Intercom/intercom-go)

The public docs expose a version picker. Implementation should pin a current
Intercom API version and make that version configurable instead of baking a
stale version into code.

## Go Client Stance

Use the small internal Intercom client for the bootstrap endpoint surface. The
legacy official Go SDK still documents the older app ID/API key flow and old API
references, so it is not the right default dependency for current bearer-token,
versioned REST calls.

Before adding broad endpoint coverage, evaluate generated Go code from
Intercom's official OpenAPI repository. Any generated or third-party client must
preserve these `fincrawl` requirements:

- configurable API version and base URL
- bearer-token auth without logging credentials
- 429 retry and low-budget throttling hooks
- access to raw provider JSON for replay and migrations
- cursor pagination control
- typed status errors for optional-scope warnings

## Compliance Boundary

Use official Intercom APIs and official export paths only. Do not implement
browser scraping, automated Intercom UI crawling, undocumented endpoint
crawling, credential-sharing workarounds, or rate-limit bypasses.

Live/manual calls must use tenant-authorized credentials and honor API scopes,
429 responses, and rate-limit headers. Historical bulk extraction should prefer
official cloud export when the tenant can enable it.

## Connector References

- [Airbyte Intercom source manifest](https://github.com/airbytehq/airbyte/blob/master/airbyte-integrations/connectors/source-intercom/manifest.yaml)
- [Airbyte Intercom custom components](https://github.com/airbytehq/airbyte/blob/master/airbyte-integrations/connectors/source-intercom/components.py)
- [Singer Intercom tap streams](https://github.com/singer-io/tap-intercom/blob/master/tap_intercom/streams.py)

Use these as extraction references only. They target warehouse-style pipelines,
not local SQLite archives, compressed and encrypted artifacts, or tenant store
guardrails.

## Sync Shape

Use `POST /conversations/search` for incremental tail sync. Query on
`updated_at`, sort deterministically, and page with
`pages.next.starting_after`.

Use `GET /conversations/{conversation_id}` for exact hydration. Conversation
parts are hydration data, not a separate global stream. Store the raw provider
JSON and normalize conversations, parts, participants, tags, assignees, ratings,
and Intercom-exposed Fin participation metadata into SQLite.

Hydrate admins, teams, tags, and minimal contacts/users through read-only API
calls when scopes allow. Treat these as support entities for search and display,
not as broad enrichment streams. Conversation sync should keep working with
clear degraded diagnostics when optional entity scopes are absent.

Prefer `display_as=plaintext` where Intercom supports it for searchable text,
but preserve the raw provider response so migrations and replay can recover
from parser bugs.

Historical backfill can use official cloud export when a tenant can enable it.
The API tail sync still handles recent updates, exact hydration, local
development, and tenants without cloud export.

## State And Pagination

Do not rely on a single timestamp bookmark. Persist enough state to recover from
interrupted runs and timestamp collisions:

- successful high-water mark
- active window start and end
- last processed conversation ID or page cursor
- provider cursor from `pages.next.starting_after`

Use an overlapping lookback window for routine sync so late updates, assignment
changes, edits, closes, and rating changes are not skipped.

## Rate Limits

Handle both hard and soft pressure:

- retry or pause on HTTP 429
- read `X-RateLimit-*` headers when present
- throttle successful responses when remaining budget is low
- bound detail hydration concurrency with a configurable worker count

Tests should cover 429 handling and low-budget successful responses.

## Test Fixtures

Committed fixtures must be synthetic. Build fake conversations that exercise:

- multiple conversations with the same `updated_at`
- pagination with `starting_after`
- interrupted sync resume
- exact hydration with `conversation_parts`
- plaintext display bodies plus raw provider JSON
- rate-limit headers and 429 responses
- tags, admins, contacts, assignments, ratings, and synthetic Fin participation
  metadata
- missing optional entity scopes and partial local search behavior

Never paste real tenant JSON into this repo, even redacted or encrypted.
