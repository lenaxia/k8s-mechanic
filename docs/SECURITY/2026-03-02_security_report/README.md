# Security Report: 2026-03-02

**Report Date:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)
**Review Type:** Partial (scope: static analysis, architecture, redaction, supply chain, operational, regression — no live cluster)
**Cluster Available:** no
**CNI (NetworkPolicy Support):** N/A
**Previous Report:** [2026-02-27_security_report](../2026-02-27_security_report/README.md)
**Git Commit Reviewed:** `8fc791a2a42d0cc500628edfe3d580da25d0f2df`
**Status:** Open

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| 2026-02-27-001 | MEDIUM | Watcher ClusterRole `secrets` — redundant pending live verification | Accepted (AR-08) |
| 2026-02-27-002 | HIGH | GitHub App private key as env var in watcher | Remediated |
| 2026-02-27-003 | MEDIUM | `git` wrapper incomplete blocklist | Partially Remediated |
| 2026-02-27-004 | MEDIUM | `gh api` write bypass in dry-run mode | Remediated |
| 2026-02-27-005 | HIGH | `agentImage` CRD field has no validation | Open |
| 2026-02-27-006 | HIGH | `agentSA` CRD field has no validation | Open |
| 2026-02-27-007 | HIGH | `anomalyco/opencode/github@latest` unpinned | Remediated |
| 2026-02-27-008 | HIGH | `renovate-analysis.yml` LLM auto-merge | Accepted/INFO (re-classified) |
| 2026-02-27-009 | MEDIUM | `emit_dry_run_report` writes unredacted output | Remediated |
| 2026-02-27-010 | MEDIUM | Circuit breaker state in-memory only | Open |
| 2026-02-27-011 | MEDIUM | Agent Job containers have no resource limits | Remediated |
| 2026-02-27-012 | LOW | `sinkRef.url` has no format validation | Open |
| 2026-02-27-013 | HIGH | `ai-comment.yml` any user can trigger LLM | Remediated |
| 2026-02-27-014 | LOW | `finding.Details` not redacted | Deferred |
| 2026-03-02-001 | LOW | `FINDING_CORRELATED_FINDINGS` not checked by `DetectInjection` | Open |
| 2026-03-02-002 | MEDIUM | `role-agent-ns.yaml` uses `resources: ["*"]` in namespace-scope overlay | Open |
| 2026-03-02-003 | INFO | Go builder image version label `1.25.7` may be inaccurate | Open |
| 2026-03-02-004 | LOW | `git` wrapper does not block `checkout -b` in dry-run mode | Open |

**Counts (this review — new findings only):**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 0 | 0 | 0 | 0 |
| MEDIUM | 1 | 0 | 0 | 0 |
| LOW | 2 | 0 | 0 | 0 |
| INFO | 1 | 0 | 0 | 0 |
| **Total** | **4** | **0** | **0** | **0** |

**Counts (all open findings across all reports):**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 2 | 5 | 1 | 0 |
| MEDIUM | 3 | 4 | 0 | 0 |
| LOW | 3 | 1 | 0 | 1 |
| INFO | 1 | 0 | 0 | 0 |
| **Total** | **9** | **10** | **1** | **1** |

---

## Scope

**Phases completed:**

- [x] Phase 1: Static Code Analysis → [phase01_static.md](phase01_static.md)
- [x] Phase 2: Architecture and Design Review → [phase02_architecture.md](phase02_architecture.md)
- [x] Phase 3: Redaction and Injection Testing → [phase03_redaction.md](phase03_redaction.md)
- [ ] Phase 4: RBAC Enforcement Testing → [phase04_rbac.md](phase04_rbac.md)
- [ ] Phase 5: Network Egress Testing → [phase05_network.md](phase05_network.md)
- [x] Phase 6: GitHub App Private Key Isolation → [phase06_privkey.md](phase06_privkey.md)
- [ ] Phase 7: Audit Log Verification → [phase07_audit.md](phase07_audit.md)
- [x] Phase 8: Supply Chain Integrity → [phase08_supply_chain.md](phase08_supply_chain.md)
- [x] Phase 9: Operational Security → [phase09_operational.md](phase09_operational.md)
- [x] Phase 10: Regression Check → [phase10_regression.md](phase10_regression.md)

**Phases skipped:**

| Phase | Reason |
|-------|--------|
| Phase 4: RBAC Enforcement Testing | No live cluster available |
| Phase 5: Network Egress Testing | No live cluster available |
| Phase 7: Audit Log Verification | No live cluster available |
| Phase 11: LLM Exfiltration Testing | No live cluster available |

---

## Executive Summary

The 2026-03-02 static code review of commit `8fc791a` found 4 new findings (1 MEDIUM, 2 LOW, 1 INFO) and confirmed remediation of 7 findings from the 2026-02-27 report. The overall security posture has materially improved: the GitHub App private key is no longer an env var in the watcher pod, the `gh` wrapper now uses an allowlist approach, the `git` wrapper blocks the most dangerous destructive subcommands, CI actions are SHA-pinned, and agent Jobs have resource limits. The two highest-priority open items remain `agentImage` and `agentSA` CRD fields with no admission validation (2026-02-27-005/006, HIGH) — an in-cluster adversary with `create` on `remediationjobs` can substitute an arbitrary container image or service account. The new MEDIUM finding (2026-03-02-002) is the namespace-scope Kustomize overlay using a wildcard resource grant that defeats the scoping purpose; this should be corrected before the overlay is applied to any production cluster. Phases requiring a live cluster (4, 5, 7, 11) remain unrun; the next review with cluster access should prioritise Phase 11 (exfil) and Phase 4 (RBAC live enforcement).

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes | No change; ClusterRole unchanged |
| AR-02 | Redaction false negatives | yes | 16 patterns reviewed; no new gaps requiring immediate action |
| AR-03 | NetworkPolicy requires CNI | yes | CNI still required for egress enforcement |
| AR-04 | Prompt injection not fully preventable | yes | 5 detection patterns reviewed; gaps known and documented |
| AR-05 | GitHub token in shared emptyDir | yes | Init/main isolation reviewed and confirmed unchanged |
| AR-06 | HARD RULEs are prompt-only controls | yes | No change |
| AR-07 | `curl` bypasses dry-run wrappers | yes | No change; `curl` is not wrapped |
| AR-08 | Watcher ClusterRole cluster-wide secrets read | yes | Namespace Role confirmed correct; live verification still pending |

**New accepted risks this review:**

| ID | Description | Severity | Rationale | Sign-off |
|----|-------------|----------|-----------|---------|
| — | None | — | — | — |

---

## Recommendations for Next Review

1. **Priority 1 — 2026-02-27-005/006 (HIGH):** Close `agentImage`/`agentSA` admission gap with CEL validation rules or the Kyverno `restrict-agent-image` policy. Verify in live cluster.
2. **Priority 2 — 2026-03-02-002 (MEDIUM):** Fix `deploy/overlays/security/role-agent-ns.yaml` wildcard resource grant before any production use.
3. **Priority 3 — 2026-02-27-001/AR-08 (MEDIUM):** Run live cluster test to verify watcher starts without the ClusterRole `secrets` entry; close or escalate based on result.
4. **Priority 4 — 2026-02-27-010 (MEDIUM):** Persist circuit breaker state using `RemediationJob` creation timestamps in etcd (or at minimum document restart behaviour for operators).
5. **Priority 5 — Phases 4, 5, 7, 11:** All cluster-dependent phases remain unrun. Next review with cluster access should run Phase 11 (exfil) first, followed by Phase 4 (RBAC live enforcement).
6. **2026-03-02-001 (LOW):** Add `DetectInjection` call on `correlatedFindings` slice in `controller.go` before env var injection in `job.go:Build()`.
7. **2026-03-02-003 (INFO):** Verify the Go builder image digest and correct the `1.25.7` version label if inaccurate.

---

## Sign-off

Checklist completed: partial (phases 4, 5, 7, 11 skipped — no live cluster)

**Reviewer:** automated (k8s-mechanic security process)
**Date:** 2026-03-02

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
