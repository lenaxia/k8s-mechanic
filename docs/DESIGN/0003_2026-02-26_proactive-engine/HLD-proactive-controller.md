# High-Level Design — Proactive Controller (`mendabot-proactive`)

**Version:** 1.0
**Date:** 2026-02-26
**Status:** Proposed
**Parent:** [`HLD.md`](HLD.md)

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-26 | Initial: extracted and revised from HLD v3.2 §§5–16 | LLM / Human |

---

## Table of Contents

1. [Binary Overview](#1-binary-overview)
2. [Check Registry](#2-check-registry)
3. [CheckBundle CRD](#3-checkbundle-crd)
4. [CheckRegistrySync CRD](#4-checkregistrysync-crd)
5. [CheckRegistryController](#5-checkregistrycontroller)
6. [AuditSchedule CRD](#6-auditschedule-crd)
7. [AuditController](#7-auditcontroller)
8. [AuditFinding CRD](#8-auditfinding-crd)
9. [AuditFindingReconciler](#9-auditfindingreconciler)
10. [PersonaProfile CRD](#10-personaprofile-crd)
11. [Proactive Agent Prompt Variant](#11-proactive-agent-prompt-variant)
12. [Sink Extension](#12-sink-extension)
13. [Auto-Remediation Integration](#13-auto-remediation-integration)
14. [Failure Modes](#14-failure-modes)

---

## 1. Binary Overview

`mendabot-proactive` runs as a long-lived Kubernetes Deployment. It contains:

- **`CheckRegistryController`** — syncs upstream Git check bundles into `CheckBundle` CRDs
- **`AuditController`** — manages `AuditSchedule` cron and event triggers; creates evaluator Jobs
- **`AuditFindingReconciler`** — drives `AuditFinding` lifecycle: batching, persona loading, report dispatch, auto-remediation gate
- **`CheckRegistrySync` reconciler** — watches `CheckRegistrySync` CRDs and triggers Git pulls
- **Check Authoring API** — HTTP API for LLM-driven check management (see `HLD-check-authoring.md`)

The binary does **not** run CEL evaluation. CEL evaluation is the responsibility of the
`mendabot-evaluator` Job, which `mendabot-proactive` spawns. See `HLD-evaluator.md`.

Entry point: `cmd/proactive/main.go`.

---

## 2. Check Registry

### Purpose

Checks are loaded from two sources:

1. **Upstream Git registry** (`lenaxia/mendabot-checks`) — curated, cosign-signed bundles
   maintained by the project. Pulled on startup and at `syncInterval`. These become
   `CheckBundle` CRDs with `spec.origin: git-sync` and `spec.trustLevel: curated`.

2. **Check Authoring API** — checks proposed and staged by operators or LLM agents.
   These become `CheckBundle` CRDs with `spec.origin: api` and `spec.trustLevel: custom`.

`CheckBundle` CRDs are the **authoritative store**. The Git registry is one write path;
the authoring API is another. The evaluator reads only from CRDs — it is source-agnostic.

### Git registry structure

```
lenaxia/mendabot-checks/
  bundles/
    cis-k8s-1.8/bundle.yaml
    nsa-k8s-hardening/bundle.yaml
    cncf-security-baseline/bundle.yaml
    reliability-baseline/bundle.yaml
    cost-baseline/bundle.yaml
  schema/
    check-bundle.schema.json
    validate.sh
  .github/workflows/
    validate.yaml     # runs validate.sh on every PR
    sign.yaml         # cosign signs bundles on merge to main
```

### Trust model

| Trust level | Criteria | CEL environment |
|---|---|---|
| `curated` | Bundle has valid cosign signature from `lenaxia/mendabot-checks` | Full CEL environment |
| `custom` | No valid signature, or API-authored | Restricted CEL (whitelisted functions; no cross-resource navigation) |

### Cosign verification

Signature verification uses the `github.com/sigstore/cosign/v2/pkg/cosign` Go library.
No `cosign` binary required in the container image.

Per bundle:
1. Load the public key from the ConfigMap referenced by `spec.trustPublicKeyRef` in the `CheckRegistrySync` object.
2. Read `spec.signature` from the bundle YAML (written by the registry's `sign.yaml` CI workflow).
3. Call `cosign.VerifyBlobSignature` with the public key and raw bundle YAML bytes.
4. Valid signature → `trustLevel: curated`. Invalid or absent → `trustLevel: custom`.

Classic key-pair signing is used. Keyless (Fulcio/Rekor) is not supported in v1.

### Git clone mechanism

The `CheckRegistryController` clones using `github.com/go-git/go-git/v5`. No `git` binary
required.

Clone workflow:
1. `git.PlainClone` with `Depth: 1` into `/tmp/mendabot-registry/<registry-name>` on first run.
2. `repository.Fetch` on each sync tick. If remote SHA matches `status.lastSyncRevision`, skip re-validation.
3. Private registries: `http.BasicAuth` transport from the Secret referenced by `spec.authSecretRef`.
4. Clone directory is ephemeral. No PersistentVolume required.

---

## 3. CheckBundle CRD

Group: `proactive.mendabot.io/v1alpha1`
Kind: `CheckBundle`
Short name: `cbundle`

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: CheckBundle
metadata:
  name: cis-k8s-1.8
  namespace: mendabot
  labels:
    proactive.mendabot.io/trust-level: curated
    proactive.mendabot.io/origin: git-sync   # git-sync | api
spec:
  sourceRepo: https://github.com/lenaxia/mendabot-checks
  sourceRevision: abc1234def5678
  trustLevel: curated          # curated | custom
  origin: git-sync             # git-sync | api
  signature: "sha256:..."      # cosign signature; empty for custom/api bundles
  checks:
    - id: no-root-containers
      category: security
      checkClass: enforcement
      severity: high
      maturityStage: "3-remediated"
      frameworks:
        - CIS-K8s-1.8:5.2.6
      description: "Containers must not run as root"
      scope: container
      cel: |
        container.securityContext.runAsNonRoot == true
      remediationAllowed: true
      remediationHint: |
        Set securityContext.runAsNonRoot: true and securityContext.runAsUser to a
        non-zero UID in the Pod template spec.
      stabilisationSeconds: 0
      failureMessageTemplate: |
        Container "{{.Container.Name}}" in {{.Resource.Kind}} "{{.Resource.Name}}"
        (namespace "{{.Resource.Namespace}}") is running as root.
      script: ~    # reserved; ignored by v1 engine
status:
  phase: Active           # Staged | Active | Deprecated | Deleted | ValidationFailed
  origin: git-sync        # mirrors spec.origin
  validatedAt: "2026-02-26T10:00:00Z"
  checkCount: 47
  validationErrors: []
  pendingPromotion: ~     # populated when an LLM proposes a maturity stage increase
    # targetStage: "3-remediated"
    # proposedAt: "2026-02-26T10:00:00Z"
    # proposedBy: "check-review-agent"
    # proposedReason: "Check stable for 14 days; suppression rate 2%; 0 failed remediations"
```

### CheckBundle phases

| Phase | Meaning |
|---|---|
| `Staged` | Check written via authoring API; not yet evaluated by the evaluator |
| `Active` | Evaluator runs this bundle on schedule |
| `Deprecated` | No new `AuditFinding` objects created; existing open findings remain |
| `Deleted` | Soft-deleted; no open findings; will be garbage-collected |
| `ValidationFailed` | One or more CEL expressions failed parse or type-check |

### CheckSpec fields

| Field | Type | Description |
|---|---|---|
| `id` | `string` | Unique within bundle; part of `AuditFinding` fingerprint |
| `category` | `string` | `security`, `reliability`, `cost`, `operations` |
| `checkClass` | `string` | `enforcement`, `advisory`, `informational` |
| `severity` | `string` | `critical`, `high`, `medium`, `low`, `info` |
| `maturityStage` | `string` | See HLD.md §6 |
| `frameworks` | `[]string` | Originating frameworks; freeform; used in reports |
| `scope` | `string` | Kubernetes object type the CEL expression evaluates against |
| `cel` | `string` | CEL expression returning bool; `true` = compliant, `false` = violation |
| `remediationAllowed` | `bool` | Whether this check may trigger a `RemediationJob` |
| `remediationHint` | `string` | Human-readable fix hint; included in `AuditFinding` and agent prompt |
| `failureMessageTemplate` | `string` | Go `text/template` string; data struct fields: `{{.Resource.Kind}}`, `{{.Resource.Name}}`, `{{.Resource.Namespace}}`, `{{.Container.Name}}` (container scope only), `{{.Check.ID}}`. Capped at 4,096 bytes. |
| `stabilisationSeconds` | `int` | For event-triggered checks: seconds to wait before emitting a finding |
| `script` | `string` | Reserved; must be empty in v1; ignored by engine |

### Check-class auto-remediation constraint

| `checkClass` | `remediationAllowed` may be `true` | Rationale |
|---|---|---|
| `enforcement` | Yes | Clear violation; fix is well-understood |
| `advisory` | No — must always be `false` | Context-dependent; human judgment required |
| `informational` | No — must always be `false` | Awareness only |

The `CheckRegistryController` rejects any bundle where `advisory` or `informational` checks
have `remediationAllowed: true`. Validation error, not a warning.

---

## 4. CheckRegistrySync CRD

Group: `proactive.mendabot.io/v1alpha1`
Kind: `CheckRegistrySync`
Short name: `checksync`

One object per upstream Git registry. The Helm chart installs one named `official` for
`lenaxia/mendabot-checks` when `proactive.enabled: true`.

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: CheckRegistrySync
metadata:
  name: official
  namespace: mendabot
spec:
  gitRepo: https://github.com/lenaxia/mendabot-checks
  gitRef: main
  syncInterval: 24h
  trustPublicKeyRef:
    name: mendabot-check-signing-key
    key: cosign.pub
  authSecretRef: ~
  bundles: []              # empty = load all; non-empty = named bundle directories only
status:
  lastSyncAt: "2026-02-26T10:00:00Z"
  lastSyncRevision: abc1234def5678
  bundlesLoaded: 5
  bundlesFailed: 0
  conditions:
    - type: Ready
      status: "True"
```

Manual refresh: annotate with `proactive.mendabot.io/force-sync: "true"`. The controller
processes the annotation, clears it, and triggers an immediate pull.

Custom registries: apply additional `CheckRegistrySync` objects. The Helm chart owns only
the `official` object.

---

## 5. CheckRegistryController

Package: `internal/proactive/checkregistry/`

Responsibilities:
1. Watch `CheckRegistrySync` CRDs.
2. On startup and at `spec.syncInterval`, pull the Git registry using go-git.
3. For each bundle YAML, verify cosign signature and set `trustLevel`.
4. Validate every CEL expression (parse + type-check; not Eval). See `HLD-evaluator.md` §3.
5. Write or update `CheckBundle` CRDs in etcd.
6. Set `CheckBundle.Status.Phase = ValidationFailed` for bundles with invalid expressions.
7. Handle the `force-sync` annotation for manual refresh.

The controller does **not** manage `CheckBundle` objects created by the authoring API
(`spec.origin: api`). Those are owned by the authoring API handler.

---

## 6. AuditSchedule CRD

Group: `proactive.mendabot.io/v1alpha1`
Kind: `AuditSchedule`
Short name: `asched`

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: AuditSchedule
metadata:
  name: security-baseline-daily
  namespace: mendabot
spec:
  type: audit              # audit | check-review
                           # check-review: no checkBundleRefs or scope needed;
                           # spawns check-review agent Job instead of evaluator Job

  # Fields below apply only to type: audit
  checkBundleRefs:
    - cis-k8s-1.8
    - nsa-k8s-hardening

  trigger:
    cron: "0 2 * * *"
    events:
      - kind: Pod
        apiVersion: v1
        verbs: [create, update]
      - kind: ClusterRoleBinding
        apiVersion: rbac.authorization.k8s.io/v1
        verbs: [create]

  scope:
    namespaces: []           # empty = cluster-wide
    labelSelector: {}        # namespace label selector

  personaRef: sre-team

  sinks:
    - type: webhook
      webhookRef:
        name: slack-webhook-secret
        key: url
      headers: {}
    - type: github-pr
      repo: lenaxia/talos-ops-prod

  autoRemediate: false

status:
  lastRunAt: "2026-02-26T02:00:00Z"
  lastRunFindings: 14
  lastRunDuration: 45s
  lastRunWarning: ""         # populated if evaluator hit MAX_FINDINGS_PER_RUN cap
  nextRunAt: "2026-02-27T02:00:00Z"
  evaluatorJobRef: ""        # name of the most recent evaluator Job
  conditions:
    - type: Ready
      status: "True"
```

### `spec.type: audit`

Default. Cron or event trigger creates a `mendabot-evaluator` Job. Evaluator writes
`AuditFinding` CRDs. `AuditFindingReconciler` dispatches the report agent Job.

### `spec.type: check-review`

Cron trigger only (event triggers are not meaningful for check review). Creates a
`mendabot-agent` Job with `PROMPT_VARIANT=check-review`. No evaluator Job. No `AuditFinding`
emission. The agent interacts with the Check Authoring API. See `HLD-check-authoring.md`.

### Trigger modes

**Cron trigger:** `github.com/robfig/cron/v3` schedules one entry per `AuditSchedule`.
Cron entry is added, removed, or replaced when the schedule is created, deleted, or updated.
The `cron.Cron` instance is started as a leader-election-aware `Runnable` registered via
`mgr.Add()`. Each cron callback enqueues a reconcile request for the `AuditSchedule` object
into the controller-runtime work queue.

**Event trigger:** Controller-runtime does not support adding watches after `mgr.Start()`.
The `AuditController` pre-registers watches for **all supported event-trigger types** at
`SetupWithManager` time. When any pre-registered resource changes, a predicate queries
the informer cache for all `AuditSchedule` objects and enqueues a reconcile request only
for schedules whose `spec.trigger.events` includes the changed resource's kind.

### Scope → event-kind canonical mapping

| CEL scope | Kubernetes Kind | API version |
|---|---|---|
| `container`, `pod` | Pod | `v1` |
| `deployment` | Deployment | `apps/v1` |
| `statefulset` | StatefulSet | `apps/v1` |
| `job` | Job | `batch/v1` |
| `namespace` | Namespace | `v1` |
| `node` | Node | `v1` |
| `rbac` | ClusterRoleBinding, RoleBinding | `rbac.authorization.k8s.io/v1` |
| `networkpolicy` | NetworkPolicy | `networking.k8s.io/v1` |
| `pvc` | PersistentVolumeClaim | `v1` |
| `serviceaccount` | ServiceAccount | `v1` |
| `configmap` | ConfigMap | `v1` |
| `cluster` | N/A | N/A — cron trigger only |

---

## 7. AuditController

Package: `internal/proactive/controller/`

Responsibilities:
1. Maintain cron entries (one per `AuditSchedule`). Add/remove/replace on reconcile.
2. On cron tick or event trigger: determine required CEL scopes from referenced `CheckBundle` CRDs.
3. Select evaluator ServiceAccount: `mendabot-evaluator-namespaced` or `mendabot-evaluator-cluster`
   based on whether any referenced check requires a cluster-scoped resource.
4. Create a `mendabot-evaluator` Job with scope-appropriate env vars.
5. Update `AuditSchedule.Status.LastRunAt`, `NextRunAt`, `EvaluatorJobRef`.
6. For `spec.type: check-review`: create a `mendabot-agent` Job with `PROMPT_VARIANT=check-review`
   (no evaluator Job).
7. On subsequent reconcile after evaluator Job completion: update `AuditSchedule.Status.LastRunFindings`,
   `LastRunDuration`, `LastRunWarning`.

### Evaluator Job spec (created by AuditController)

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: mendabot-evaluator-<scheduleRef[:8]>-<runTick-hash[:8]>
  namespace: mendabot
  labels:
    proactive.mendabot.io/schedule-ref: security-baseline-daily
    proactive.mendabot.io/run-tick: "2026-02-26T02:00:00Z"
spec:
  backoffLimit: 1
  activeDeadlineSeconds: 600          # 10 min hard timeout
  ttlSecondsAfterFinished: 86400
  template:
    spec:
      serviceAccountName: mendabot-evaluator-cluster  # or -namespaced
      restartPolicy: Never
      containers:
        - name: evaluator
          image: ghcr.io/lenaxia/mendabot-evaluator:<tag>
          env:
            - name: AUDIT_SCHEDULE_REF
              value: security-baseline-daily
            - name: AUDIT_RUN_TICK
              value: "2026-02-26T02:00:00Z"
            - name: CHECK_BUNDLE_REFS
              value: "cis-k8s-1.8,nsa-k8s-hardening"
            - name: SCOPE_NAMESPACES
              value: ""                # empty = cluster-wide
            - name: MAX_FINDINGS_PER_RUN
              value: "500"
            - name: TRIGGER_KIND      # populated for event triggers
              value: ""
            - name: TRIGGER_RESOURCE_NAME
              value: ""
            - name: TRIGGER_RESOURCE_NAMESPACE
              value: ""
```

### ServiceAccount selection

```
required_scopes = union of all scope fields across all checks in referenced CheckBundle CRDs

if required_scopes ∩ {node, rbac, cluster} ≠ ∅:
    serviceAccountName = mendabot-evaluator-cluster
else:
    serviceAccountName = mendabot-evaluator-namespaced
```

### Leader election

The `robfig/cron` scheduler runs inside a `LeaderElectionRunnable` registered via
`mgr.Add()`. The cron instance is started only on the elected leader and stopped on
leader loss. In the current single-replica deployment this gate is inactive but the
structure is in place.

---

## 8. AuditFinding CRD

Group: `proactive.mendabot.io/v1alpha1`
Kind: `AuditFinding`
Short name: `afinding`

The `AuditFinding` CRD is the durable dedup state for proactive violations. Created by the
evaluator. Reconciled by the `AuditFindingReconciler`.

### Fingerprint

```
fingerprint = sha256(checkID + bundleName + namespace + kind + resourceName)
```

Cross-schedule deduplication: the fingerprint does not include `scheduleRef`. Two schedules
detecting the same violation produce one `AuditFinding`. The second detection updates
`status.lastDetectedAt` only.

### Schema

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: AuditFinding
metadata:
  name: afinding-<fingerprint[:12]>
  namespace: mendabot
  labels:
    proactive.mendabot.io/fingerprint: <fingerprint[:12]>
    proactive.mendabot.io/check-id: no-root-containers
    proactive.mendabot.io/category: security
    proactive.mendabot.io/severity: high
    proactive.mendabot.io/maturity-stage: "3-remediated"
  annotations:
    proactive.mendabot.io/fingerprint-full: <64-char sha256>
    proactive.mendabot.io/run-tick: "2026-02-26T02:00:00Z"
    proactive.mendabot.io/check-deprecated: "false"  # set true when parent CheckBundle deprecated
spec:
  checkID: no-root-containers
  bundleName: cis-k8s-1.8
  checkClass: enforcement
  category: security
  severity: high
  frameworks:
    - CIS-K8s-1.8:5.2.6
  maturityStageAtCreation: "3-remediated"   # immutable
  affectedResource:
    apiVersion: v1
    kind: Pod
    namespace: default
    name: web-abc123
    parentObject: Deployment/web
  fingerprint: <64-char sha256>
  details: |
    Container "app" in Pod "web-abc123" has securityContext.runAsNonRoot unset.
    [sensitive fields redacted by evaluator]
  remediationAllowed: true
  remediationHint: |
    Set securityContext.runAsNonRoot: true and securityContext.runAsUser to a
    non-zero UID in the Pod template spec.
  scheduleRef: security-baseline-daily
status:
  phase: Detected
  firstDetectedAt: "2026-02-26T02:00:00Z"
  lastDetectedAt: "2026-02-26T02:00:00Z"
  reportRunID: "security-baseline-daily/2026-02-26T02:00:00Z"
  reportedAt: ~
  remediationJobRef: ~
  suppressedUntil: ~
  suppressedReason: ~
  verificationRequiredBy: ~
  sinkDeliveries:
    - sinkType: webhook
      deliveredAt: "2026-02-26T02:05:00Z"
      status: Delivered
```

### Phase lifecycle

| Phase | Meaning | Triggered by |
|---|---|---|
| `Detected` | CEL violation confirmed; no report dispatched yet | Evaluator creates the `AuditFinding` |
| `Reported` | Aggregated into a report agent Job | `AuditFindingReconciler` after report agent Job created |
| `Dispatched` | A `RemediationJob` has been created | `AuditFindingReconciler` (RemediationGate) after creating `RemediationJob` CRD |
| `Remediating` | `RemediationJob` is running | `AuditFindingReconciler` watching `RemediationJob` status |
| `PendingVerification` | `RemediationJob` succeeded; awaiting human (stage 3 only) | `AuditFindingReconciler` after `RemediationJob` reaches `Succeeded` |
| `Resolved` | Violation confirmed gone | Human patch (stages 2–3) or automatic on next clean CEL eval (stages 4–5) |
| `Suppressed` | Risk accepted until `suppressedUntil` | Human patch |

```
Stage 2-detected:
  Detected → Reported → [human] → Resolved | Suppressed

Stage 3-remediated (autoRemediate=true):
  Detected → Reported → Dispatched → Remediating → PendingVerification
  → [human patches Resolved] → Resolved

Stage 3-remediated (autoRemediate=false):
  Detected → Reported → [human] → Resolved | Suppressed

Stage 4–5 (autoRemediate=true):
  Detected → Reported → Dispatched → Remediating
  → [RemediationJob Succeeded + next CEL eval passes] → Resolved

Resolved → Detected  (regression: evaluator re-detects violation)
Suppressed → Detected  (expiry: suppressedUntil passed; reconciler reopens)
```

### Regression detection

On every evaluator run, the evaluator re-evaluates all active checks — including checks
for resources that have existing `Resolved` `AuditFinding` objects. If a `Resolved` finding
is detected violating again, the evaluator patches the phase to `Detected`. The
`AuditFindingReconciler` picks it up through the normal dispatch pipeline.

The `AuditFindingReconciler` does **not** run CEL. It owns lifecycle transitions driven
by K8s API events (Job completion, human patches, suppression expiry) only.

---

## 9. AuditFindingReconciler

Package: `internal/proactive/controller/`

The `AuditFindingReconciler` is a single controller-runtime reconciler internally decomposed
into two files:
- `internal/proactive/controller/findings.go` — batch aggregation, persona loading, report dispatch
- `internal/proactive/controller/remediation.go` — RemediationGate: auto-remediation decision, `RemediationJob` creation, outcome mapping

This is an internal code organisation decision. There is one reconciler, not two, to avoid
race conditions on `AuditFinding` phase transitions.

### Responsibilities: findings.go

**1. Persona loading**
On reconcile, load the `PersonaProfile` CRD referenced by the finding's `scheduleRef` →
`AuditSchedule.Spec.PersonaRef`. Use informer cache (no live API call per finding).

If the `AuditSchedule` has been deleted: fall back to built-in default persona. Emit warning
event: `Warning ScheduleNotFound`.

If the `PersonaProfile` cannot be found: fall back to built-in default persona. Emit warning
event: `Warning PersonaProfileNotFound`.

**2. Batch aggregation**
Findings are grouped by `scheduleRef` + `runTick` annotation (`reportRunID`). The reconciler
waits for a configurable aggregation window (default 60s) after the first `Detected` finding
for a run before dispatching the report agent Job.

If an audit run produces more findings than `PersonaProfile.spec.maxFindingsPerReport`
(default 100), the excess findings are dispatched in a second agent Job with a `runID`
suffix of `-part2`, `-part3`, etc.

**3. Report agent dispatch**
Create `mendabot-agent` Job with `PROMPT_VARIANT=proactive`. Write findings and persona
to a ConfigMap mounted as a volume. All included findings transition: `Detected` → `Reported`
at the moment the Job is **created** (not when it completes). If the Job fails, findings
revert to `Detected`.

ConfigMap naming: `mendabot-audit-<runID-hash[:12]>`. The ConfigMap has an `ownerReference`
pointing to the agent Job (GC on Job deletion). No explicit ConfigMap cleanup needed.

ConfigMap size limit: Kubernetes etcd limit is ~1.5 MiB. Enforced by:
- `AuditFinding.spec.details` capped at 4,096 bytes (enforced by evaluator)
- Batch size capped at `maxFindingsPerReport` (default 100)
- Maximum estimated payload: 100 × ~4 KiB + metadata ≈ 420 KiB — well within limit

**4. Suppression expiry**
On every reconcile, check `status.suppressedUntil` for findings in `Suppressed` phase.
When `time.Now()` exceeds `suppressedUntil`, transition to `Detected` and trigger normal
dispatch (aggregation window → report batch → optional remediation).

### Responsibilities: remediation.go (RemediationGate)

**5. Auto-remediation decision**
Auto-remediation fires when all three conditions hold simultaneously:
```
spec.remediationAllowed == true
  AND AuditSchedule.spec.autoRemediate == true
  AND check.maturityStage IN (3-remediated, 4-spot-checked, 5-automated)
```

`advisory` and `informational` checks cannot auto-remediate (enforced at bundle load time).

**6. RemediationJob creation**
Create a `RemediationJob` CRD using the typed K8s client:
- `spec.sourceType: "proactive"`
- `spec.finding` from `AuditFinding.Spec.AffectedResource` and `AuditFinding.Spec.Details`
- `spec.remediationHint` from `AuditFinding.Spec.RemediationHint`
- Label: `proactive.mendabot.io/audit-finding-ref: <finding-name>`

Finding transitions: `Reported` → `Dispatched` immediately after `RemediationJob` CRD write.

**7. Outcome mapping from RemediationJob**
Watch `RemediationJob` objects filtered by `proactive.mendabot.io/audit-finding-ref` label.

| `RemediationJob` outcome | `AuditFinding` transition |
|---|---|
| Running | `Dispatched` → `Remediating` |
| Succeeded (stage 3) | `Remediating` → `PendingVerification`; set `status.verificationRequiredBy` |
| Succeeded (stages 4–5) | `Remediating` stays until next evaluator run confirms clean |
| Failed or Cancelled | `Remediating` → `Detected` |

### Agent Job environment variables (proactive prompt)

| Variable / Mount | Content |
|---|---|
| `PROMPT_VARIANT=proactive` | Selects proactive prompt file |
| `/etc/mendabot/audit/findings.json` | JSON array of `AuditFinding` spec objects (ConfigMap mount) |
| `/etc/mendabot/audit/persona.json` | Serialised `PersonaProfile` spec (ConfigMap mount) |
| `AUDIT_SCHEDULE_REF` | Name of the `AuditSchedule` |
| `AUDIT_RUN_ID` | `<scheduleRef>/<runTick>` |
| `AUDIT_SINK_GITHUB_PR_REPO` | `owner/repo`; empty if no GitHub PR sink |
| `AUDIT_SINK_WEBHOOK_URL_COUNT` | Number of webhook sinks |
| `AUDIT_SINK_WEBHOOK_URL_0` ... `_N` | Individual webhook URLs (injected from Secrets by reconciler; never stored in CRDs) |

---

## 10. PersonaProfile CRD

Group: `proactive.mendabot.io/v1alpha1`
Kind: `PersonaProfile`
Short name: `persona`

```yaml
apiVersion: proactive.mendabot.io/v1alpha1
kind: PersonaProfile
metadata:
  name: sre-team
  namespace: mendabot
spec:
  role: sre                         # sre | developer | manager | security | exec
  verbosity: technical              # technical | summary | executive
  priorities:
    - security
    - reliability
    - operations
    - cost
  riskTolerance: low
  suppressSeveritiesBelow: low      # info | low | medium | high | critical
  maxFindingsPerReport: 50
  groupByCategory: true
  customInstructions: |
    Focus on findings that could affect production SLOs. Include kubectl commands.
```

### Role semantics

| Role | Expected output |
|---|---|
| `sre` | Technical depth; kubectl commands; specific field paths; root cause chain |
| `developer` | Application-level framing; manifest changes; no cluster internals jargon |
| `manager` | Risk summary; business impact; effort estimate; no command-line content |
| `security` | CVE references; compliance mapping; blast radius; remediation urgency |
| `exec` | Three sentences maximum per finding; trend indicators; risk vs. cost framing |

### Default fallback persona

Used when referenced `PersonaProfile` cannot be found. Hardcoded in
`internal/proactive/persona/default.go`.

```yaml
role: sre
verbosity: technical
priorities: [security, reliability, operations, cost]
riskTolerance: low
suppressSeveritiesBelow: info
maxFindingsPerReport: 100
groupByCategory: true
customInstructions: ""
```

---

## 11. Proactive Agent Prompt Variant

The `mendabot-agent` image gains a second prompt file. Selected by `PROMPT_VARIANT=proactive`.

### What the proactive prompt instructs the agent to do

1. Read aggregated findings from `/etc/mendabot/audit/findings.json`.
2. Read persona from `/etc/mendabot/audit/persona.json`.
3. Filter and sort findings per persona `priorities` and `suppressSeveritiesBelow`.
4. Produce a report entry per finding at the persona's verbosity and role level.
5. POST structured JSON to each `AUDIT_SINK_WEBHOOK_URL_N` (if count > 0).
6. If `AUDIT_SINK_GITHUB_PR_REPO` is set, open a GitHub PR with a markdown audit report.
7. Exit 0. **Do not modify cluster state. Do not open fix PRs.**

### Hard rules (same as reactive)

- Never commit directly to the default branch.
- Never touch Kubernetes Secrets.
- One report per invocation. Do not open multiple PRs.
- If persona is `exec` or `manager`, do not include raw YAML or kubectl commands.

### GitHub PR report

PR branch: `audit/<schedule-ref>/<runTick-sanitised>`
(e.g., `audit/security-baseline-daily/2026-02-26T02-00-00Z`)

If a PR already exists for the same branch, add a new commit — do not open a duplicate PR.

**Report file:** Full report written as:
```
audit-reports/<runTick-sanitised>-<schedule-ref>.md
```

**PR body:** Executive summary table + top 5 findings + link to full report file. This keeps
the PR body within GitHub's 65,536 character limit while ensuring the full report is
version-controlled.

---

## 12. Sink Extension

### 12.1 Webhook JSON payload

```json
{
  "report_id": "<uuid>",
  "generated_at": "2026-02-26T02:05:00Z",
  "schedule_ref": "security-baseline-daily",
  "persona_role": "sre",
  "summary": {
    "total": 14,
    "critical": 0,
    "high": 3,
    "medium": 8,
    "low": 2,
    "info": 1,
    "auto_remediated": 2,
    "pending_verification": 1
  },
  "findings": [
    {
      "check_id": "no-root-containers",
      "bundle": "cis-k8s-1.8",
      "category": "security",
      "severity": "high",
      "maturity_stage": "3-remediated",
      "frameworks": ["CIS-K8s-1.8:5.2.6"],
      "affected_resource": {
        "kind": "Pod",
        "namespace": "default",
        "name": "web-abc123",
        "parent_object": "Deployment/web"
      },
      "title": "Container running as root",
      "description": "...",
      "recommended_action": "...",
      "remediation_hint": "...",
      "auto_remediated": false,
      "remediation_job_ref": null,
      "suppressed": false,
      "finding_ref": "afinding-abc123def456"
    }
  ]
}
```

### 12.2 Kubernetes Events (side-channel)

For each `AuditFinding` that transitions to `Detected`, the `AuditFindingReconciler` emits
a Kubernetes `Warning` event on the affected resource with reason `AuditFindingDetected`.
This allows observability stacks (Loki, Datadog, etc.) to receive proactive findings
without additional webhook configuration. Side-channel only — not the primary delivery.

### 12.3 Webhook URL injection

The `AuditFindingReconciler` reads each `spec.sinks[].webhookRef` (a `SecretKeyRef`),
fetches the value from the referenced Kubernetes Secret, and injects it directly into the
agent Job env var `AUDIT_SINK_WEBHOOK_URL_<N>`. The URL is never written to any CRD,
log, or ConfigMap.

---

## 13. Auto-Remediation Integration

### Decision logic

```
check.remediationAllowed == true
  AND AuditSchedule.spec.autoRemediate == true
  AND check.maturityStage IN (3-remediated, 4-spot-checked, 5-automated)
```

### RemediationJob creation

The `AuditFindingReconciler` (remediation.go) creates a `RemediationJob` CRD via the typed
K8s client. The existing `RemediationJobReconciler` in `mendabot-watcher` drives the
remediation lifecycle without modification. It treats all `RemediationJob` objects identically
regardless of `sourceType`.

### Clean resolution (stages 4–5)

For stages 4–5, `PendingVerification` is not used. The finding stays in `Remediating`
after `RemediationJob` succeeds. On the next evaluator run, if the check passes for the
affected resource, the evaluator patches the `AuditFinding` phase to `Resolved`.

The `AuditFindingReconciler` does not auto-resolve findings — it only responds to evaluator
writes and human patches.

---

## 14. Failure Modes

Specific to the proactive controller. See `HLD.md §9` for the full cross-binary table.

| Failure | Behaviour |
|---|---|
| Evaluator Job not created (proactive controller API error) | `AuditController` returns error; controller-runtime requeues with backoff |
| Aggregation window expires with zero new findings | No agent Job dispatched; `AuditSchedule.Status.LastRunFindings = 0` |
| Agent Job fails (`backoffLimit` exhausted) | All `Reported` findings for the batch revert to `Detected`; included in next run |
| ConfigMap write fails before agent Job creation | `AuditFindingReconciler` returns error; requeues; retries ConfigMap write |
| Partial `RemediationJob` creation failure | Reconciler requeues; idempotent — already-created RJs are detected by fingerprint check and skipped |
| `AuditSchedule` deleted while findings are open | Dangling `scheduleRef` findings use default persona; emit `Warning ScheduleNotFound` |
| `CheckBundle` deprecated while findings are open | Existing open findings annotated `check-deprecated: true`; no new findings emitted; findings remain for human review |
