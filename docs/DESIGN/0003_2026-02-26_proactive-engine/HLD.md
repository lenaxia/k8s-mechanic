# High-Level Design — Proactive Engine

**Version:** 4.0
**Date:** 2026-02-26
**Status:** Proposed
**Supersedes:** v3.2 (single-document design)

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 3.0–3.2 | 2026-02-26 | Initial proactive engine HLD (single document) | LLM / Human |
| 4.0 | 2026-02-26 | Architecture revision: four-binary topology; CEL evaluation moved to evaluator Job; check authoring API; check lifecycle; split into four HLD documents | LLM / Human |

---

## Related Documents

| Document | Content |
|---|---|
| [`HLD-proactive-controller.md`](HLD-proactive-controller.md) | `mendabot-proactive` binary: check registry, CRDs, controllers, reconcilers, persona, sinks |
| [`HLD-check-authoring.md`](HLD-check-authoring.md) | Check lifecycle, Check Authoring API, LLM check quality review workflow |
| [`HLD-evaluator.md`](HLD-evaluator.md) | `mendabot-evaluator` binary: CEL evaluation engine, RBAC, secret redaction, Job lifecycle |
| [`../HLD.md`](../HLD.md) | Reactive engine HLD (v1/v2) — unchanged |

---

## Table of Contents

1. [Motivation and Context](#1-motivation-and-context)
2. [Design Principles](#2-design-principles)
3. [Four-Binary Topology](#3-four-binary-topology)
4. [CRD Inventory](#4-crd-inventory)
5. [Data Flows](#5-data-flows)
6. [Check Maturity Model](#6-check-maturity-model)
7. [Package Structure](#7-package-structure)
8. [RBAC Summary](#8-rbac-summary)
9. [Failure Modes](#9-failure-modes)
10. [Configuration Reference](#10-configuration-reference)
11. [What Does Not Change](#11-what-does-not-change)
12. [Implementation Prerequisites](#12-implementation-prerequisites)
13. [Scope and Success Criteria](#13-scope-and-success-criteria)

---

## 1. Motivation and Context

The reactive path (v1/v2) is entirely event-driven: mendabot waits for something to break
and then investigates. This is the right default — it requires no configuration and produces
actionable output for failures the operator already cares about.

However, a cluster can be in a degraded posture without any single resource being visibly
broken:

- Containers running as root, no resource limits, missing readiness probes — no failure
  event fires, but the cluster is outside security and reliability baselines.
- PodDisruptionBudgets absent for stateful workloads — fine until a node drain during
  maintenance cascades into a service outage.
- RBAC bindings granting cluster-admin to service accounts — invisible in the reactive path,
  high blast-radius risk.
- Deprecated API versions in manifests — accepted today, breaking silently on next upgrade.

None of these produce Kubernetes events or failing resources. They require scheduled
evaluation against codified best-practice checks.

The proactive engine adds this capability. It is additive — the reactive path is unchanged.
The two engines share the `RemediationJob` pipeline but run as separate binaries.

**Checks must be maintainable by an LLM.** Human-maintained check libraries do not scale.
The architecture is designed so that an LLM agent can propose, validate, stage, and improve
checks through an API — no Git workflow, no binary release, no human in the loop for
routine check maintenance.

---

## 2. Design Principles

**1. Checks are data, not code.**
Check definitions live in `CheckBundle` CRDs — the authoritative store. An upstream Git
registry exists for curated bundles but is a sync source, not the write path. Adding or
updating a check does not require a binary release.

**2. The Check Authoring API is the primary write interface.**
All check creation and modification goes through a typed HTTP API exposed by
`mendabot-proactive`. The LLM calls this API. Git sync is for curated upstream bundles only.
See `HLD-check-authoring.md`.

**3. CEL evaluation is a separate stateless workload.**
The `mendabot-evaluator` binary runs as a short-lived Kubernetes Job. It reads cluster
state, evaluates CEL expressions, and writes `AuditFinding` CRDs. It never runs inside
the controller reconcile loop. This keeps the controller responsive and makes evaluation
independently scalable.

**4. Trust is explicit and enforced structurally.**
Checks from the official curated registry are cosign-signed and evaluated in the full CEL
environment. Unsigned or LLM-authored checks run in a restricted CEL environment. Trust
is not an honour system.

**5. Checks have a maturity stage that drives automation.**
A check exists on a continuum from fully manual to fully automated. Maturity stage is
declared on the check and determines whether detection, resolution, and verification are
automated or human-driven. Promotion to stages that enable auto-remediation (stage 3+)
requires explicit human approval.

**6. Deduplication is CRD-based.**
`AuditFinding` objects are durable dedup state, mirroring the `RemediationJob` CRD pattern.
No external state store. Watcher restarts are safe.

**7. Secret redaction is structural, not instructional.**
The evaluator binary redacts sensitive field values before writing to `AuditFinding` CRDs.
The evaluator ServiceAccount has no access to Kubernetes `Secret` objects. LLM agents
never see raw secret values.

**8. The proactive engine is independently deployable.**
`mendabot-proactive` and `mendabot-evaluator` have no Go import dependency on
`mendabot-watcher`. Communication is exclusively through the Kubernetes API. Moving either
binary to a separate repository requires copying one `cmd/` directory and updating import
paths — no logic changes.

**9. Results are CRD-only in v1.**
`AuditFinding` CRDs in etcd are the result store. No external database. Trending and
historical queries are out of scope for v1.

---

## 3. Four-Binary Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                                          │
│                                                                              │
│  ┌──────────────────────────┐   ┌──────────────────────────────────────┐    │
│  │  mendabot-watcher        │   │  mendabot-proactive                  │    │
│  │  (Deployment)            │   │  (Deployment)                        │    │
│  │                          │   │                                      │    │
│  │  Reactive path:          │   │  CheckRegistryController             │    │
│  │  - NativeProviders       │   │  AuditController                     │    │
│  │  - RemediationJob        │   │  AuditFindingReconciler              │    │
│  │    Reconciler            │   │  CheckRegistrySync reconciler        │    │
│  │                          │   │  Check Authoring API (HTTP)          │    │
│  │  Unchanged.              │   │                                      │    │
│  └──────────┬───────────────┘   └──────────────┬───────────────────────┘    │
│             │ creates                           │ creates                    │
│             │                    ┌──────────────▼──────────────────────┐    │
│             │                    │  mendabot-evaluator Job             │    │
│             │                    │  (short-lived, per AuditSchedule    │    │
│             │                    │   cron tick or event trigger)       │    │
│             │                    │                                      │    │
│             │                    │  - reads CheckBundle CRDs           │    │
│             │                    │  - lists cluster resources          │    │
│             │                    │  - evaluates CEL expressions        │    │
│             │                    │  - redacts sensitive fields         │    │
│             │                    │  - writes AuditFinding CRDs        │    │
│             │                    └──────────────┬──────────────────────┘    │
│             │                                   │ writes                     │
│             │              ┌────────────────────▼────────────────────┐      │
│             │              │  AuditFinding CRDs                      │      │
│             │              │  (durable dedup state per violation)    │      │
│             │              └────────────────────┬────────────────────┘      │
│             │                                   │ reconciled by              │
│             │                    ┌──────────────▼──────────────────────┐    │
│             │                    │  AuditFindingReconciler              │    │
│             │                    │  (inside mendabot-proactive)        │    │
│             │                    │                                      │    │
│             │                    │  - persona loading + batching        │    │
│             │                    │  - spawns report agent Job          │    │
│             │                    │  - RemediationGate (internal)       │    │
│             │                    │    creates RemediationJob CRDs      │    │
│             │                    └──────────────┬──────────────────────┘    │
│             │                                   │ creates                    │
│  ┌──────────▼────────────────────────────────── ▼──────────────────────┐    │
│  │  RemediationJob CRDs                                                 │    │
│  │  (reactive and proactive paths converge here)                        │    │
│  └──────────┬───────────────────────────────────────────────────────────┘    │
│             │ creates                                                         │
│  ┌──────────▼──────────────────────┐                                         │
│  │  mendabot-agent Job             │                                         │
│  │  (short-lived; three variants)  │                                         │
│  │                                  │                                         │
│  │  PROMPT_VARIANT=reactive         │  ← investigate broken resource, PR     │
│  │  PROMPT_VARIANT=proactive        │  ← aggregate findings, deliver report  │
│  │  PROMPT_VARIANT=check-review     │  ← review check quality, propose edits │
│  └──────────────────────────────────┘                                         │
└─────────────────────────────────────────────────────────────────────────────┘
                    │ proactive agent opens PR / posts webhook
                    ▼
     ┌──────────────────────────────────┐
     │  GitOps repo / webhook endpoints │
     └──────────────────────────────────┘
```

### Binary responsibilities

| Binary | Kind | Responsibility |
|---|---|---|
| `mendabot-watcher` | Deployment | Reactive: watches K8s events → `RemediationJob` CRDs → agent Jobs |
| `mendabot-proactive` | Deployment | Proactive: check registry, audit scheduling, finding lifecycle, authoring API |
| `mendabot-evaluator` | Job (short-lived) | Stateless CEL worker: reads cluster state, writes `AuditFinding` CRDs |
| `mendabot-agent` | Job (short-lived) | LLM worker: reactive remediation, proactive reports, check quality review |

### ServiceAccounts

| ServiceAccount | Used by | Permissions summary |
|---|---|---|
| `mendabot-watcher` | watcher Deployment | Read core K8s resources; create/manage `RemediationJob` CRDs; create agent Jobs |
| `mendabot-proactive` | proactive Deployment | Read/write all proactive CRDs; read core K8s resources; create evaluator Jobs |
| `mendabot-evaluator-namespaced` | evaluator Jobs (namespaced scope) | Read namespaced resources (no Secrets); create/patch `AuditFinding` CRDs |
| `mendabot-evaluator-cluster` | evaluator Jobs (cluster scope) | Read namespaced + cluster-scoped resources (no Secrets); create/patch `AuditFinding` CRDs |
| `mendabot-agent` | agent Jobs | Read-only cluster access; gh CLI for PR creation; no Secrets |

`mendabot-proactive` selects `mendabot-evaluator-namespaced` or `mendabot-evaluator-cluster`
based on the CEL scopes required by the `CheckBundle` CRDs referenced in the `AuditSchedule`.
If any check requires a cluster-scoped resource (Node, ClusterRoleBinding, Namespace), the
cluster SA is used.

---

## 4. CRD Inventory

| CRD | Group | Created by | Reconciled by | Purpose |
|---|---|---|---|---|
| `RemediationJob` | `remediation.mendabot.io/v1alpha1` | watcher, AuditFindingReconciler | watcher RemediationJobReconciler | Reactive + proactive remediation Jobs |
| `CheckBundle` | `proactive.mendabot.io/v1alpha1` | CheckRegistryController, Authoring API | CheckRegistryController | Loaded check definitions |
| `CheckRegistrySync` | `proactive.mendabot.io/v1alpha1` | operator / Helm | CheckRegistryController | Upstream Git sync configuration |
| `AuditSchedule` | `proactive.mendabot.io/v1alpha1` | operator | AuditController | When, what, and where to audit |
| `AuditFinding` | `proactive.mendabot.io/v1alpha1` | evaluator | AuditFindingReconciler | Per-violation durable state |
| `PersonaProfile` | `proactive.mendabot.io/v1alpha1` | operator | read-only (AuditFindingReconciler) | Report delivery configuration |

All CRDs are in the `mendabot` namespace except where explicitly cluster-scoped.

Full CRD schemas are documented in `HLD-proactive-controller.md`. The `CheckBundle` authoring
flow and lifecycle are in `HLD-check-authoring.md`. The `AuditFinding` write path from the
evaluator is in `HLD-evaluator.md`.

---

## 5. Data Flows

### 5.1 Scheduled audit run

```
1. AuditController: cron tick fires for AuditSchedule "security-baseline-daily"
   → determine required CEL scopes from referenced CheckBundle CRDs
   → select evaluator ServiceAccount (namespaced or cluster)
   → create mendabot-evaluator Job with:
       AUDIT_SCHEDULE_REF=security-baseline-daily
       AUDIT_RUN_TICK=2026-02-26T02:00:00Z
       CHECK_BUNDLE_REFS=cis-k8s-1.8,nsa-k8s-hardening
       SCOPE_NAMESPACES=[]  (empty = cluster-wide)
   → update AuditSchedule.Status.LastRunAt

2. mendabot-evaluator Job:
   → reads CheckBundle CRDs: cis-k8s-1.8 (47 checks), nsa-k8s-hardening (31 checks)
   → enumerates cluster objects per scope using informer cache
   → evaluates 78 checks × ~300 objects = ~23,400 CEL evaluations
   → 14 violations detected
   → redacts sensitive field values in all violation details
   → for each violation:
       fingerprint = sha256(checkID + bundleName + namespace + kind + resourceName)
       if no existing non-terminal AuditFinding with same fingerprint:
         create AuditFinding (phase=Detected)
       else:
         patch AuditFinding.Status.LastDetectedAt only
   → enforces MAX_FINDINGS_PER_RUN cap (default 500)
   → exits 0

3. AuditFindingReconciler: aggregation window (60s) expires for this run tick
   → load PersonaProfile "sre-team"
   → write findings.json + persona.json to ConfigMap "mendabot-audit-<hash[:12]>"
   → create mendabot-agent Job (PROMPT_VARIANT=proactive)
   → all included findings: Detected → Reported
   → RemediationGate (internal): for qualifying findings, create RemediationJob CRDs
   → qualifying findings: Reported → Dispatched

4. mendabot-agent Job (PROMPT_VARIANT=proactive):
   → reads /etc/mendabot/audit/findings.json and persona.json
   → generates persona-adjusted report
   → POSTs structured JSON to webhook sinks
   → opens GitHub PR with markdown report
   → exits 0

5. RemediationJobReconciler (in mendabot-watcher, unchanged):
   → dispatches agent Jobs for each RemediationJob CRD
   → reactive agent investigates and opens fix PRs

6. AuditFindingReconciler: RemediationJob outcome mapping
   → succeeded (stage 3): AuditFinding → PendingVerification
   → succeeded (stages 4–5): AuditFinding stays Remediating until next CEL pass confirms clean
   → failed: AuditFinding → Detected (re-queued for next run)
```

### 5.2 Event-triggered check

```
1. User creates a ClusterRoleBinding granting cluster-admin to a ServiceAccount
   → controller-runtime informer in mendabot-proactive fires

2. AuditController: event matches trigger in AuditSchedule "security-realtime"
   → creates mendabot-evaluator Job scoped to rbac checks only
   → evaluator evaluates CEL for the specific changed resource
   → stabilisationSeconds: 0 → AuditFinding emitted immediately (phase=Detected)

3. AuditFindingReconciler: aggregation window expires
   → report dispatched to "security-team" persona
   → maturityStage=2-detected: no RemediationJob created

4. Finding remains open until human patches phase=Resolved
   or the next evaluation run finds the binding removed
```

### 5.3 Check quality review

```
1. AuditSchedule "weekly-check-review" (spec.type: check-review) cron fires
   → AuditController creates mendabot-agent Job (PROMPT_VARIANT=check-review)
   → no evaluator Job; no AuditFinding emission

2. mendabot-agent Job (PROMPT_VARIANT=check-review):
   → GET /api/v1/checks from Check Authoring API
   → reads AuditFinding CRDs to assess finding patterns per check
   → identifies: noisy checks (high suppression), broken checks (zero findings),
     promotion candidates (stable, low suppression, successful resolutions)
   → for each check to revise:
       POST /api/v1/checks/validate (test revised CEL)
       PATCH /api/v1/checks/{id} (update CEL or remediationHint)
   → for each promotion candidate:
       PUT /api/v1/checks/{id}/promote
       (if stage >= 3: writes pendingPromotion; requires human approval)
   → writes review summary report
   → exits 0

3. Operator reviews pendingPromotion entries via kubectl or future UI
   → approves or rejects each promotion proposal
```

### 5.4 Regression detection

```
On every scheduled evaluator run:
- evaluator re-evaluates all checks, including those with existing Resolved AuditFindings
- if a Resolved finding is now violating again:
    AuditController patches AuditFinding.Status.Phase: Resolved → Detected
    AuditFindingReconciler picks it up through normal dispatch pipeline
```

---

## 6. Check Maturity Model

Every check declares a `maturityStage` field. This drives how `AuditFinding` objects are
created, resolved, and what automation fires.

| Stage | Name | Detection | Resolution | Auto-remediation |
|---|---|---|---|---|
| `1-manual` | Manual audit | Human | Human | Never |
| `2-detected` | Automated detection | CEL (evaluator) | Human | Never |
| `3-remediated` | Automated remediation | CEL (evaluator) | `RemediationJob` | Optional — human confirms |
| `4-spot-checked` | Spot-checked | CEL (evaluator) | `RemediationJob` | Optional — auto-confirms |
| `5-automated` | Fully automated | CEL (evaluator) | `RemediationJob` | Optional — auto-confirms |
| `6-preventative` | Preventative | N/A — structurally prevented | N/A | N/A |

Promotion to stages 3–5 (which enable `RemediationJob` creation) requires explicit human
approval via the `pendingPromotion` approval gate. See `HLD-check-authoring.md` §5.

Stage transitions are one step at a time. The authoring API enforces this — no skipping.

---

## 7. Package Structure

```
k8s-mendabot-2/
├── cmd/
│   ├── watcher/
│   │   └── main.go                 # reactive binary entry point (existing)
│   ├── proactive/
│   │   └── main.go                 # proactive controller + authoring API entry point
│   └── evaluator/
│       └── main.go                 # CEL evaluation worker entry point
│
├── internal/
│   ├── shared/
│   │   ├── domain/                 # MaturityStage, AuditCategory, AuditSeverity,
│   │   │                           # ResourceRef — pure Go types, no K8s imports
│   │   ├── sink/                   # SinkDispatcher interface only
│   │   └── logging/                # Zap logger construction
│   │
│   ├── reactive/                   # renamed from current flat internal/ structure
│   │   ├── provider/               # SourceProviderReconciler, NativeProvider impls
│   │   ├── controller/             # RemediationJobReconciler
│   │   └── jobbuilder/             # agent Job construction
│   │   # MUST NOT import internal/proactive/ or internal/evaluator/
│   │
│   ├── proactive/                  # mendabot-proactive binary internals
│   │   ├── checkregistry/          # CheckBundle + CheckRegistrySync CRD types; Git puller; CEL validator
│   │   ├── controller/             # AuditController; AuditFindingReconciler; CheckRegistryController
│   │   ├── persona/                # PersonaProfile CRD types; persona loader; default persona
│   │   ├── sink/                   # webhook formatter; report builder; K8s Event emitter
│   │   └── authoring/              # Check Authoring API HTTP handlers; staging; promotion
│   │   # MUST NOT import internal/reactive/ or internal/evaluator/
│   │
│   └── evaluator/                  # mendabot-evaluator binary internals
│       ├── cel/                    # CEL env construction; expression evaluation; type-check
│       ├── enumerate/              # cluster object enumeration per scope
│       ├── redact/                 # sensitive field redaction before AuditFinding write
│       └── writer/                 # AuditFinding CRD create/update logic
│       # MUST NOT import internal/reactive/ or internal/proactive/
│
├── api/
│   └── v1alpha1/
│       ├── remediationjob_types.go # existing; minor additions only (see §12 Step 0d)
│       └── proactive_types.go      # CheckBundle, CheckRegistrySync, AuditSchedule,
│                                   # AuditFinding, PersonaProfile Go types
│
├── docker/
│   ├── Dockerfile.watcher          # existing; unchanged
│   ├── Dockerfile.agent            # existing; gains check-review prompt file
│   ├── Dockerfile.proactive        # new: multi-stage Go build → debian-slim
│   └── Dockerfile.evaluator        # new: multi-stage Go build → debian-slim (no LLM tools)
│
└── charts/mendabot/
    ├── crds/
    │   ├── remediationjob.yaml     # existing
    │   └── proactive.yaml          # new: all five proactive CRDs in one file
    └── templates/
        ├── ...                     # existing reactive templates
        ├── proactive-deployment.yaml
        ├── proactive-serviceaccount.yaml
        ├── proactive-rbac.yaml
        ├── evaluator-serviceaccounts.yaml
        ├── evaluator-rbac.yaml
        └── proactive-checksync.yaml  # default CheckRegistrySync object
```

### Import rules (enforced)

- `internal/shared/` — no imports from `internal/reactive/`, `internal/proactive/`, `internal/evaluator/`
- `internal/reactive/` — imports `internal/shared/` only; no proactive or evaluator imports
- `internal/proactive/` — imports `internal/shared/` only; no reactive or evaluator imports
- `internal/evaluator/` — imports `internal/shared/` only; no reactive or proactive imports
- Communication across binaries is exclusively through the Kubernetes API (CRD read/write)

---

## 8. RBAC Summary

Full RBAC detail is in the relevant sub-HLD. This table summarises the additions required.

### `mendabot-watcher` ServiceAccount — additions

| Resource | Verbs | Reason |
|---|---|---|
| `remediationjobs.remediation.mendabot.io` | existing | No change |

The watcher ServiceAccount requires no new permissions. Proactive `RemediationJob` objects
are created by `mendabot-proactive` using the proactive ServiceAccount, not the watcher SA.

### `mendabot-proactive` ServiceAccount

| Resource | Verbs | Scope |
|---|---|---|
| `checkbundles.proactive.mendabot.io` | `get`, `list`, `watch`, `create`, `update`, `patch` | ClusterRole |
| `checkregistrysyncs.proactive.mendabot.io` | `get`, `list`, `watch`, `update`, `patch` | ClusterRole |
| `auditschedules.proactive.mendabot.io` | `get`, `list`, `watch`, `update`, `patch` | ClusterRole |
| `auditfindings.proactive.mendabot.io` | `get`, `list`, `watch`, `create`, `update`, `patch` | ClusterRole |
| `personaprofiles.proactive.mendabot.io` | `get`, `list`, `watch` | ClusterRole |
| `remediationjobs.remediation.mendabot.io` | `get`, `list`, `watch`, `create`, `patch` | ClusterRole |
| `pods`, `deployments`, `statefulsets`, `nodes`, `jobs`, `namespaces` | `get`, `list`, `watch` | ClusterRole |
| `configmaps` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` | ClusterRole |
| `events` | `create`, `patch` | ClusterRole |
| `batch/v1 jobs` | `create`, `get`, `list`, `watch`, `delete` | ClusterRole |
| `secrets` | `get` (specific keys only for webhook URLs) | Role (mendabot namespace only) |

### `mendabot-evaluator-namespaced` ServiceAccount

| Resource | Verbs | Scope |
|---|---|---|
| `pods`, `deployments`, `statefulsets`, `jobs`, `persistentvolumeclaims` | `get`, `list`, `watch` | ClusterRole |
| `serviceaccounts`, `configmaps`, `networkpolicies`, `namespaces` | `get`, `list`, `watch` | ClusterRole |
| `auditfindings.proactive.mendabot.io` | `create`, `patch`, `get`, `list` | ClusterRole |
| `auditschedules.proactive.mendabot.io` | `get` | ClusterRole |
| `checkbundles.proactive.mendabot.io` | `get`, `list` | ClusterRole |
| `secrets` | **none** | — |

### `mendabot-evaluator-cluster` ServiceAccount

All permissions from `mendabot-evaluator-namespaced`, plus:

| Resource | Verbs | Scope |
|---|---|---|
| `nodes` | `get`, `list`, `watch` | ClusterRole |
| `clusterrolebindings`, `rolebindings` | `get`, `list`, `watch` | ClusterRole |
| `clusterroles`, `roles` | `get`, `list`, `watch` | ClusterRole |
| `secrets` | **none** | — |

---

## 9. Failure Modes

| Failure | Behaviour |
|---|---|
| Git registry pull fails | `CheckRegistrySync.Status` updated with error; existing `CheckBundle` CRDs remain active; retried at `syncInterval`; K8s Event emitted on the `CheckRegistrySync` object |
| CEL expression fails type-check at load | `CheckBundle.Status.Phase = ValidationFailed`; entire bundle skipped by evaluator; error in `status.validationErrors`; other bundles unaffected |
| CEL evaluation panics at runtime | Recovered by evaluator; check skipped; error logged with check ID and resource; other checks continue |
| Evaluator Job exceeds `activeDeadlineSeconds` | Job killed; `AuditSchedule.Status` updated with timeout; next cron tick creates a new evaluator Job |
| Evaluator hits `MAX_FINDINGS_PER_RUN` cap | Evaluator stops, writes warning annotation to `AuditSchedule.Status.LastRunWarning`, exits 0 with findings written so far |
| `AuditFinding` creation fails (API error) | Evaluator retries with exponential backoff; if cap exceeded, logs error and continues with remaining checks |
| Aggregation window expires with zero findings | No agent Job dispatched; `AuditSchedule.Status.LastRunFindings = 0`; normal |
| Webhook POST fails | Logged; `AuditFinding.Status.SinkDeliveries[].Status = Failed`; not retried automatically |
| GitHub PR creation fails | Logged; agent exits non-zero; Job retried once per `backoffLimit: 1`; `AuditFinding` reverts to `Detected` |
| Agent Job exceeds `activeDeadlineSeconds` | Job killed; `AuditFinding` transitions back to `Detected`; next schedule run re-dispatches |
| `RemediationJob` creation fails | `AuditFindingReconciler` returns error; requeues; `AuditFinding` remains in `Detected` |
| `PersonaProfile` not found | `AuditFindingReconciler` logs warning; uses built-in default persona; emits warning event on `AuditSchedule` |
| `mendabot-proactive` restarts | `AuditFinding` CRDs survive in etcd; reconcilers re-reconcile all non-terminal findings on startup |
| `mendabot-evaluator` container OOMKilled | Job fails; `AuditSchedule.Status` updated; next cron tick creates a new evaluator Job |
| Check Authoring API unavailable | In-cluster: K8s liveness probe restarts the pod; authoring calls fail with 503; no data loss (CRDs are the store) |
| Pending promotion approval never given | Check stays at current maturityStage indefinitely; `status.pendingPromotion` field visible via `kubectl get checkbundle`; no automatic expiry in v1 |

---

## 10. Configuration Reference

### New Helm values (`proactive:` key — entirely net-new)

```yaml
proactive:
  enabled: true                         # master switch

  checkRegistry:
    syncInterval: 24h
    gitRepo: https://github.com/lenaxia/mendabot-checks
    gitRef: main
    signingKeyConfigMap: mendabot-check-signing-key
    signingKeyConfigMapKey: cosign.pub

  auditController:
    aggregationWindowSeconds: 60
    maxFindingsPerRun: 500

  authoring:
    enabled: true                       # enable Check Authoring API
    port: 8081                          # separate from metrics/health port
```

### New environment variables

All gated behind `PROACTIVE_ENABLED=true`.

| Variable | Default | Binary | Purpose |
|---|---|---|---|
| `PROACTIVE_ENABLED` | `false` | proactive | Master switch |
| `CHECK_REGISTRY_SYNC_INTERVAL` | `24h` | proactive | Git sync cadence |
| `CHECK_REGISTRY_GIT_REPO` | (from Helm) | proactive | Git URL of check registry |
| `CHECK_REGISTRY_GIT_REF` | `main` | proactive | Branch, tag, or SHA |
| `AUDIT_AGGREGATION_WINDOW_SECONDS` | `60` | proactive | Batching window before report dispatch |
| `PROACTIVE_AUTO_REMEDIATE_ENABLED` | `false` | proactive | Global auto-remediation override |
| `AUTHORING_API_ENABLED` | `true` | proactive | Enable/disable the HTTP authoring API |
| `AUTHORING_API_PORT` | `8081` | proactive | HTTP port for authoring API |
| `AUDIT_SCHEDULE_REF` | (set by proactive per Job) | evaluator | Which schedule triggered this run |
| `AUDIT_RUN_TICK` | (set by proactive per Job) | evaluator | RFC3339 cron tick timestamp |
| `CHECK_BUNDLE_REFS` | (set by proactive per Job) | evaluator | Comma-separated bundle names |
| `SCOPE_NAMESPACES` | (set by proactive per Job) | evaluator | Comma-separated namespaces; empty = all |
| `MAX_FINDINGS_PER_RUN` | `500` | evaluator | Hard cap on AuditFinding writes per Job |

---

## 11. What Does Not Change

The following components are **unchanged** by the proactive engine:

- `RemediationJobReconciler` in `mendabot-watcher` — drives reactive and proactive-triggered
  `RemediationJob` lifecycle identically. Source-type-agnostic.
- `batch/v1 Job` construction (`jobbuilder`) — extended with new `PROMPT_VARIANT` and
  `AUDIT_*` env vars for proactive Jobs, but structure is unchanged.
- GitHub authentication flow — unchanged; the proactive agent Job uses the same init
  container token exchange pattern.
- `RemediationJob` CRD schema — two minor additions only:
  1. `spec.remediationHint string` — carries the check's remediation hint to the agent.
  2. `spec.sourceType` gains `"proactive"` as a valid value.
  All other fields and status fields unchanged.
- Cascade suppression, circuit breaker logic — unchanged for the reactive path.
- Agent Docker image structure — gains two prompt files (`proactive`, `check-review`) and
  handles new env vars; build and entrypoint script structure unchanged.
- `NativeProviders` (Pod, Deployment, StatefulSet, PVC, Node, Job) — unchanged.

---

## 12. Implementation Prerequisites

These must be completed before any proactive controller code is written. Each is a
self-contained change that compiles and passes tests independently.

**Step 0a: Package structure refactor**
Rename `internal/provider/` → `internal/reactive/provider/`,
`internal/controller/` → `internal/reactive/controller/`,
`internal/jobbuilder/` → `internal/reactive/jobbuilder/`,
`internal/domain/` → `internal/shared/domain/`,
`internal/logging/` → `internal/shared/logging/`.
Create `internal/proactive/`, `internal/evaluator/`, `internal/shared/sink/` as empty
packages with stub `doc.go` files. Update all import paths. Build and all tests must pass.

**Step 0b: Proactive CRD types**
Add `api/v1alpha1/proactive_types.go` with Go types for `CheckBundle`, `CheckRegistrySync`,
`AuditSchedule`, `AuditFinding`, and `PersonaProfile`. Add `AddProactiveToScheme()`.
Generate CRD YAML. Add to `charts/mendabot/crds/`. Register scheme in both
`cmd/watcher/main.go` and `cmd/proactive/main.go`. Build must pass.

**Step 0c: Proactive config fields**
Add `Proactive ProactiveConfig` struct to config. Add `FromEnv()` parsing for all
proactive variables. Add `Evaluator EvaluatorConfig` for evaluator variables. Build must pass.

**Step 0d: RemediationJob spec extension**
Add `RemediationHint string` field and `SourceTypeProactive = "proactive"` constant to
`api/v1alpha1/remediationjob_types.go`. Update CRD YAML in **all three locations**:
1. `testdata/crds/remediationjob_crd.yaml`
2. `charts/mendabot/crds/remediationjob.yaml`
3. `deploy/kustomize/crd-remediationjob.yaml`
Build and all tests must pass.

**Step 0e: cmd/proactive and cmd/evaluator entry points**
Add `cmd/proactive/main.go` and `cmd/evaluator/main.go` as stub entry points that compile
but do nothing. Add corresponding `Dockerfile.proactive` and `Dockerfile.evaluator`.
Add GitHub Actions workflows: `build-proactive.yaml`, `build-evaluator.yaml`. Build must pass.

**Step 0f: Feature flag wiring in cmd/watcher/main.go**
No change required — the watcher does not call proactive packages. The proactive engine
is a separate binary from day one.

**Step 0g: Helm chart default `CheckRegistrySync` object**
Add `charts/mendabot/templates/proactive-checksync.yaml` rendering a `CheckRegistrySync`
named `official` when `proactive.enabled: true`. Pre-configured for `lenaxia/mendabot-checks`.

**Step 0h: Evaluator ServiceAccounts**
Add `charts/mendabot/templates/evaluator-serviceaccounts.yaml` and
`charts/mendabot/templates/evaluator-rbac.yaml` for both evaluator ServiceAccounts.

### Recommended epic order

```
Step 0a → Step 0b → Step 0c → Step 0d → Step 0e → Step 0g → Step 0h
  ↓
epic14: CheckRegistryController + CheckBundle/CheckRegistrySync CRDs
  ↓
epic15: AuditSchedule CRD + AuditController (creates evaluator Jobs)
  ↓
epic16: mendabot-evaluator binary (CEL evaluation, AuditFinding write)
  ↓
epic17: AuditFinding CRD + AuditFindingReconciler (report dispatch)
epic18: PersonaProfile CRD + persona loading          } parallel
  ↓
epic19: Sink extension (webhook, GitHub PR report)
  ↓
epic20: Check Authoring API + check lifecycle
  ↓
epic21: check-review agent prompt variant
epic22: initial curated check bundles (lenaxia/mendabot-checks repo)  } parallel
```

---

## 13. Scope and Success Criteria

### In scope

- Four-binary monorepo: `mendabot-watcher` (unchanged), `mendabot-proactive`, `mendabot-evaluator`, `mendabot-agent` (extended)
- All five proactive CRDs: `CheckBundle`, `CheckRegistrySync`, `AuditSchedule`, `AuditFinding`, `PersonaProfile`
- `CheckRegistryController`: Git pull, CEL validation, trust enforcement
- `AuditController`: cron triggers, event triggers, evaluator Job creation
- `mendabot-evaluator`: CEL evaluation, cluster enumeration, secret redaction, `AuditFinding` write
- `AuditFindingReconciler`: maturity lifecycle, persona loading, batch aggregation, report agent dispatch, auto-remediation gate (internal)
- `PersonaProfile` CRD: schema, loader, default fallback
- Proactive agent prompt variant: findings synthesis, persona-adjusted output, webhook POST, GitHub PR
- Check-review agent prompt variant: check quality assessment, authoring API calls
- Webhook sink: structured JSON payload
- GitHub PR report sink: markdown report, branch naming, update-vs-create logic
- Check Authoring API: validate, stage, promote, deprecate, list, get
- Check lifecycle: Proposed → Staged → Active → maturity stages → Deprecated → Deleted
- Human approval gate: `pendingPromotion` for stage 3+ promotions
- `AuditSchedule` `spec.type: audit | check-review`
- Feature flag: `PROACTIVE_ENABLED` gating all proactive controllers
- Two evaluator ServiceAccounts with scope-appropriate RBAC
- Secret redaction in evaluator before `AuditFinding` write
- `MAX_FINDINGS_PER_RUN` cap in evaluator
- Helm chart extensions: new values under `proactive:` key
- Initial curated check bundles in `lenaxia/mendabot-checks` (epic 22)

### Out of scope (v1)

- `AuditRun` CRD and trending/historical queries (deferred to v2)
- External result database
- Scripted checks (CEL extension point exists in schema; execution deferred)
- Persona microservice (CRD is the interface; service is a future upgrade)
- Cloud provider checks (AWS, GCP, Azure) — K8s scope only
- Admission webhook integration (stage 6 preventative controls)
- Web dashboard
- Multi-cluster proactive scanning
- Guidance ingestion pipeline (external policy docs → LLM → checks)
- Keyless cosign signing (Fulcio/Rekor)
- Pending promotion expiry / auto-rejection

### Success criteria

- [ ] `PROACTIVE_ENABLED=false` (default) produces identical behaviour to the existing binary; all existing tests pass
- [ ] `mendabot-watcher`, `mendabot-proactive`, `mendabot-evaluator` each compile independently
- [ ] `internal/reactive/`, `internal/proactive/`, `internal/evaluator/` have no cross-imports (enforced by build tags or linter)
- [ ] A valid `CheckRegistrySync` pointing to `lenaxia/mendabot-checks` results in `CheckBundle` CRDs in etcd
- [ ] A bundle with an invalid CEL expression is written with `status.phase: ValidationFailed` and is not executed
- [ ] An `AuditSchedule` with a cron trigger causes `mendabot-proactive` to create a `mendabot-evaluator` Job at the scheduled time
- [ ] The evaluator Job reads `CheckBundle` CRDs, evaluates CEL, and writes `AuditFinding` CRDs
- [ ] Sensitive field values (env vars matching `*SECRET*`, `*PASSWORD*`, etc.) are redacted in `AuditFinding.spec.details`
- [ ] Duplicate findings (same fingerprint, non-terminal existing finding) do not create a second `AuditFinding`
- [ ] A regression (resolved finding re-detected) transitions the finding back to `Detected`
- [ ] `MAX_FINDINGS_PER_RUN` is enforced; a run that would produce more findings writes a warning and stops cleanly
- [ ] The proactive agent Job reads findings and persona from ConfigMap volume and produces a correctly formatted webhook payload
- [ ] A GitHub PR is opened for a schedule with a `github-pr` sink; subsequent runs update the branch rather than opening a duplicate PR
- [ ] `autoRemediate: true` + `remediationAllowed: true` + `maturityStage: 3-remediated` creates a `RemediationJob`; `advisory` checks never create one
- [ ] `PUT /checks/{id}/promote` for a stage 3+ target writes `status.pendingPromotion` and does not apply the promotion until human approval
- [ ] Check-review agent Job reads check summaries, calls authoring API to update CEL/hints, and proposes promotions
- [ ] All tests pass with the race detector: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds for all three new entry points
