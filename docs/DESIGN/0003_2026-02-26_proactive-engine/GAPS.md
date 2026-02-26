# Design Gaps — Proactive Engine HLD

**Date:** 2026-02-26
**Status:** Open
**Relates to:** HLD.md, HLD-proactive-controller.md, HLD-check-authoring.md, HLD-evaluator.md

These are identified gaps, unsafe assumptions, and arbitrary limits in the proactive engine
HLD documents that must be resolved before or during LLD authoring. They are grouped by
priority.

---

## Priority: Fix Before LLD

These two issues will cause silent, hard-to-diagnose failures if carried into implementation.

---

### GAP-01 — Reconciler `RequeueAfter` not specified (aggregation window + suppression expiry)

**Affects:** `HLD-proactive-controller.md §9`

**Problem:**

The `AuditFindingReconciler` is described as "waiting" for two time-based conditions:

1. **Aggregation window (60s):** After the first `Detected` finding for a run tick, the
   reconciler waits 60 seconds before dispatching the report agent Job.
2. **Suppression expiry:** When `status.suppressedUntil` passes, the finding transitions
   back to `Detected`.

A controller-runtime reconciler cannot block or sleep. It must return immediately and be
re-enqueued. Neither of these behaviours is achievable without `ctrl.Result{RequeueAfter: d}`.

Without `RequeueAfter`:
- The aggregation window is either bypassed (reconciler dispatches immediately) or never
  fires (reconciler returns without dispatching and is never woken up).
- Suppression expiry never fires. A `Suppressed` finding with a future `suppressedUntil`
  receives no K8s event when the timestamp passes and is never re-processed.

**Required fix:**

The reconciler contract must be updated to explicitly state:

- On first `Detected` finding for a new run tick: return
  `ctrl.Result{RequeueAfter: aggregationWindow}`. On re-reconcile, check whether the window
  has elapsed before dispatching.
- On reconciling a `Suppressed` finding: if `suppressedUntil` is in the future, return
  `ctrl.Result{RequeueAfter: time.Until(suppressedUntil)}`.

The HLD must be explicit about this because `RequeueAfter` is part of the reconciler's
observable contract (it affects when the report agent Job is created) and is easy to omit
in implementation.

**Documents to update:** `HLD-proactive-controller.md §9` (both findings.go and suppression
expiry sections).

---

### GAP-02 — Human approval `kubectl patch` command is wrong for status subresource CRDs

**Affects:** `HLD-check-authoring.md §4`

**Problem:**

The human approval gate instructs operators to run:

```bash
kubectl patch checkbundle my-org-security --type=merge \
  -p '{"status":{"pendingPromotion":{"approved":true}}}'
```

Standard Kubernetes CRD practice is to declare `subresources: status` in the CRD definition.
When a CRD uses the status subresource, the main object endpoint ignores writes to the
`status` field — they are silently dropped. The correct command in that case is:

```bash
kubectl patch checkbundle my-org-security --subresource=status --type=merge \
  -p '{"status":{"pendingPromotion":{"approved":true}}}'
```

The human approval gate is the only mandatory human step in the entire auto-remediation
safety chain (stage 2→3 promotion). If the command silently does nothing, operators will
believe they have approved a promotion when they have not. This is the highest-consequence
place in the system to have a wrong command.

**Required fix:**

Make an explicit decision:

- **Option A (recommended):** `CheckBundle` uses the status subresource (standard practice).
  Update the approval command to use `--subresource=status`. Update all kubectl examples
  in §4.
- **Option B:** `CheckBundle` intentionally does not use the status subresource. Document
  this as an explicit deviation from standard practice and the reason for it.

Whichever option is chosen, the CRD schema in `HLD-proactive-controller.md §3` and the
approval commands in `HLD-check-authoring.md §4` must be consistent.

**Documents to update:** `HLD-check-authoring.md §4`, `HLD-proactive-controller.md §3`.

---

## Priority: Address in LLD

These are implementation traps that do not require HLD changes but must be explicitly
handled in LLD documents or noted as implementation requirements.

---

### GAP-03 — CEL type-check does not guarantee nil-safety at runtime

**Affects:** `HLD-evaluator.md §3.1, §3.3`

**Problem:**

The design states that CEL expressions are validated via parse + type-check at bundle load
time. Type-check validates types but not nil-safety. An expression like:

```
container.securityContext.runAsNonRoot == true
```

passes type-check but panics at runtime on any Pod where `securityContext` is nil — which
is the majority of Pods on a default cluster. The `recover()` in the evaluation loop catches
the panic and skips the check, producing **zero findings with no visible error to the
operator**.

A broken check and a clean cluster are indistinguishable from the `AuditFinding` perspective.
The check-review agent's 14-day stability window means weeks of silent non-detection before
the broken check is flagged.

**Required fix (LLD):**

- The `POST /api/v1/checks/validate` endpoint must warn (not block) when a CEL expression
  accesses a field without a preceding `has()` guard. This can be implemented as a static
  analysis pass over the CEL AST — walk the AST and flag field dereferences that are not
  guarded by `has()`.
- The check authoring guide (in LLD or check authoring docs) must specify that all field
  accesses on optional Kubernetes fields require `has()` guards.
- The `CheckRegistryController` validation step should surface this as a `ValidationWarning`
  on the `CheckBundle` status (non-blocking but visible).

---

### GAP-04 — Cache sync must complete before evaluation begins

**Affects:** `HLD-evaluator.md §2, §4`

**Problem:**

The evaluator uses a controller-runtime cached client. The cache must be fully synced before
evaluation begins — otherwise `client.List` returns stale or empty results, producing silent
false-negatives with no observable error.

The design does not specify that `WaitForCacheSync()` is called and its return value checked
before the evaluation loop starts.

**Required fix (LLD):**

The evaluator `cmd/evaluator/main.go` startup sequence must explicitly:

1. Start the cache (`mgr.Start()` or equivalent).
2. Call `mgr.GetCache().WaitForCacheSync(ctx)` and treat `false` as a fatal error.
3. Only then begin evaluation.

This must be stated in the LLD for `mendabot-evaluator`. It is a one-line addition but
is the kind of correctness requirement that gets omitted from implementation without
explicit specification.

---

### GAP-05 — Concurrent evaluator Jobs for the same AuditSchedule not prevented

**Affects:** `HLD-proactive-controller.md §7`, `HLD-evaluator.md §7`

**Problem:**

An `AuditSchedule` can have both a cron trigger and event triggers. If an event fires
shortly before a cron tick, two evaluator Jobs for the same schedule can run concurrently.

Consequences:
- Both Jobs independently count toward `MAX_FINDINGS_PER_RUN`, effectively doubling the cap.
- Both Jobs write to `AuditSchedule.Status.LastRunFindings` — the second write overwrites
  the first, producing an inaccurate finding count.
- Finding deduplication (fingerprint check) handles concurrent creates correctly
  (`AlreadyExists` → `lastDetectedAt` patch). This part is safe.

**Required fix (LLD):**

The `AuditController` reconciler must check for an existing running evaluator Job for the
same `AuditSchedule` before creating a new one. If `AuditSchedule.Status.EvaluatorJobRef`
points to a Job that is still running (`status.active > 0`), skip Job creation and requeue.

---

### GAP-06 — Concurrent evaluator and reconciler status patches on AuditFinding

**Affects:** `HLD-evaluator.md §8`, `HLD-proactive-controller.md §9`

**Problem:**

Two separate binaries write to `AuditFinding.status`:
- The `mendabot-evaluator` Job patches `phase`, `lastDetectedAt` (regression detection).
- The `AuditFindingReconciler` in `mendabot-proactive` patches `phase`, `reportedAt`,
  `remediationJobRef`, etc.

There is no described conflict resolution strategy. A specific scenario: the reconciler
transitions a finding to `Reported` at the same moment the evaluator patches it back to
`Detected` (regression). One write wins; the other is silently dropped. The dropped write
leaves the finding in an inconsistent state.

**Required fix (LLD):**

The LLD must state that all `AuditFinding` status patches use optimistic locking:
- Always use `client.Status().Patch()` with `client.MergeFrom(existing.DeepCopy())`.
- On `Conflict` error (HTTP 409), re-fetch the object and retry the patch.
- The evaluator's regression detection patch must re-check the current phase after re-fetch:
  if the phase is already `Detected` (reconciler won the race), no action needed.

This is standard controller-runtime practice but must be explicit in the LLD given that
two binaries own writes to the same status field.

---

### GAP-07 — PATCH of CEL expression does not enforce prior validation call

**Affects:** `HLD-check-authoring.md §3.3`

**Problem:**

`PATCH /api/v1/checks/{bundleName}/{checkID}` runs validation on the new CEL expression
before writing. However, the API does not prevent a caller from directly PATCHing the
`CheckBundle` CRD via `kubectl` or the K8s API, bypassing the validation endpoint entirely.

More concretely within the API: the design states the LLM "should" call `POST /validate`
before `PATCH`, but the `PATCH` endpoint runs validation itself (§3.3: "Runs validation on
the new CEL expression. Returns HTTP 422 on failure."). This is actually fine — the PATCH
endpoint is self-validating. The concern is direct CRD writes bypassing the API entirely.

**Required fix (LLD):**

The `CheckRegistryController` must re-validate `api`-origin `CheckBundle` CRDs on any
update to the `spec.checks` field, not only on initial load. If validation fails after a
direct CRD write, set `status.phase: ValidationFailed`. This closes the bypass path.

This is already implied by the controller's responsibilities in `HLD-proactive-controller.md
§5` ("Validate every CEL expression") but should be stated explicitly as covering update
events, not only create events.

---

### GAP-08 — Finding severity filtering should be deterministic, not LLM-driven

**Affects:** `HLD-proactive-controller.md §9, §11`

**Problem:**

The design writes all findings (up to `maxFindingsPerReport`) into the ConfigMap regardless
of `PersonaProfile.spec.suppressSeveritiesBelow`, and instructs the proactive agent to
filter them per persona. LLM filtering is non-deterministic — the agent may surface a `low`
severity finding despite `suppressSeveritiesBelow: medium`.

For a `security` persona with `suppressSeveritiesBelow: high`, this means the agent may
include noise that the operator explicitly opted out of.

**Required fix (LLD):**

The `AuditFindingReconciler` must filter findings by `suppressSeveritiesBelow` before
writing the ConfigMap. Only findings at or above the threshold are included. The agent
receives only findings it is authorised to report. The agent prompt can still re-sort and
group them, but the filter is deterministic.

This is a one-line filter in the reconciler before ConfigMap construction.

---

### GAP-09 — Pre-registered event-trigger watches are a silent coupling to the scope table

**Affects:** `HLD-proactive-controller.md §6, §7`

**Problem:**

The `AuditController` pre-registers watches for all event-trigger-capable scope types at
`SetupWithManager` time. If a new scope type is added to the scope table (e.g., `ingress`,
`hpa`) without a corresponding pre-registered watch, event triggers for that scope silently
never fire. There is no compile-time or test-time enforcement of this coupling.

**Required fix (LLD):**

The scope→watch registration table must be covered by a unit test that asserts every entry
in the canonical scope table (the table in `HLD-proactive-controller.md §6`) has a
corresponding watch registered. This is a table-driven test that breaks at compile time if
the tables diverge.

---

### GAP-10 — AuditFinding name collision on 12-character fingerprint prefix

**Affects:** `HLD-evaluator.md §6`

**Problem:**

`AuditFinding` objects are named `afinding-<fingerprint[:12]>`. The deduplication check
uses the full 64-character fingerprint stored in an annotation for collision detection.
However, the Kubernetes object name uses only 12 hex characters (6 bytes, 2^48 space).

If two logically distinct violations produce the same 12-char prefix, the second
`client.Create` returns `AlreadyExists`. The evaluator's error handling for `AlreadyExists`
treats this as a duplicate and patches `lastDetectedAt` — silently dropping the second
violation. The full fingerprint comparison in the dedup loop would identify the collision,
but the code path after falling through to `client.Create` for a genuinely new finding
still receives `AlreadyExists` for the wrong reason.

This is astronomically unlikely in practice but represents a logical inconsistency in the
design: the name space (12 chars) and the dedup space (64 chars) are different.

**Required fix (LLD):**

Use 16 hex characters (8 bytes, 2^64 space) for the object name suffix. This makes the
name space match standard collision probability expectations. Alternatively, document
explicitly that the 12-char name is intentionally lossy and the full fingerprint annotation
is the authoritative identity — and add a code comment explaining the `AlreadyExists`
path for a genuinely novel finding with a prefix collision.

---

## Informational — Low Priority

These do not require HLD or LLD changes but should be noted for check authors and operators.

---

### GAP-11 — No latency analysis for evaluator Job timeout

`activeDeadlineSeconds: 600` (10 minutes) has no supporting analysis. The stated scale
targets (1,000 pods, 100 nodes, 500 RBAC bindings, 78 checks) combined with sequential
evaluation may approach this limit on large clusters. Should be validated empirically during
epic 16 implementation and the timeout made configurable via a Helm value.

---

### GAP-12 — `MAX_FINDINGS_PER_RUN: 500` may be too low for realistic CIS benchmark runs

On a cluster with 1,000 pods and 47 CIS checks, a 2% failure rate across all checks
produces ~940 findings — nearly double the default cap. The cap silently truncates the run.
The warning is written to `AuditSchedule.Status.LastRunWarning` but is not surfaced as a
Prometheus metric or K8s Event. Consider adding a metric counter `mendabot_evaluator_cap_hit_total`
to make cap hits visible to observability stacks.

---

### GAP-13 — `stabilisationSeconds` guidance absent from check authoring docs

No guidance exists on when non-zero `stabilisationSeconds` is appropriate or what values
are reasonable. Rolling deployments, node drains, and CronJob schedules all create transient
states that look like violations. The check authoring guidelines (future LLD) should include
a table of recommended stabilisation values per scope type.

---

### GAP-14 — `pendingPromotion` has no expiry in v1

Pending promotions that are never approved or rejected accumulate silently. There is no
`kubectl get checkbundle` column showing pending promotion age. Consider adding a
`promotionPendingSince` duration to the `kubectl` printer columns in the CRD definition
so operators can see stale approvals at a glance without inspecting the full object.

---

## Status tracking

| Gap | Priority | Status | Assigned |
|-----|----------|--------|----------|
| GAP-01 | Fix before LLD | Open | — |
| GAP-02 | Fix before LLD | Open | — |
| GAP-03 | Address in LLD | Open | — |
| GAP-04 | Address in LLD | Open | — |
| GAP-05 | Address in LLD | Open | — |
| GAP-06 | Address in LLD | Open | — |
| GAP-07 | Address in LLD | Open | — |
| GAP-08 | Address in LLD | Open | — |
| GAP-09 | Address in LLD | Open | — |
| GAP-10 | Address in LLD | Open | — |
| GAP-11 | Informational | Open | — |
| GAP-12 | Informational | Open | — |
| GAP-13 | Informational | Open | — |
| GAP-14 | Informational | Open | — |
