# High-Level Design — Evaluator Binary (`mendabot-evaluator`)

**Version:** 1.0
**Date:** 2026-02-26
**Status:** Proposed
**Parent:** [`HLD.md`](HLD.md)

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-26 | Initial: evaluator binary, CEL engine, RBAC, redaction, AuditFinding write path | LLM / Human |

---

## Table of Contents

1. [Binary Overview](#1-binary-overview)
2. [Job Lifecycle](#2-job-lifecycle)
3. [CEL Evaluation Engine](#3-cel-evaluation-engine)
4. [Cluster Enumeration](#4-cluster-enumeration)
5. [Secret Redaction](#5-secret-redaction)
6. [AuditFinding Write Path](#6-auditfinding-write-path)
7. [Max-Findings Cap](#7-max-findings-cap)
8. [Regression Detection](#8-regression-detection)
9. [RBAC and Security](#9-rbac-and-security)
10. [Failure Modes](#10-failure-modes)

---

## 1. Binary Overview

`mendabot-evaluator` is a **stateless, short-lived binary** that runs as a Kubernetes Job.
It is spawned by `mendabot-proactive` in response to a cron tick or event trigger on an
`AuditSchedule`.

The evaluator's sole responsibility is:
1. Read `CheckBundle` CRDs to obtain check definitions.
2. Enumerate cluster resources matching the required CEL scopes.
3. Evaluate CEL expressions against those resources.
4. Redact sensitive field values from violation details.
5. Write `AuditFinding` CRDs for confirmed violations.
6. Exit.

The evaluator does **not**:
- Run a controller-runtime reconciler loop
- Have an HTTP server
- Make LLM calls
- Read or write any resource other than `CheckBundle` (read) and `AuditFinding` (read/write)
  and `AuditSchedule` (read for scope config)

Entry point: `cmd/evaluator/main.go`.

---

## 2. Job Lifecycle

### Input: environment variables

The evaluator receives all configuration via environment variables set by `mendabot-proactive`
at Job creation time.

| Variable | Required | Description |
|---|---|---|
| `AUDIT_SCHEDULE_REF` | Yes | Name of the `AuditSchedule` that triggered this run |
| `AUDIT_RUN_TICK` | Yes | RFC3339 timestamp of the cron tick (truncated to minute) |
| `CHECK_BUNDLE_REFS` | Yes | Comma-separated list of `CheckBundle` CRD names |
| `SCOPE_NAMESPACES` | No | Comma-separated namespaces to restrict evaluation; empty = cluster-wide |
| `MAX_FINDINGS_PER_RUN` | No | Hard cap on `AuditFinding` writes; default 500 |
| `TRIGGER_KIND` | No | For event-triggered runs: Kubernetes Kind of the changed resource |
| `TRIGGER_RESOURCE_NAME` | No | For event-triggered runs: resource name |
| `TRIGGER_RESOURCE_NAMESPACE` | No | For event-triggered runs: resource namespace |

### Execution flow

```
1. Parse and validate environment variables
2. Build K8s client from in-cluster ServiceAccount token
3. Load CheckBundle CRDs from CHECK_BUNDLE_REFS
   → skip bundles with status.phase: ValidationFailed or Deprecated
   → filter out checks with maturityStage: 1-manual
   → filter out deprecated individual checks (status.phase field on check entry)
4. Build CEL environments (full for curated; restricted for custom)
5. Determine required scopes from loaded checks
6. Enumerate cluster objects per scope (§4)
   → if TRIGGER_KIND is set: enumerate only the specific triggered resource
7. For each (check, resource) pair:
   a. Evaluate CEL expression
   b. If false (violation): redact sensitive fields (§5), write AuditFinding (§6)
   c. Enforce MAX_FINDINGS_PER_RUN cap (§7)
8. Perform regression detection pass (§8)
9. Patch AuditSchedule.Status.LastRunFindings, LastRunDuration, LastRunWarning
10. Exit 0 (even if some evaluations errored; errors are logged and skipped)
```

### Job spec

See `HLD-proactive-controller.md §7` for the full Job spec written by the `AuditController`.

Key settings:
```
backoffLimit: 1
activeDeadlineSeconds: 600   (10 min hard timeout)
ttlSecondsAfterFinished: 86400
restartPolicy: Never
```

---

## 3. CEL Evaluation Engine

Package: `internal/evaluator/cel/`

### 3.1 Evaluation context per scope

CEL expressions are evaluated against a typed context populated from live cluster state.

| Scope | Context variables |
|---|---|
| `container` | `container` (ContainerSpec), `pod` (PodSpec), `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `pod` | `pod` (PodSpec), `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `deployment` | `deployment` (Deployment), `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `statefulset` | `statefulset` (StatefulSet), `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `job` | `job` (Job), `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `namespace` | `namespace` (Namespace), `labels` (map), `annotations` (map) |
| `node` | `node` (Node), `labels` (map), `annotations` (map) |
| `cluster` | `cluster` (object with `nodeCount int`, `version string`) |
| `rbac` | `binding` (ClusterRoleBinding or RoleBinding), `role` (ClusterRole or Role) |
| `networkpolicy` | `policy` (NetworkPolicy), `namespace` (Namespace) |
| `pvc` | `pvc` (PersistentVolumeClaim), `namespace` (Namespace) |
| `serviceaccount` | `serviceaccount` (ServiceAccount), `namespace` (Namespace) |
| `configmap` | `configmap` (ConfigMap), `namespace` (Namespace) |

`Secret` is explicitly absent from this table. The evaluator ServiceAccount has no `Secret`
RBAC and the CEL environment does not declare a `secret` scope variable. This is a
structural guarantee — not a prompt instruction.

All context variables are read-only. The CEL type-checker enforces this at bundle validation
time (performed by `CheckRegistryController`, not at evaluation time).

### 3.2 CEL environments

Two `cel.Env` instances are constructed at evaluator startup:

**Full environment** (for `trustLevel: curated`):
- All standard CEL functions
- Custom extension functions:
  - `hasLabel(resource, key) bool` — true if label key exists
  - `hasAnnotation(resource, key) bool` — true if annotation key exists
  - `matchesRegex(str, pattern) bool` — regex match
  - `isIn(value, list) bool` — membership test

**Restricted environment** (for `trustLevel: custom`):
- Standard CEL arithmetic, comparison, and logical operators only
- No custom extension functions
- No cross-scope variable access (type checker enforces this because cross-scope variables
  are not declared in the restricted environment)

### 3.3 Validation at load time (performed by CheckRegistryController, not evaluator)

When `CheckRegistryController` pulls a bundle, it validates every CEL expression before
writing the `CheckBundle` CRD:

1. `cel.Env.Parse(expr)` — syntactic correctness
2. `cel.Env.Check(ast)` — type correctness against declared scope variables

`cel.Program.Eval()` is **not** used for validation. Evaluating against a zero-value
context would fail for any expression that dereferences a variable. Parse + Check is the
correct static validation path.

A bundle with any invalid expression is written with `status.phase: ValidationFailed`.
The evaluator skips `ValidationFailed` bundles entirely.

### 3.4 Runtime evaluation

For each (check, resource) pair:

```go
program, err := env.Program(check.CompiledAST)
// CompiledAST is stored in the CheckBundle CRD as a pre-compiled artifact
// If not pre-compiled: compile at evaluator startup from the cel string

out, _, err := program.Eval(celContext)
if err != nil {
    // log and skip; do not emit AuditFinding; do not count against cap
    continue
}

compliant, ok := out.Value().(bool)
if !ok || !compliant {
    // violation: emit AuditFinding
}
```

CEL runtime panics are caught with `recover()`. A panicking check is logged at error level
with check ID and resource, and skipped. Other checks continue. The bundle is not invalidated
by a runtime panic.

### 3.5 Stabilisation (event-triggered runs)

For event-triggered evaluations with `check.stabilisationSeconds > 0`:

1. The evaluator records the violation in an in-memory map `(checkID, resourceFingerprint) → violationTime`.
2. If `time.Since(violationTime) < stabilisationSeconds`: re-evaluate after the remaining
   duration. The evaluator sleeps for the minimum remaining stabilisation time across all
   pending (check, resource) pairs, then re-evaluates only those pairs.
3. If the resource still violates after the stabilisation window: emit `AuditFinding`.
4. If the resource is now compliant during re-evaluation: no `AuditFinding` emitted.

Stabilisation state is in-memory only. If the evaluator Job is killed (e.g.,
`activeDeadlineSeconds` exceeded), stabilisation state is lost. The next evaluator Job
starts fresh. This is acceptable — event-triggered checks with stabilisation are re-evaluated
on the next cron tick.

### 3.6 RBAC scope role resolution

The `rbac` scope requires a secondary lookup: a `ClusterRoleBinding` or `RoleBinding`
references its `Role` or `ClusterRole` by name in `spec.roleRef`, but does not embed the
role's rules inline. The `role` CEL context variable must be populated before evaluation.

**Resolution:** The evaluator calls `client.Get(ctx, types.NamespacedName{Name: binding.RoleRef.Name}, role)`.
The evaluator uses a cached client (controller-runtime cache) — this is a cache read, not
an API server roundtrip.

**Cache miss:** If the referenced role is not found (deleted after the binding was created,
or cache not yet synced), the rbac check is **skipped for that binding only**. Not a fatal
error. Logged at warn level with binding name and missing role name. The binding is
re-evaluated on the next scheduled run.

**Type resolution:** `binding.RoleRef.Kind` is either `"ClusterRole"` or `"Role"`. The
code checks this field and calls `Get` for the appropriate type. A `RoleBinding` referencing
a `ClusterRole` is valid Kubernetes and is handled correctly.

---

## 4. Cluster Enumeration

Package: `internal/evaluator/enumerate/`

The evaluator uses a controller-runtime cached client to enumerate cluster objects per
scope. The cache is populated on startup from the evaluator's in-cluster ServiceAccount.

### Namespace filtering

If `SCOPE_NAMESPACES` is non-empty, the list call is restricted to those namespaces:
```go
client.List(ctx, &podList, client.InNamespace(ns))
```

If `SCOPE_NAMESPACES` is empty, the list is cluster-wide.

### ConfigMap cache filter

`ConfigMap` objects are high-volume in typical clusters. The evaluator's cached client
filters `ConfigMap` watches to exclude system namespaces (`kube-system`, `kube-public`,
`kube-node-lease`) unless those namespaces are explicitly listed in `SCOPE_NAMESPACES`.
This is set in the cache options at startup.

### Event-triggered enumeration

When `TRIGGER_KIND`, `TRIGGER_RESOURCE_NAME`, and `TRIGGER_RESOURCE_NAMESPACE` are set,
the evaluator evaluates only the specific triggered resource rather than the full scope.

For `scope: container` (triggered by a Pod event): the evaluator iterates all containers
within the triggered pod only.

For `scope: rbac` (triggered by a ClusterRoleBinding event): the evaluator evaluates both
`rbac` and any related binding checks for the specific binding, plus its resolved role.

### Object counts and performance

The evaluator is designed to handle clusters with:
- Up to 1,000 pods (3,000+ containers)
- Up to 100 nodes
- Up to 500 RoleBindings/ClusterRoleBindings

For larger clusters, the `MAX_FINDINGS_PER_RUN` cap (§7) ensures the evaluator does not
run indefinitely, but the evaluation itself may be slow. Future versions may add per-scope
parallelism. In v1, evaluation is sequential per scope, scopes are evaluated sequentially.

---

## 5. Secret Redaction

Package: `internal/evaluator/redact/`

Before any resource field value is written to `AuditFinding.spec.details`, the evaluator
runs a redaction pass on the violation detail string produced by `failureMessageTemplate`.

### Redaction rules

1. **Environment variable names:** In any string that contains an environment variable
   representation (e.g., from a Pod's `spec.containers[].env` field rendered in the
   template), replace the value of any env var whose **name** matches the pattern:
   ```
   (SECRET|PASSWORD|PASSWD|TOKEN|KEY|CREDENTIAL|CERT|PRIVATE|AUTH|API_KEY)
   ```
   (case-insensitive, anywhere in the name)

   Pattern: `<NAME>=<value>` → `<NAME>=[REDACTED]`

2. **Common secret patterns:** Replace values matching well-known secret formats with
   `[REDACTED]`:
   - Strings matching `^[A-Za-z0-9+/]{40,}={0,2}$` (base64, length ≥ 40)
   - Strings matching `^[A-Fa-f0-9]{32,}$` (hex, length ≥ 32)
   - Strings matching `^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$` (JWT)
   - Strings matching `^-----BEGIN` (PEM block)

3. **Field length cap:** Any single field value exceeding 512 bytes is truncated with
   `[truncated at 512 bytes]` suffix before being written to `details`.

4. **Total `details` cap:** The full rendered `failureMessageTemplate` output is capped at
   4,096 bytes. If the rendered output exceeds this, it is truncated with a `[truncated]`
   suffix. This cap applies after redaction.

### What redaction does NOT cover

- The CEL expression itself (never written to `AuditFinding`)
- Kubernetes `Secret` object field values (the evaluator SA has no `Secret` RBAC; these
  cannot appear in evaluation context)
- Resource labels and annotations (included in CEL context but not written verbatim to
  `details` — only the `failureMessageTemplate` output is written)

### Redaction log

When a redaction occurs, the evaluator logs at `debug` level:
```
redacted field value in AuditFinding details: check=no-root-containers resource=Pod/default/web-abc123 field=env[DATABASE_PASSWORD]
```

---

## 6. AuditFinding Write Path

Package: `internal/evaluator/writer/`

### Fingerprint computation

```go
fingerprint = sha256(checkID + "\x00" + bundleName + "\x00" + namespace + "\x00" + kind + "\x00" + resourceName)
```

`\x00` null byte separators prevent fingerprint collisions between (e.g.)
`checkID="a"` + `bundleName="bc"` and `checkID="ab"` + `bundleName="c"`.

### Deduplication check

Before writing a new `AuditFinding`, the evaluator checks for an existing non-terminal
finding with the same fingerprint:

```go
var existingList AuditFindingList
client.List(ctx, &existingList, client.MatchingLabels{
    "proactive.mendabot.io/fingerprint": fingerprint[:12],
})
for _, existing := range existingList.Items {
    if existing.Annotations["proactive.mendabot.io/fingerprint-full"] == fingerprint {
        if !isTerminal(existing.Status.Phase) {
            // update lastDetectedAt only; do not create a duplicate
            patch existing.Status.LastDetectedAt = now
            return
        }
    }
}
// no existing non-terminal finding: create a new AuditFinding
```

Terminal phases: `Resolved`, `Suppressed`, `Deleted`.

A finding in `Resolved` phase being re-detected is a regression. The evaluator patches
its phase back to `Detected`. See §8.

### AuditFinding creation

```go
finding := &AuditFinding{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "afinding-" + fingerprint[:12],
        Namespace: "mendabot",
        Labels: map[string]string{
            "proactive.mendabot.io/fingerprint":    fingerprint[:12],
            "proactive.mendabot.io/check-id":       check.ID,
            "proactive.mendabot.io/category":       check.Category,
            "proactive.mendabot.io/severity":       check.Severity,
            "proactive.mendabot.io/maturity-stage": check.MaturityStage,
        },
        Annotations: map[string]string{
            "proactive.mendabot.io/fingerprint-full": fingerprint,
            "proactive.mendabot.io/run-tick":         runTick,
            "proactive.mendabot.io/check-deprecated": "false",
        },
    },
    Spec: AuditFindingSpec{
        CheckID:               check.ID,
        BundleName:            bundle.Name,
        CheckClass:            check.CheckClass,
        Category:              check.Category,
        Severity:              check.Severity,
        Frameworks:            check.Frameworks,
        MaturityStageAtCreation: check.MaturityStage,  // immutable
        AffectedResource: ResourceRef{
            APIVersion:   resource.APIVersion,
            Kind:         resource.Kind,
            Namespace:    resource.Namespace,
            Name:         resource.Name,
            ParentObject: resource.OwnerRef,
        },
        Fingerprint:       fingerprint,
        Details:           redactedDetails,    // capped at 4096 bytes
        RemediationAllowed: check.RemediationAllowed,
        RemediationHint:   check.RemediationHint,
        ScheduleRef:       os.Getenv("AUDIT_SCHEDULE_REF"),
    },
    Status: AuditFindingStatus{
        Phase:           "Detected",
        FirstDetectedAt: metav1.Now(),
        LastDetectedAt:  metav1.Now(),
    },
}
client.Create(ctx, finding)
```

### Error handling

If `client.Create` returns an `AlreadyExists` error (race: two evaluators created the same
finding simultaneously), the evaluator retries with a `client.Get` + status patch for
`LastDetectedAt`. This is safe — the second writer simply updates the timestamp.

If `client.Create` returns any other error, the evaluator logs the error with check ID and
resource, increments an error counter, and continues with the next (check, resource) pair.
Transient API errors do not abort the evaluation run.

---

## 7. Max-Findings Cap

The evaluator enforces `MAX_FINDINGS_PER_RUN` (default 500) as a hard cap on the number
of **new** `AuditFinding` CRDs created in a single run. Updates to existing findings
(`lastDetectedAt` patches) do not count against the cap.

When the cap is reached:
1. Evaluation of remaining (check, resource) pairs stops immediately.
2. The evaluator writes a warning to `AuditSchedule.Status.LastRunWarning`:
   ```
   MAX_FINDINGS_PER_RUN (500) reached after evaluating 23 of 78 checks. 
   Remaining checks were not evaluated. Consider reducing scope or increasing the cap.
   ```
3. The evaluator exits 0. It does not retry or continue.
4. Already-written findings (up to the cap) are complete and valid.

The cap protects etcd from a flood of findings caused by a newly-added check that fires on
every resource in the cluster, or a CEL expression that is unintentionally broad.

---

## 8. Regression Detection

On every evaluator run, the evaluator evaluates all active checks against all in-scope
resources — including resources that currently have `Resolved` `AuditFinding` objects.

If a `Resolved` finding is detected violating again:

```go
if existing.Status.Phase == "Resolved" && violation {
    // regression: patch phase back to Detected
    patch := client.MergeFrom(existing.DeepCopy())
    existing.Status.Phase = "Detected"
    existing.Status.LastDetectedAt = metav1.Now()
    client.Status().Patch(ctx, existing, patch)
}
```

The `AuditFindingReconciler` picks up the phase change and drives the normal dispatch
pipeline (aggregation window → report batch → optional remediation).

The evaluator does **not** check whether `Suppressed` findings have regressed. Suppression
expiry is handled by the `AuditFindingReconciler` checking `suppressedUntil` against
`time.Now()` on each reconcile.

---

## 9. RBAC and Security

### Two ServiceAccounts

`mendabot-evaluator-namespaced` — for schedules requiring only namespaced resources:

| Resource | Verbs |
|---|---|
| `pods`, `deployments`, `statefulsets`, `jobs`, `persistentvolumeclaims` | `get`, `list`, `watch` |
| `serviceaccounts`, `configmaps`, `networkpolicies`, `namespaces` | `get`, `list`, `watch` |
| `auditfindings.proactive.mendabot.io` | `get`, `list`, `create`, `patch` |
| `auditschedules.proactive.mendabot.io` | `get`, `patch` (status only) |
| `checkbundles.proactive.mendabot.io` | `get`, `list` |
| **`secrets`** | **none** |

`mendabot-evaluator-cluster` — for schedules requiring cluster-scoped resources (superset):

All permissions from `mendabot-evaluator-namespaced`, plus:

| Resource | Verbs |
|---|---|
| `nodes` | `get`, `list`, `watch` |
| `clusterrolebindings`, `rolebindings` | `get`, `list`, `watch` |
| `clusterroles`, `roles` | `get`, `list`, `watch` |
| **`secrets`** | **none** |

### ServiceAccount selection by AuditController

```
if any check in CHECK_BUNDLE_REFS has scope in {node, rbac, cluster}:
    serviceAccountName = mendabot-evaluator-cluster
else:
    serviceAccountName = mendabot-evaluator-namespaced
```

This selection is made at Job creation time by `mendabot-proactive`. The evaluator itself
does not select its own ServiceAccount — it uses whichever was assigned to its Pod.

### No write access beyond AuditFinding

The evaluator has `create` and `patch` on `AuditFinding` CRDs only. It has no write access
to any cluster resource. A compromised evaluator binary cannot modify workloads, delete
resources, or create arbitrary objects. The worst-case blast radius of a compromised
evaluator is: read all cluster resources (no Secrets) and write arbitrary `AuditFinding`
CRDs.

### Image provenance

The evaluator image is built from `Dockerfile.evaluator` in the same monorepo as the other
images. It is pushed to `ghcr.io/lenaxia/mendabot-evaluator:<tag>` by the `build-evaluator.yaml`
GitHub Actions workflow. Image signing (cosign) is applied the same as other mendabot images.

The evaluator image contains **only** the Go binary and a minimal base image (distroless or
debian-slim). It does not contain `kubectl`, `gh`, `helm`, `flux`, or `opencode`. These are
not needed and would increase the attack surface.

---

## 10. Failure Modes

| Failure | Behaviour |
|---|---|
| CEL expression runtime panic | `recover()` catches it; check skipped; error logged; other checks continue |
| `client.Create` on AuditFinding returns error | Log error; continue with next check; increment error counter |
| `client.Create` returns AlreadyExists | Retry as `lastDetectedAt` patch; safe concurrent operation |
| RBAC role missing for rbac scope check (cache miss) | Skip that binding; warn log; re-evaluated on next run |
| `activeDeadlineSeconds` exceeded | Job killed; no graceful shutdown; findings written so far are persisted; AuditSchedule.Status not updated (proactive controller detects Job failure and updates status) |
| Pod OOMKilled | Same as above |
| `MAX_FINDINGS_PER_RUN` reached | Write warning to AuditSchedule.Status; exit 0; findings written so far are valid |
| CheckBundle with ValidationFailed phase | Skipped entirely; logged at info level |
| CHECK_BUNDLE_REFS references non-existent bundle | Logged at warn; bundle skipped; other bundles evaluated |
| All CHECK_BUNDLE_REFS fail to load | No evaluations performed; exit 0 with zero findings; AuditSchedule.Status updated |
| Evaluator Job `backoffLimit` exhausted | Job fails permanently; AuditSchedule.Status updated by proactive controller; next cron tick creates a new Job |
| Stabilisation timer: Job killed before re-evaluation | Stabilisation state lost; violation may be re-detected on next cron run |
