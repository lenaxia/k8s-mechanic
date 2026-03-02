# Phase 10: Regression Check

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)
**Previous report:** [2026-02-27_security_report](../2026-02-27_security_report/README.md)

---

## 10.1 Previous Findings Verification

| Finding ID | Title | Previous Status | Still Remediated? | Evidence | Notes |
|-----------|-------|----------------|-------------------|---------|-------|
| 2026-02-27-001 | Watcher ClusterRole `secrets` | Accepted (AR-08) | N/A — accepted | Unchanged; namespace Role still has correct verbs | Live cluster verification still pending |
| 2026-02-27-002 | GitHub App private key as env var in watcher | Fixed | yes | `cmd/watcher/main.go:233` uses `os.ReadFile()`; no env var in deployment-watcher.yaml | |
| 2026-02-27-003 | `git` wrapper incomplete blocklist | Fixed (partial) | partial | `scripts/git-wrapper.sh` blocks expanded set; `checkout -b` still not blocked — see 2026-03-02-004 | |
| 2026-02-27-004 | `gh api` write bypass | Fixed | yes | `scripts/gh-wrapper.sh` allowlist confirmed; GET allowed, write methods blocked | |
| 2026-02-27-005 | `agentImage` CRD no validation | Open | N/A | CRD schema still lacks CEL validation rule for image allowlist at this commit | Kyverno policy path noted but not verified present |
| 2026-02-27-006 | `agentSA` CRD no validation | Open | N/A | CRD schema still lacks allowlist constraint | |
| 2026-02-27-007 | `anomalyco/opencode/github@latest` unpinned | Fixed | yes | All 4 action uses pinned to SHA `0cf0294...` | |
| 2026-02-27-008 | `renovate-analysis.yml` LLM auto-merge | Accepted | re-classified | Workflow confirmed posts comment only; no merge authority — re-classified to Accepted/INFO | Original HIGH severity overstated |
| 2026-02-27-009 | `emit_dry_run_report` no redaction | Fixed | yes | `scripts/entrypoint-common.sh:159,172` — both outputs piped through `redact`; hard-fail if absent | |
| 2026-02-27-010 | Circuit breaker in-memory only | Open | N/A | `internal/circuitbreaker/circuitbreaker.go` — `lastAllowed` still an in-memory struct field | No persistence added |
| 2026-02-27-011 | Agent Job no resource limits | Fixed | yes | `internal/domain/job.go` — `containerResources()` applies CPU/memory requests and limits | |
| 2026-02-27-012 | `sinkRef.url` no format validation | Open | N/A | Pattern constraint noted in 2026-02-27 fix field; requires live CRD apply to confirm | |
| 2026-02-27-013 | `ai-comment.yml` any user trigger | Fixed | yes | `.github/workflows/ai-comment.yml` — `author_association` gate confirmed present | |
| 2026-02-27-014 | `finding.Details` not redacted | Deferred | N/A | No native provider sets `Details`; latent risk unchanged | |

---

## 10.2 Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | New Control Available? | Notes |
|---------|-------------|------------------------|----------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes | no | ClusterRole explicitly excludes Secrets in current version; AR-01 may be ready to close pending live verification |
| AR-02 | Redaction false negatives | yes | no | 16 patterns reviewed; no new critical gaps identified |
| AR-03 | NetworkPolicy requires CNI | yes | no | CNI still required for egress enforcement |
| AR-04 | Prompt injection not fully preventable | yes | no | 5 detection patterns reviewed; prompt envelope is compensating control |
| AR-05 | GitHub token in shared emptyDir | yes | no | Init/main isolation confirmed unchanged |
| AR-06 | HARD RULEs are prompt-only controls | yes | no | No change |
| AR-07 | `curl` bypasses dry-run wrappers | yes | no | `curl` still not wrapped; accepted |
| AR-08 | Watcher ClusterRole cluster-wide secrets read | yes | no | Live cluster verification still pending; namespace Role conditions met in code |

---

## 10.3 Architecture Changes Since Last Review

| Change | Files Affected | Security Implication | Reviewed in This Report? |
|--------|---------------|---------------------|--------------------------|
| EX-009 fix: `AGENT_PROVIDER_CONFIG` unset before opencode launch | `scripts/entrypoint-opencode.sh` | Removes LLM API key from agent process environment | yes — Phase 9, confirmed pass |
| Resource limits added to agent Job containers | `internal/domain/job.go` | Prevents resource exhaustion (finding 2026-02-27-011 remediated) | yes — Phase 9 |
| GitHub App key moved to volume mount in watcher | `deploy/helm/templates/deployment-watcher.yaml`, `cmd/watcher/main.go` | Private key removed from process environment (finding 2026-02-27-002 remediated) | yes — Phase 6 |
| `gh` wrapper rewritten with allowlist | `scripts/gh-wrapper.sh` | Write methods blocked fail-closed in dry-run mode | yes — Phase 9 |
| `git` wrapper blocklist expanded | `scripts/git-wrapper.sh` | Blocks destructive subcommands; `checkout -b` gap noted (2026-03-02-004) | yes — Phase 9 |
| `emit_dry_run_report` redaction added | `scripts/entrypoint-common.sh` | Prevents unredacted output in ConfigMap (finding 2026-02-27-009 remediated) | yes — Phase 9 |
| `anomalyco/opencode/github` pinned to SHA | `.github/workflows/*.yml` | Supply chain risk mitigated (finding 2026-02-27-007 remediated) | yes — Phase 8 |
| `ai-comment.yml` author_association gate | `.github/workflows/ai-comment.yml` | Restricts LLM trigger to trusted collaborators (finding 2026-02-27-013 remediated) | yes — Phase 8 |

---

## Phase 10 Summary

**Total findings:** 0 new from regression check
**Findings added to findings.md:** none (regressions documented in findings.md under each 2026-02-27 entry)
**No regressions identified — all previously remediated findings confirmed still in place.**
