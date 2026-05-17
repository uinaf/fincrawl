# Open Source Readiness

Status: active public-repository checklist

`fincrawl` is public generic infrastructure. Use this checklist before major
public-facing changes, release pipeline changes, or any future history rewrite.

Tenant stores stay private. Making `uinaf/fincrawl` public does not make any
tenant credentials, tenant config, encrypted snapshots, plaintext scratch data,
logs, reports, screenshots, summaries, or transcript-derived examples public.

## Public Baseline

- `LICENSE` declares the public reuse terms.
- `SECURITY.md` points reporters to `dev@uinaf.dev` and GitHub private
  vulnerability reporting.
- The current tree, history, release pipeline, and docs should contain no
  tenant-derived residue.
- Repository metadata should stay aligned with the public CLI surface.

## Current Tree Audit

Run these checks from the repo root:

```bash
git status --short --branch
./scripts/verify
./scripts/release-check
go run ./cmd/fincrawl guard --json
for path in .env.local .env.local.tpl tmp; do
  git check-ignore -q "$path" || { echo "$path is not ignored"; exit 1; }
done
git check-ignore -v .env.local .env.local.tpl tmp
```

Search tracked files for tenant residue and secret-looking values:

```bash
secret_uri_pattern='op'"://"'[^<]'
tenant_pattern="${FINCRAWL_PUBLIC_AUDIT_TENANT_PATTERN:?set a private pipe-separated tenant residue pattern before running}"
secret_pattern="${tenant_pattern}|${secret_uri_pattern}|Bearer [A-Za-z0-9._=-]{20,}|INTERCOM_TOKEN=.*[^}> ]|FINCRAWL_INTERCOM_TOKEN=.*[^}> ]|BEGIN (RSA|OPENSSH|AGE) PRIVATE KEY|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9_]{36,}|github_pat_[A-Za-z0-9_]+|xox[baprs]-|sk-[A-Za-z0-9]{20,}"
git ls-files -z | xargs -0 rg -n \
  "$secret_pattern" \
  || true
```

Expected allowed hits are placeholder examples such as
`{{ op://<vault>/<item>/<field> }}` and synthetic guard-test strings. Any real
tenant name, provider account identifier, concrete 1Password item path, token,
private key, generated snapshot, transcript-like content, or provider URL is a
blocker.

## History Audit

Search reachable history before changing visibility:

```bash
secret_uri_pattern='op'"://"'[^<]'
tenant_pattern="${FINCRAWL_PUBLIC_AUDIT_TENANT_PATTERN:?set a private pipe-separated tenant residue pattern before running}"
secret_pattern="${tenant_pattern}|${secret_uri_pattern}|Bearer [A-Za-z0-9._=-]{20,}|INTERCOM_TOKEN=.*[^}> ]|FINCRAWL_INTERCOM_TOKEN=.*[^}> ]|BEGIN (RSA|OPENSSH|AGE) PRIVATE KEY|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9_]{36,}|github_pat_[A-Za-z0-9_]+|xox[baprs]-|sk-[A-Za-z0-9]{20,}"

git log --all --name-only --pretty=format: | sort -u | rg \
  '(^|/)(\.env|state|artifacts|snapshots|reports|logs|screenshots|transcripts)|\.jsonl|\.sqlite|\.db|\.age$|\.har$|\.log$' \
  || true

secret_uri_prefix='op'"://"
git log --all -S "$secret_uri_prefix" --source --pretty=format:'%h %D %s' -- . || true
git log --all -S 'Bearer ' --source --pretty=format:'%h %D %s' -- . || true
git log --all -S 'dG9r' --source --pretty=format:'%h %D %s' -- . || true

IFS='|' read -r -a tenant_needles <<< "$tenant_pattern"
for needle in "${tenant_needles[@]}"; do
  git log --all -S "$needle" --source --pretty=format:'%h %D %s' -- . || true
done

git rev-list --objects --all |
while read -r object path; do
  test "$(git cat-file -t "$object" 2>/dev/null)" = blob || continue
  git cat-file blob "$object" | LC_ALL=C rg -n "$secret_pattern" >/dev/null || continue
  echo "history residue candidate: ${path:-$object}"
done
```

Allowed history hits must be explainable as synthetic fixtures, placeholder
templates, or generic docs. Do not publish if history contains real tenant
material; rotate any exposed secrets and rewrite history before changing
visibility.

## Public Repo Metadata

Keep public-facing metadata aligned:

- Description: `Local-first support conversation archive CLI`
- Topics: `intercom`, `support`, `archive`, `sqlite`, `age-encryption`,
  `crawler`, `cli`
- Security policy: enabled or backed by `SECURITY.md`
- Issues: enabled only if maintainers are ready to receive public reports that
  obey the tenant-data rules
- Wiki and projects: disabled unless there is a concrete maintenance need

After the repo is public, private tenant-store workflows may stop using a
source checkout token for `uinaf/fincrawl`, but tenant provider credentials and
artifact storage must remain private.

## Visibility Check

Confirm visibility and metadata with:

```bash
gh repo view uinaf/fincrawl --json visibility,isPrivate,licenseInfo,securityPolicyUrl,repositoryTopics
```

Confirm the public release and CI surfaces still work without exposing tenant
data:

```bash
./scripts/verify
./scripts/release-check
```
