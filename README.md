# fincrawl

`fincrawl` is a reusable local-first crawler/archive tool for support
conversation systems, starting with Intercom and Fin.

The project is the generic crawler core: CLI, schemas, local archive mechanics,
search, encryption pipeline, fake fixtures, tests, and docs. Tenant credentials,
runtime config, crawl state, logs, reports, screenshots, plaintext snapshots,
encrypted snapshots, and transcript-derived examples do not belong in this repo.

## Start Here

- [Intercom archive MVP](docs/specs/intercom-archive-mvp.md) describes the
  product shape, data scope, security model, and done criteria.
- [Bootstrap MVP plan](docs/plans/bootstrap-mvp.md) describes the first Go
  implementation slice.
- [Intercom API reference](docs/references/intercom-api.md) collects public API
  links and extraction patterns for agents implementing sync.
- [Tenant data boundary](docs/tenant-data-boundary.md) is the reusable policy
  for credentials, real artifacts, manual tenant testing, and committed
  fixtures.
- [Local live smoke](docs/runbooks/local-live-smoke.md) shows how to exercise
  read-only Intercom calls without committing tenant state.

## Development Boundary

Committed tests and fixtures must be fake or synthetic. Real tenant crawl state
and generated artifacts belong in tenant-controlled private storage outside this
repository, even when encrypted.

## Current Offline Slice

The bootstrapped CLI runs without live Intercom credentials and stores only
synthetic fixture data during deterministic checks:

```bash
./scripts/smoke
./scripts/verify
```

`mise` is available as a convenience task index for ad-hoc local work. The
underlying scripts remain the source of truth for verification:

```bash
mise trust mise.toml
mise tasks
mise run smoke
mise run fixture-loop
mise run verify
mise run guard
```

Use `FINCRAWL_HOME=tmp/fincrawl-home` to keep local CLI smoke state inside the
ignored repo `tmp/` directory. The archive command above uses a synthetic public
age recipient for smoke tests only. The CLI may read ignored `.env.local` for
`FINCRAWL_AGE_RECIPIENT`; it does not shell out to 1Password.

## Local Live Smoke

Live sync is intentionally manual and local. Use a read-only Intercom app token
from ignored `.env.local`; do not write tenant config or generated artifacts
inside this repository. The safe default smoke hydrates admins, teams, and tags
only; contact listing and conversation hydration are explicit opt-ins.

```bash
cp .env.local.example .env.local.tpl
op inject -i .env.local.tpl -o .env.local
chmod 600 .env.local
./scripts/local-live-smoke
```

For contact scope proof, keep the list capped:

```bash
FINCRAWL_LIVE_CONTACT_LIMIT=10 ./scripts/local-live-smoke
```

For bounded conversation proof, prefer a short window and explicit limit:

```bash
FINCRAWL_LIVE_UPDATED_SINCE=2h FINCRAWL_LIVE_CONVERSATION_LIMIT=5 ./scripts/local-live-smoke
```

See [Local live smoke](docs/runbooks/local-live-smoke.md) for the full local
workflow and cleanup notes.

## Current Local Slice

The current local slice focuses on entity hydration, useful local search, and
local encrypted snapshot portability:

- Hydrate and normalize Intercom admins, teams, tags, and capped contacts/users
  where the tenant-authorized token exposes them.
- Enrich SQLite and FTS with participant, assignee, tag, rating, state, and
  Fin-like metadata while keeping provider-specific raw JSON for replay.
- Provide local smoke checks for read-only scopes without writing tenant config,
  logs, snapshots, or generated artifacts into this repo.
- Publish local SQLite state as compressed age-encrypted JSONL with
  `fincrawl publish`.
- Import compressed age-encrypted JSONL into a fresh local SQLite archive with
  `fincrawl import`.

The local publish/import loop is the current portability path. Remote
publish/push/subscribe flows are intentionally deferred while tenant-controlled
local usage proves the archive is useful.

The default Intercom API version is `2.15`. Set
`FINCRAWL_INTERCOM_BASE_URL` only for a regional Intercom API host and
`FINCRAWL_INTERCOM_VERSION` only when intentionally testing another supported
API version.

## Agent Workflow

Agents that need to use `fincrawl` as a local support archive should load the
repo skill at [skills/fincrawl/SKILL.md](skills/fincrawl/SKILL.md). It covers
local search, bounded live sync, tenant-data boundaries, and current CLI Agent
DX guidance.

Useful agent-facing commands:

```bash
fincrawl describe search
fincrawl search "login code expired" --fields provider_id,subject,updated_at
fincrawl search "login code expired" --fields provider_id,subject,updated_at --ndjson
fincrawl sync --updated-since 2h --limit 50 --dry-run
fincrawl archive --fixture testdata/synthetic --recipient age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl --out tmp/snapshot.jsonl.zst.age --dry-run
fincrawl publish --recipient <age-recipient> --out snapshots/local.jsonl.zst.age --dry-run
FINCRAWL_AGE_IDENTITY=<age-identity> fincrawl import --in snapshots/local.jsonl.zst.age --dry-run
```

CLI output defaults to JSON, including structured error envelopes. Use
`--json=false` only for human-oriented text output.

`./scripts/smoke` runs a fast offline CLI proof against synthetic fixtures.
`./scripts/verify` runs the full local gate: module tidy check, tests, vet, race
tests, smoke, guardrails, and whitespace checks.
`./scripts/release-check` validates the GoReleaser release config.

Enable the repo hook path if you want the local pre-push gate:

```bash
git config core.hooksPath .git-hooks
```

The pre-push hook runs tests, vet, smoke, and guardrails. The commit-msg hook
enforces Conventional Commit subjects for semantic-release.

## Release

Pushes and pull requests run the verify job in GitHub Actions. Pushes to `main`
also run semantic-release; Conventional Commit messages decide whether a release
is needed. During bootstrap, releasable commit types advance the patch-only
`0.0.x` line. When a release is published, GoReleaser builds `fincrawl`
binaries for Linux, macOS, and Windows and appends them to the GitHub Release.
