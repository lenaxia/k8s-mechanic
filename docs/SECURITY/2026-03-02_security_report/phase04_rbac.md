# Phase 4: RBAC Enforcement Testing

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)
**Cluster:** no — all tests SKIPPED

---

## 4.1 Default Cluster Scope — Secret Read

**Status:** SKIPPED — no live cluster available

**Notes:** This test confirms accepted residual risk AR-01. The `mechanic-agent`
ClusterRole grants `get/list/watch` on a fixed resource list that does not include
`secrets`. Expected result when run: agent cannot read Secrets. This is a change from
the original AR-01 acceptance — the current ClusterRole explicitly excludes secrets
(only the old 2026-02-23 role granted them). To be confirmed in next live cluster review.

---

## 4.2 Namespace Scope — Secret Read Restriction

**Status:** SKIPPED — no live cluster available

**Notes:** The `role-agent-ns.yaml` overlay currently uses `resources: ["*"]` which
would cause this test to FAIL (see finding 2026-03-02-002). This test should be re-run
after the finding is remediated to confirm restriction is effective.

---

## 4.3 Agent Write Restriction

**Status:** SKIPPED — no live cluster available

**Notes:** Static review of ClusterRole confirms no write verbs, no `pods/exec`, no
`nodes/proxy`. Expected result: all write checks blocked.

---

## 4.4 Watcher Escalation Paths

**Status:** SKIPPED — no live cluster available

**Notes:** Watcher ClusterRole retains `secrets: get/list/watch` (AR-08, finding
2026-02-27-001). Expected watcher-secret-read result: allowed (until AR-08 is resolved
by live cluster test confirming namespace Role is sufficient).

---

## Phase 4 Summary

**Total findings:** 0
**Findings added to findings.md:** none
**All tests SKIPPED — no live cluster. Schedule for next review.**
