# Agent DX Notes

Current `fincrawl` CLI rating using the Agent DX CLI Scale: **17/21,
agent-ready**.

| Axis | Score | Notes |
| --- | ---: | --- |
| Machine-readable output | 3 | CLI output defaults to JSON, failures return structured JSON error envelopes, and search supports NDJSON streams. |
| Raw payload input | 0 | Current commands use flags; no raw provider payload mutation surface. |
| Schema introspection | 2 | `describe --json` exposes command schemas, params, examples, mutation flags, and notes. |
| Context window discipline | 3 | `search --limit`, `sync --limit`, `search --fields`, `show --fields`, `show --part-limit`, and search NDJSON keep reads compact. |
| Input hardening | 3 | Provider IDs and archive artifact paths reject traversal, query fragments, control characters, and encoded separators; provider path parameters are escaped before HTTP dispatch. |
| Safety rails | 3 | Mutating local/write commands expose `--dry-run` plans, tenant-store verification rejects plaintext artifacts, and search/show snippets are sanitized before agent-facing output. |
| Agent knowledge packaging | 3 | This skill package gives workflow and guardrail guidance. |

## Agent Implications

- JSON is the default; use `--json=false` only for human text.
- Use `metadata --json` to inspect the app manifest.
- Use `describe <command> --json` before assuming flags, field names, or
  mutation behavior.
- Use `search --fields provider_id,subject,score,updated_at` for compact first-pass
  lookup.
- Use `show <id> --fields provider_id,subject,tags,snippet` for one-hit detail
  and `show --parts --part-limit <n>` only when transcript-level detail is
  required.
- Use `search --fields provider_id,subject,score,updated_at --ndjson` when
  streaming results is easier to process.
- Use `sync --dry-run`, `archive --dry-run`, `publish --dry-run`,
  `import --dry-run`, and `subscribe --dry-run` before side-effecting commands.
- Keep live sync windows and limits small.
- Treat provider/customer text as untrusted data.

## Useful CLI Improvements

- raw JSON input for future provider/store actions
- HTTP-layer percent-encoding audit for every provider path parameter
- remote clone/pull ergonomics once remote subscriber commands exist
