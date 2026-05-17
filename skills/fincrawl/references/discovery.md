# Discovery And Readiness

Start with machine-readable local state:

```bash
fincrawl metadata
fincrawl describe
fincrawl doctor --offline
fincrawl status
```

Use `metadata` to discover canonical command examples, default config paths, and
privacy flags. Use `describe` to inspect command arguments, flags, field masks,
and mutation behavior as JSON. CLI output defaults to JSON. Use
`doctor --offline` before assuming credentials exist. Use `status` to decide
whether local SQLite has any archive data.

## Binary Fallback

Prefer the installed `fincrawl` binary. If it is unavailable and the current
working tree is the `uinaf/fincrawl` source checkout, use:

```bash
go run ./cmd/fincrawl <command>
```

Outside the source checkout, do not invent paths or install tooling without user
approval. Tell the user `fincrawl` is not installed.

## Local State Isolation

Use `FINCRAWL_HOME` when a task needs isolated state:

```bash
FINCRAWL_HOME="$HOME/.local/share/fincrawl-agent" fincrawl status
```

Use tenant-specific `FINCRAWL_HOME` paths only when supplied by the user or
tenant-controlled repo docs/config. Do not encode tenant paths in generic repo
docs.

## Readiness Signals

Ready for local search:

- `status.state` is `ready`
- counts show conversations or raw blobs
- `describe search` lists the expected search flags
- search commands return JSON without errors

Ready for live sync:

- `doctor --offline` passes
- a live credential is present in redacted form
- the user has authorized live provider calls for the task

Empty archive is not a failure. It means search should report no local data and
ask before live refresh unless live refresh was requested.
