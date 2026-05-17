# Distribution

Status: active

`fincrawl` is a CLI/library repo. Every merge to `main` should already be
release-ready.

## Local Contract

`./scripts/verify` is the source of truth for CI:

```bash
./scripts/verify
```

It runs module tidy checking, tests, vet, race tests, the offline smoke flow,
repo guardrails, and whitespace checks.

Release config changes also need:

```bash
./scripts/release-check
```

## GitHub Actions

The GitHub workflow stays thin:

- Pull requests and pushes run the `verify` job.
- Pushes to `main` run `release` only after `verify` passes.
- Release jobs use `contents: write`; verify jobs use `contents: read`.
- Third-party release actions are pinned by full commit SHA.
- Checkout credentials are not persisted.
- Release dependency caches are disabled.

The default branch is protected by the `protect-main-release-flow` ruleset. It
blocks deletion and non-fast-forward updates, requires the pull request path,
requires review thread resolution, and requires a strict `verify` status check.

## Versioning

Semantic-release reads Conventional Commit metadata on `main` and publishes
patch releases on the `0.0.x` bootstrap line.

During bootstrap, these commit types produce patch releases:

```text
feat fix perf revert build ci docs test refactor chore
```

Release tags use the `v${version}` format.

## Artifacts

When semantic-release publishes a new GitHub Release, GoReleaser builds
`fincrawl` binaries for Linux, macOS, and Windows.

GoReleaser appends artifacts to the release and fails closed if an artifact with
the same name already exists. `scripts/release-check` blocks re-enabling mutable
release assets.

## Security Model

The release flow must not use live provider credentials, tenant store secrets,
age identities, 1Password item paths, or tenant-derived artifacts.

Release hardening rules:

- No `pull_request_target` release flow.
- No unpinned release actions.
- No shared dependency cache in the privileged release job.
- No persisted checkout credentials.
- No replacement of existing release assets.
- No tenant data in release artifacts.

Run `go run ./cmd/fincrawl guard --json` before publishing changes that touch
docs, examples, fixtures, or archive paths.

## Collaboration Template

Pull requests use a compact template that asks for summary, verification,
tenant-data boundary confirmation, and release-note relevance. Keep deeper
review prompts in the template instead of duplicating them across top-level
docs.
