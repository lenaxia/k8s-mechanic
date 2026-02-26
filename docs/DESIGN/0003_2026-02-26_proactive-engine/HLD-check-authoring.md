# High-Level Design — Check Authoring API and Check Lifecycle

**Version:** 1.0
**Date:** 2026-02-26
**Status:** Proposed
**Parent:** [`HLD.md`](HLD.md)

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-26 | Initial: check lifecycle, authoring API, LLM workflow, human gate, check-review prompt | LLM / Human |

---

## Table of Contents

1. [Purpose](#1-purpose)
2. [Check Lifecycle](#2-check-lifecycle)
3. [Check Authoring API](#3-check-authoring-api)
4. [Human Approval Gate](#4-human-approval-gate)
5. [LLM Check Quality Review Workflow](#5-llm-check-quality-review-workflow)
6. [Check-Review Agent Prompt Variant](#6-check-review-agent-prompt-variant)
7. [AuditSchedule: check-review type](#7-auditschedule-check-review-type)
8. [Authentication and Authorisation](#8-authentication-and-authorisation)
9. [Failure Modes](#9-failure-modes)

---

## 1. Purpose

This document covers two related concerns:

**Check lifecycle** — how a check goes from an idea (or a guidance document) to an active,
auto-remediating enforcement rule, and how it is retired when no longer needed.

**Check Authoring API** — the HTTP API exposed by `mendabot-proactive` through which LLM
agents and operators create, update, promote, and deprecate checks. This is the primary
write path for checks. The upstream Git registry (`lenaxia/mendabot-checks`) is a read
path for curated bundles; LLM-authored checks go through this API, not through Git.

---

## 2. Check Lifecycle

```
                  POST /api/v1/checks/validate
                  (no persistence; returns errors or ok)
                           │
                           ▼
                        Proposed
                  (validated but not written)
                           │
                  POST /api/v1/checks
                           │
                           ▼
                        Staged
                  CheckBundle CRD written
                  spec.origin: api
                  status.phase: Staged
                  maturityStage: 1-manual (default)
                  NOT evaluated by the evaluator
                           │
              PUT /api/v1/checks/{id}/promote
              (LLM or human; stage 1→2 is autonomous)
                           │
                           ▼
                        Active
                  status.phase: Active
                  maturityStage: 2-detected
                  Evaluator runs checks; AuditFindings emitted
                           │
              PUT /api/v1/checks/{id}/promote
              (stage 2→3 requires human approval)
                           │
                           ▼
                  pendingPromotion written
                  status.phase: Active (unchanged)
                  maturityStage: 2-detected (unchanged until approved)
                           │
              Human approves via kubectl patch or future UI
                           │
                           ▼
                        Active
                  maturityStage: 3-remediated
                  RemediationJobs now possible
                           │
              (repeat for stages 4, 5 — each requires human approval)
                           │
              PUT /api/v1/checks/{id}/deprecate
                           │
                           ▼
                        Deprecated
                  status.phase: Deprecated
                  Evaluator skips this bundle
                  Existing open AuditFindings: annotated check-deprecated: true
                  No new AuditFindings created
                           │
              All open AuditFindings closed (Resolved or Suppressed)
              DELETE /api/v1/checks/{id} (or automatic GC)
                           │
                           ▼
                        Deleted
```

### Lifecycle rules

**Stage 1–2 promotion:** LLM or human can call `PUT /api/v1/checks/{id}/promote` directly.
No approval required. Stage 2 enables CEL evaluation but not auto-remediation.

**Stage 2–3 and above promotion:** The API writes `status.pendingPromotion` to the
`CheckBundle` CRD. The check remains at its current stage. An operator must explicitly
approve the promotion. Until approved, the pending promotion is visible via:
```
kubectl get checkbundle <name> -o jsonpath='{.status.pendingPromotion}'
```

**Deprecation:** Allowed at any stage. Does not require pending approval. All open
`AuditFinding` objects for the deprecated check have annotation
`proactive.mendabot.io/check-deprecated: "true"` written immediately. The `AuditFindingReconciler`
surfaces this annotation in reports. Open findings remain in their current lifecycle phase —
deprecation does not close them.

**Deletion:** Only allowed if no open (non-terminal) `AuditFinding` objects reference the
check. The API enforces this guard. If open findings exist, the API returns HTTP 409 with
a list of blocking findings.

**Immutable field:** `spec.checkClass` is immutable after a check reaches `Active` phase.
Changing check class requires deprecating the old check and creating a new one.

### Git-synced check lifecycle

`CheckBundle` CRDs with `spec.origin: git-sync` follow a simplified lifecycle:
- `Active` or `ValidationFailed` only — no `Staged` phase
- Maturity stage changes come from the Git repository YAML, not the authoring API
- The authoring API will reject `promote` and `deprecate` calls for git-sync bundles
- Curated bundles are managed upstream; changes go through the `lenaxia/mendabot-checks` PR process

---

## 3. Check Authoring API

Hosted by `mendabot-proactive` on a separate HTTP port (`AUTHORING_API_PORT`, default `8081`).
All endpoints authenticate via Kubernetes ServiceAccount Bearer token. See §8.

All write operations (`POST`, `PUT`, `PATCH`, `DELETE`) write to `CheckBundle` CRDs via the
typed K8s client. The API is a thin HTTP facade over the CRD store — no separate database.

### 3.1 Validate (no persistence)

```
POST /api/v1/checks/validate
Content-Type: application/json

{
  "id": "no-root-containers",
  "category": "security",
  "checkClass": "enforcement",
  "severity": "high",
  "scope": "container",
  "cel": "container.securityContext.runAsNonRoot == true",
  "remediationAllowed": true,
  "remediationHint": "Set securityContext.runAsNonRoot: true",
  "failureMessageTemplate": "Container {{.Container.Name}} runs as root",
  "stabilisationSeconds": 0
}
```

Response (success):
```json
{ "valid": true, "errors": [] }
```

Response (failure):
```json
{
  "valid": false,
  "errors": [
    {
      "field": "cel",
      "message": "undefined field 'runAsNonRoot' on type 'SecurityContext'"
    }
  ]
}
```

Validation performs:
1. JSON schema check on all required fields
2. CEL parse (`cel.Env.Parse`)
3. CEL type-check (`cel.Env.Check`) against the scope's declared variable types
4. `checkClass` + `remediationAllowed` constraint check (advisory/informational must be false)
5. `failureMessageTemplate` parse (go `text/template` parse only — not rendered)

No write to any CRD. Safe to call repeatedly during LLM iteration.

### 3.2 Stage a check (create)

```
POST /api/v1/checks
Content-Type: application/json

{
  "bundleName": "my-org-security",   # CheckBundle CRD name; created if not exists
  "check": { ... }                   # same fields as validate body
}
```

Response (success, HTTP 201):
```json
{
  "bundleName": "my-org-security",
  "checkID": "no-root-containers",
  "phase": "Staged",
  "maturityStage": "1-manual"
}
```

Behaviour:
- Runs the same validation as `POST /validate`. Returns HTTP 422 on failure.
- If `bundleName` CheckBundle CRD does not exist: creates it with `spec.origin: api`, `status.phase: Staged`.
- If `bundleName` CheckBundle CRD exists with `spec.origin: git-sync`: returns HTTP 409 (cannot add checks to a git-sync bundle via API).
- If `checkID` already exists in the bundle: returns HTTP 409.
- New check is added with `maturityStage: 1-manual` and `status.phase: Staged`.

### 3.3 Update a check

```
PATCH /api/v1/checks/{bundleName}/{checkID}
Content-Type: application/json

{
  "cel": "container.securityContext.runAsNonRoot == true && container.securityContext.runAsUser > 0",
  "remediationHint": "Updated hint text"
}
```

Response (success, HTTP 200):
```json
{
  "bundleName": "my-org-security",
  "checkID": "no-root-containers",
  "phase": "Active",
  "maturityStage": "2-detected",
  "validationResult": "ok"
}
```

Behaviour:
- Only `cel`, `remediationHint`, `failureMessageTemplate`, `stabilisationSeconds`, `severity`,
  `frameworks`, `description` are patchable.
- `checkClass`, `scope`, `remediationAllowed` are immutable once the check is `Active`.
- Runs validation on the new CEL expression. Returns HTTP 422 on failure.
- Updates the `CheckBundle` CRD in etcd. The evaluator picks up the new expression on its
  next run.
- If the check is `Active` with existing open `AuditFinding` objects: the update is applied.
  Existing findings are not retroactively re-evaluated. The new CEL runs on the next evaluator
  pass — findings that now pass will not be re-detected; findings that were missed before may
  appear.

### 3.4 Promote a check

```
PUT /api/v1/checks/{bundleName}/{checkID}/promote
Content-Type: application/json

{
  "targetStage": "3-remediated",
  "reason": "Check stable for 14 days; suppression rate 2%; zero failed remediations"
}
```

Response (stage 1→2, HTTP 200 — applied immediately):
```json
{
  "checkID": "no-root-containers",
  "previousStage": "1-manual",
  "newStage": "2-detected",
  "applied": true,
  "pendingApproval": false
}
```

Response (stage 2→3, HTTP 202 — pending approval):
```json
{
  "checkID": "no-root-containers",
  "previousStage": "2-detected",
  "requestedStage": "3-remediated",
  "applied": false,
  "pendingApproval": true,
  "approvalInstruction": "kubectl patch checkbundle my-org-security --type=merge -p '{\"status\":{\"pendingPromotion\":{\"approved\":true}}}'"
}
```

Behaviour:
- `targetStage` must be exactly one stage above current. HTTP 422 otherwise.
- For stage 1→2: applies immediately. Updates `maturityStage` in the `CheckBundle` CRD.
  If the check was `Staged`, transitions to `Active`.
- For stage 2→3 and above: writes `status.pendingPromotion` to the `CheckBundle` CRD.
  Does **not** change `maturityStage`. Returns HTTP 202.
- Calls to `promote` while `pendingPromotion` is already set (and not yet approved/rejected):
  returns HTTP 409.

### 3.5 Deprecate a check

```
PUT /api/v1/checks/{bundleName}/{checkID}/deprecate
```

Response (HTTP 200):
```json
{
  "checkID": "no-root-containers",
  "phase": "Deprecated",
  "openFindingsAnnotated": 3
}
```

Behaviour:
- Sets `status.phase: Deprecated` on the check entry in the `CheckBundle` CRD.
- If the entire bundle has no active (non-deprecated) checks: sets `CheckBundle.Status.Phase: Deprecated`.
- Writes `proactive.mendabot.io/check-deprecated: "true"` annotation to all open
  `AuditFinding` objects with `spec.checkID` matching this check.
- Returns the count of annotated findings.

### 3.6 List checks

```
GET /api/v1/checks?phase=Active&maturityStage=2-detected&category=security
```

Response (HTTP 200):
```json
{
  "checks": [
    {
      "bundleName": "cis-k8s-1.8",
      "checkID": "no-root-containers",
      "category": "security",
      "severity": "high",
      "maturityStage": "3-remediated",
      "phase": "Active",
      "origin": "git-sync",
      "findingSummary": {
        "detected": 3,
        "reported": 0,
        "suppressed": 1,
        "resolved": 12,
        "pendingVerification": 0
      }
    }
  ],
  "total": 1
}
```

The `findingSummary` is computed by the API handler by listing `AuditFinding` CRDs filtered
by `proactive.mendabot.io/check-id` label. This is a live query against the informer cache.

Query parameters: `phase`, `maturityStage`, `category`, `origin`, `severity`.

### 3.7 Get a check

```
GET /api/v1/checks/{bundleName}/{checkID}
```

Response: full check spec + status + `findingSummary`.

### 3.8 Delete a check

```
DELETE /api/v1/checks/{bundleName}/{checkID}
```

Response (HTTP 200 if no open findings):
```json
{ "deleted": true }
```

Response (HTTP 409 if open findings exist):
```json
{
  "deleted": false,
  "reason": "2 open AuditFinding objects reference this check",
  "blockingFindings": ["afinding-abc123", "afinding-def456"]
}
```

Behaviour:
- Only allowed for `api`-origin checks. Git-sync checks return HTTP 403.
- Only allowed if no non-terminal `AuditFinding` objects reference this check.
- Removes the check entry from the `CheckBundle` CRD. If the bundle has no remaining
  checks: marks the `CheckBundle` phase as `Deleted`.

---

## 4. Human Approval Gate

### What requires approval

Any maturity stage promotion from stage 2 to stage 3 or above. These are the stages where
`RemediationJob` creation becomes possible — automated cluster modification. An LLM
proposing this transition writes evidence but cannot apply it unilaterally.

### pendingPromotion field

Written to `CheckBundle.Status.PendingPromotion` by the authoring API:

```yaml
status:
  pendingPromotion:
    targetStage: "3-remediated"
    proposedAt: "2026-02-26T10:00:00Z"
    proposedBy: "check-review-agent"   # agent Job name or "human"
    reason: |
      Check stable for 14 days. Suppression rate: 2% (1 of 47 findings).
      All RemediationJob attempts succeeded. Zero rollbacks observed.
    approved: false   # human sets to true to approve; or deletes the field to reject
```

### Approval

The operator approves by patching the `CheckBundle`:

```bash
kubectl patch checkbundle my-org-security --type=merge \
  -p '{"status":{"pendingPromotion":{"approved":true}}}'
```

The `CheckRegistryController` (or a dedicated promotion reconciler within proactive) watches
for `pendingPromotion.approved == true` and applies the stage transition:
- Updates `maturityStage` on the check entry
- Clears `pendingPromotion`
- Emits a Kubernetes Event: `Normal StagePromoted check no-root-containers promoted to 3-remediated`

### Rejection

The operator rejects by deleting the `pendingPromotion` field:

```bash
kubectl patch checkbundle my-org-security --type=json \
  -p '[{"op":"remove","path":"/status/pendingPromotion"}]'
```

No event is emitted for rejection. The `pendingPromotion` field simply disappears. The
check remains at its current stage. The LLM can propose again in the next review cycle.

### No automatic expiry in v1

Pending promotions do not automatically expire. An operator must explicitly approve or
reject. This is noted as a limitation — future versions may add a configurable expiry and
auto-rejection policy.

---

## 5. LLM Check Quality Review Workflow

The check quality review is performed by a `mendabot-agent` Job with
`PROMPT_VARIANT=check-review`. It is triggered by an `AuditSchedule` with
`spec.type: check-review`. See §7.

### What the LLM reviews

For each `Active` check, the agent collects evidence from two sources:

1. **Check metadata** via `GET /api/v1/checks`:
   - Current maturity stage
   - `remediationHint` text
   - CEL expression
   - `findingSummary` (detected, suppressed, resolved, pendingVerification counts)

2. **AuditFinding CRDs** via K8s API (the agent has read access):
   - Age of open findings (firstDetectedAt)
   - Suppression rate: count(Suppressed) / total
   - Resolution rate: count(Resolved) / total
   - PendingVerification age (findings stuck > threshold indicate bad remediationHint)
   - Regression count: findings that cycled Resolved → Detected more than once

### Decision criteria

| Signal | Interpretation | Action |
|---|---|---|
| Suppression rate > 30% | Check fires on intentional configurations; too aggressive | Propose CEL revision to narrow scope |
| Zero findings on a cluster known to have violations | CEL may be wrong or scope too narrow | Propose CEL revision; flag for human review |
| PendingVerification age > 14 days | RemediationJob succeeded but human hasn't confirmed; possibly bad remediationHint | Propose remediationHint update |
| RemediationJob failure rate > 20% | Remediation is unreliable; check should not be at stage 3+ | Propose demotion (deprecate + re-create at stage 2) |
| Stage 2, stable > 14 days, suppression rate < 5%, zero failed remediations | Ready for stage 3 | Propose promotion |
| Stage 3–4, stable > 30 days, zero failed remediations, suppression rate < 2% | Ready for next stage | Propose promotion |

These thresholds are parameters in the check-review prompt, not hardcoded in the engine.
The LLM applies judgment within these guidelines.

### What the LLM can do autonomously

- Update `cel` expression on any `Staged` or `Active` check
- Update `remediationHint` on any check at any stage
- Update `failureMessageTemplate`, `description`, `severity`
- Propose stage 1→2 promotion (applied immediately)
- Propose stage 2→3+ promotion (written as `pendingPromotion`; requires human)

### What the LLM cannot do

- Approve its own promotion proposals
- Change `checkClass` or `scope` on an `Active` check
- Delete checks with open findings
- Modify git-sync bundles
- Access Kubernetes Secrets

### Review output

The agent produces a structured review report written to:
- A GitHub PR on the target repo (branch: `check-review/<runTick-sanitised>`)
- Or a webhook POST (same payload format as proactive reports but with `report_type: check-review`)

The report includes:
- Summary of checks reviewed (counts by action taken)
- Per-check review entries for checks that had changes proposed or applied
- Pending promotion proposals with evidence summaries
- Checks flagged for human review (anomalies the LLM could not confidently diagnose)

---

## 6. Check-Review Agent Prompt Variant

The `mendabot-agent` image gains a third prompt file. Selected by `PROMPT_VARIANT=check-review`.

### What the check-review prompt instructs the agent to do

1. Read the authoring API base URL from `CHECK_AUTHORING_API_URL`.
2. `GET /api/v1/checks` to retrieve all active checks with finding summaries.
3. For each check, read relevant `AuditFinding` CRDs via `kubectl get auditfindings`.
4. Apply the review criteria (§5) to identify checks that need attention.
5. For each check requiring change:
   a. `POST /api/v1/checks/validate` to test any revised CEL before applying.
   b. `PATCH /api/v1/checks/{bundleName}/{checkID}` to apply the change.
   c. `PUT /api/v1/checks/{bundleName}/{checkID}/promote` for promotions.
6. Write the review report as a GitHub PR or webhook POST.
7. Exit 0.

### Hard rules

- Never modify git-sync bundle checks.
- Never approve pending promotions.
- Never set `maturityStage` above 2 without going through the promote endpoint (which
  enforces the human gate automatically).
- Never delete checks — only deprecate.
- Do not make changes to more than 20 checks in a single review pass. If more are
  identified, report them but only apply changes to the highest-priority 20.

### Agent Job environment variables (check-review)

| Variable | Content |
|---|---|
| `PROMPT_VARIANT=check-review` | Selects check-review prompt file |
| `CHECK_AUTHORING_API_URL` | `http://mendabot-proactive.mendabot.svc.cluster.local:8081` |
| `AUDIT_RUN_ID` | `<scheduleRef>/<runTick>` |
| `AUDIT_SINK_GITHUB_PR_REPO` | `owner/repo`; empty if no GitHub PR sink |
| `AUDIT_SINK_WEBHOOK_URL_COUNT` | Number of webhook sinks |
| `AUDIT_SINK_WEBHOOK_URL_0` ... `_N` | Webhook URLs injected from Secrets |

The check-review agent Job does **not** receive a ConfigMap with findings.json. It queries
the authoring API and K8s API directly.

---

## 7. AuditSchedule: check-review type

An `AuditSchedule` with `spec.type: check-review` triggers the check quality review
workflow instead of a cluster audit.

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: AuditSchedule
metadata:
  name: weekly-check-review
  namespace: mendabot
spec:
  type: check-review
  trigger:
    cron: "0 3 * * 0"    # every Sunday at 03:00; event triggers not supported for check-review
  sinks:
    - type: github-pr
      repo: lenaxia/talos-ops-prod
  # personaRef: not used for check-review
  # checkBundleRefs: not used for check-review
  # scope: not used for check-review
  # autoRemediate: not used for check-review
```

Unused fields (`checkBundleRefs`, `scope`, `personaRef`, `autoRemediate`) are ignored by
the `AuditController` when `spec.type: check-review`. They may be present without effect.

`AuditController` behaviour for `spec.type: check-review`:
1. Cron fires.
2. Create `mendabot-agent` Job with `PROMPT_VARIANT=check-review`.
3. Inject `CHECK_AUTHORING_API_URL` and sink env vars.
4. Update `AuditSchedule.Status.LastRunAt`.
5. No evaluator Job created. No `AuditFinding` emission.

---

## 8. Authentication and Authorisation

### API authentication

The Check Authoring API authenticates all requests via Kubernetes ServiceAccount Bearer
tokens. The token is passed in the `Authorization: Bearer <token>` header.

The API handler calls the Kubernetes `TokenReview` API to validate the token and extract
the ServiceAccount identity. No custom token infrastructure required.

### API authorisation

Authorisation is based on the caller's ServiceAccount identity mapped to a role:

| Caller | Permitted operations |
|---|---|
| `mendabot-agent` ServiceAccount | All read endpoints; `validate`; `stage`; `PATCH` (update); `promote` (writes pendingPromotion only; cannot approve); `deprecate` |
| Operator (human, any ServiceAccount in `mendabot` namespace) | All operations including `DELETE` and approval |
| Any other ServiceAccount | No access (HTTP 403) |

This is enforced by the API handler, not by Kubernetes RBAC on the CRDs directly. The
authorisation table is hardcoded in the API handler — no configurable RBAC policy in v1.

### In-cluster access

The check-review agent Job calls the authoring API via the cluster-internal service DNS:
```
http://mendabot-proactive.mendabot.svc.cluster.local:8081
```

No Ingress or external exposure is required for the LLM workflow. External access (for
operator tooling) requires `kubectl port-forward` or an operator-managed Ingress — not
provided by the Helm chart in v1.

---

## 9. Failure Modes

| Failure | Behaviour |
|---|---|
| `POST /validate` with invalid CEL | HTTP 422; detailed error per field; no side effect |
| `POST /checks` with invalid CEL | HTTP 422; no CheckBundle CRD written |
| `PATCH /checks` with invalid CEL | HTTP 422; existing CRD unchanged |
| Authoring API pod crash during write | CRD write is atomic (K8s API); partial writes not possible; on restart, CRD state is consistent |
| Check-review agent hits authoring API rate limit | No rate limit in v1; noted as future work |
| Check-review agent proposes > 20 changes | Agent prompt instruction limits to 20; excess are reported but not applied |
| `pendingPromotion` field never approved | Check stays at current stage indefinitely; visible via kubectl; no automatic expiry in v1 |
| `DELETE` on check with open findings | HTTP 409; no deletion; caller receives list of blocking findings |
| Check-review agent cannot reach authoring API | Agent exits non-zero; Job retried once per `backoffLimit: 1`; `AuditSchedule.Status` updated |
| TokenReview API unavailable | API returns HTTP 503; caller retries; affects all authoring API calls |
| git-sync bundle modification attempt via API | HTTP 403; git-sync bundles are read-only via API |
