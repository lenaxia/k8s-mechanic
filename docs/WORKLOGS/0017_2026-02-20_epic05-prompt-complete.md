# Worklog: Epic 05 — Prompt

**Date:** 2026-02-20
**Session:** Write real prompt into ConfigMap (S01), verify envsubst (S03), code review, 5 gaps fixed
**Status:** Complete

---

## Objective

Implement the agent investigation prompt per `docs/DESIGN/lld/PROMPT_LLD.md`:
- S01: Write the full SRE investigation prompt into `deploy/kustomize/configmap-prompt.yaml`
- S02: Verify `docker/scripts/agent-entrypoint.sh` is fully implemented (was already done in epic03)
- S03: Verify `envsubst` substitution works correctly for all 9 required variables

---

## Work Completed

### S01 — Prompt content

Replaced the placeholder content in `deploy/kustomize/configmap-prompt.yaml` with the full
195-line prompt from PROMPT_LLD.md §2. Content includes:

- FINDING section with all 9 `${VAR}` placeholders
- ENVIRONMENT section describing in-cluster context and available tools
- 9 ordered INVESTIGATION STEPS (PR dedup check, kubectl inspect, k8sgpt analysis, GitOps
  manifest location, Flux/Helm state, root cause, validation, PR creation)
- PR BODY FORMAT template
- HARD RULES (7 rules — no main commits, no secrets, one PR per invocation, confidence
  threshold for investigation-only PRs)
- DECISION TREE

### S02 — Entrypoint (already complete from epic03)

`docker/scripts/agent-entrypoint.sh` was already fully implemented. Verified it matches
the PROMPT_LLD.md §3 specification.

### S03 — envsubst smoke test

Ran `envsubst` with all 9 required variables exported against the written ConfigMap:

```bash
export FINDING_KIND=Pod FINDING_NAME=my-deployment-abc12 FINDING_NAMESPACE=default \
  FINDING_PARENT=my-deployment FINDING_FINGERPRINT=a3f9c2b14d8e... \
  FINDING_ERRORS='[{"text":"CrashLoopBackOff"}]' \
  FINDING_DETAILS="The pod is in CrashLoopBackOff..." \
  GITOPS_REPO=lenaxia/talos-ops-prod GITOPS_MANIFEST_ROOT=kubernetes
VARS='${FINDING_KIND}...' envsubst "$VARS" < configmap-prompt.yaml > /tmp/rendered.txt
grep -c '\${FINDING_' /tmp/rendered.txt  # → 0 (no unsubstituted vars)
```

Result: all 9 substitutions applied correctly; 0 unsubstituted `${FINDING_*}` patterns
remain; all values appear in the correct locations in the rendered output.

---

## Bugs Found and Fixed During Code Review

| # | Severity | File | Bug | Fix |
|---|----------|------|-----|-----|
| 1 | Critical | `agent-entrypoint.sh:17` | `opencode run --file` is the *attach-files* flag, not *run-from-file*. Every agent Job would exit immediately with `Error: You must provide a message or a command`. | Changed to `exec opencode run "$(cat /tmp/rendered-prompt.txt)"` — the entire rendered prompt is passed as a single positional argument. |
| 2 | High | `agent-entrypoint.sh` | `set -u` does not fire for the 9 required env vars because `VARS` is single-quoted. Missing vars would silently produce blank prompt fields. | Added explicit `: "${VAR:?VAR must be set}"` guards for all 9 vars before any other work. |
| 3 | High | `configmap-prompt.yaml` Step 1 | The deduplication path said "Add a comment" with no `gh` CLI command — every other step has explicit shell commands; this was the only exception. | Added explicit `gh pr comment "$EXISTING" --repo ... --body "..."` example. |
| 4 | High | `configmap-prompt.yaml` Step 1 | `jq` filter could return multiple PR numbers if duplicates exist; prompt said "the existing PR" (singular) with no disambiguation. | Added `| head -1` to the `EXISTING=$(...)` command and updated the condition to "take the first number if multiple are returned". |
| 5 | Medium | `agent-entrypoint.sh:5` | `gh auth login --with-token` exits 0 on bad credentials; the token failure would only surface mid-investigation. | Added `gh auth status` check immediately after login to fail fast with a clear error message. |

Bugs 7–10 from the full review report (version drift between LLD and Dockerfile, wrong
GitHub org in LLD, smoke test coverage gap) are LLD documentation issues noted for future
LLD revision — not blocking for the configmap/entrypoint deliverables.

---

## Key Design Notes

- **`opencode run` invocation**: The correct form is `opencode run "<prompt string>"`.
  The `--file`/`-f` flag attaches files to a message (similar to how you'd attach a file
  in a chat). The prompt must be the positional `message` argument. Using
  `"$(cat /tmp/rendered-prompt.txt)"` is safe because the shell expands it as a single
  quoted string — word-splitting only occurs on unquoted expansions.

- **envsubst restricted mode**: By passing `"$VARS"` to `envsubst`, only the 9 known
  variable names are substituted. Any `$` signs in `FINDING_ERRORS` or `FINDING_DETAILS`
  (e.g. from Helm templates in log output) are left untouched.

- **Env var guards placement**: The `: "${VAR:?}"` guards run before `gh auth login`.
  This means a misconfigured Job fails immediately at container start (visible in Pod
  logs) rather than after authentication.

---

## Tests Run

```
go build ./... + go test -timeout 30s -race ./... → all 9 packages pass (no Go changes)
envsubst smoke test → 0 unsubstituted ${FINDING_*} vars in rendered output
```

---

## Next Steps

epic05 is complete. Next epic: **epic06 — CI/CD** (GitHub Actions workflows for image
build and push). Read `docs/DESIGN/lld/` for the CI/CD LLD before starting.

---

## Files Created/Modified

| File | Change |
|------|--------|
| `deploy/kustomize/configmap-prompt.yaml` | Real prompt content (was placeholder) |
| `docker/scripts/agent-entrypoint.sh` | Fixed `opencode run` invocation; added env var guards and gh auth validation |
