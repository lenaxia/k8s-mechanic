# Story 02: CorrelationWindow in RemediationJobReconciler

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 4 hours

---

## User Story

As a **mendabot operator**, I want the `RemediationJobReconciler` to hold newly-created
`RemediationJob` objects in `Pending` phase for a configurable window before dispatching,
so that correlated findings created within the same window are grouped rather than
dispatched independently.

---

## Background

This is the most significant change in the epic. The reconciler today transitions a
`Pending` `RemediationJob` to `Dispatched` as soon as it sees it. After this story, it
holds the object for `CORRELATION_WINDOW_SECONDS` after creation, then runs the
correlator before deciding whether to dispatch or suppress.

The hold is implemented with `ctrl.Result{RequeueAfter: remaining}`, not a goroutine
sleep. This preserves idempotency and survives watcher restarts without any additional
durable state.

---

## Acceptance Criteria

- [ ] `RemediationJobReconciler` holds any new `Pending` job for `CORRELATION_WINDOW_SECONDS`
      (default: 30) using `ctrl.Result{RequeueAfter: remaining}` before proceeding
- [ ] After the window, the reconciler lists all `Pending` `RemediationJob` objects in the
      same namespace and passes them to `Correlator.Evaluate`
- [ ] `Correlator` struct exists in `internal/correlator/correlator.go` with method
      `Evaluate(ctx, candidate, peers, client) (CorrelationGroup, bool, error)`
      (the `bool` is `true` when a match was found; idiomatic Go "found" return)
- [ ] When `Correlator.Evaluate` returns `found=true`:
  - Primary `RemediationJob` proceeds to dispatch (phase → `Dispatched`)
  - Non-primary jobs transition to `Suppressed` phase with `CorrelationGroupID` set on status
  - `mendabot.io/correlation-group-id` and `mendabot.io/correlation-role` labels are
    patched onto all jobs in the group
- [ ] The `switch` statement in `Reconcile()` has an explicit `case v1alpha1.PhaseSuppressed`
      that returns `ctrl.Result{}, nil` immediately, preventing suppressed jobs from
      ever being re-dispatched on subsequent reconcile events
- [ ] When `r.Correlator == nil` (set when `DISABLE_CORRELATION=true` in main.go):
  - Window hold is skipped entirely
  - No correlator is called
  - Existing dispatch behaviour is unchanged
- [ ] `internal/controller/remediationjob_controller_test.go` covers:
  - Window hold: job created, reconcile returns `RequeueAfter`, job still `Pending`
  - Window elapsed: correlator returns no match, job dispatched normally
  - Window elapsed: correlator matches two jobs, primary dispatched, secondary suppressed
  - `r.Correlator == nil`: job dispatched immediately without hold
- [ ] `go test -timeout 30s -race ./internal/controller/...` passes

---

## Technical Implementation

### `Suppressed` phase in the reconciler switch (Gap 6 fix)

The `switch` in `Reconcile()` at `remediationjob_controller.go:51` currently handles
`PhaseSucceeded`, `PhaseFailed`, and `PhaseCancelled`. Any unmatched phase falls through
to step 3 (list owned jobs) and then to job creation. A `Suppressed` job must be handled
as a terminal case:

```go
case v1alpha1.PhaseSuppressed:
    return ctrl.Result{}, nil
```

Add this case immediately after `PhaseCancelled` at line 69. Without it, every requeue
event for a `Suppressed` job will dispatch a new `batch/v1 Job`, defeating suppression.

### Window hold logic

The window hold is inserted in the `Pending` path, **after** the phase switch and **before**
step 3. The `Pending` phase check at `remediationjob_controller.go:147` sets `Pending`
when `MAX_CONCURRENT_JOBS` is reached. The correlation window is a separate concern that
gates all `Pending` jobs regardless of the concurrency check:

```go
// After the phase switch (line 71), before step 3 (line 73):
if r.Correlator != nil {
    window := time.Duration(r.Cfg.CorrelationWindowSeconds) * time.Second
    age := time.Since(rjob.CreationTimestamp.Time)
    if age < window {
        return ctrl.Result{RequeueAfter: window - age}, nil
    }
    group, found, err := r.Correlator.Evaluate(ctx, &rjob, r.pendingPeers(ctx, &rjob), r.Client)
    if err != nil {
        return ctrl.Result{}, err
    }
    if found {
        isPrimary := group.PrimaryUID == rjob.UID
        if !isPrimary {
            return ctrl.Result{}, r.transitionSuppressed(ctx, &rjob, group.GroupID)
        }
        // Primary: fall through to dispatch, passing the correlated findings
        return ctrl.Result{}, r.dispatch(ctx, &rjob, group.AllFindings)
    }
}
// No correlation (or correlator disabled): dispatch immediately with no correlated findings
return ctrl.Result{}, r.dispatch(ctx, &rjob, nil)
```

`pendingPeers` is a private helper that lists all `Pending` `RemediationJob` objects in
`r.Cfg.AgentNamespace` (excluding the candidate itself):

```go
func (r *RemediationJobReconciler) pendingPeers(ctx context.Context, candidate *v1alpha1.RemediationJob) []*v1alpha1.RemediationJob {
    var list v1alpha1.RemediationJobList
    if err := r.List(ctx, &list, client.InNamespace(r.Cfg.AgentNamespace)); err != nil {
        return nil
    }
    var peers []*v1alpha1.RemediationJob
    for i := range list.Items {
        p := &list.Items[i]
        if p.UID == candidate.UID || p.Status.Phase != v1alpha1.PhasePending {
            continue
        }
        peers = append(peers, p)
    }
    return peers
}
```

### `Correlator` struct and `Evaluate` signature

```go
// internal/correlator/correlator.go

type CorrelationGroup struct {
    GroupID        string
    PrimaryUID     types.UID
    CorrelatedUIDs []types.UID
    Rule           string
    // AllFindings collects rjob.Spec.Finding from the primary and all correlated peers.
    // Populated by the Correlator after a rule match; passed to JobBuilder.Build().
    AllFindings    []v1alpha1.FindingSpec
}

type Correlator struct {
    Rules []domain.CorrelationRule
}

// Evaluate applies rules in order, returning the first match.
// Returns (CorrelationGroup{}, false, nil) when no rule matches.
func (c *Correlator) Evaluate(
    ctx context.Context,
    candidate *v1alpha1.RemediationJob,
    peers []*v1alpha1.RemediationJob,
    cl client.Client,
) (CorrelationGroup, bool, error)
```

The correlator iterates `c.Rules` in order. On the first match it assembles
`CorrelationGroup.AllFindings` by collecting `rjob.Spec.Finding` from the candidate and
all matched peers, then returns `(group, true, nil)`. If no rule matches, it returns
`(CorrelationGroup{}, false, nil)`.

**AllFindings population (Gap 14 fix):** This must happen inside `Correlator.Evaluate`,
not in the reconciler. After a rule returns `CorrelationResult{Matched: true}`, the
correlator resolves the primary job and the list of matched peers, then:

```go
group.AllFindings = make([]v1alpha1.FindingSpec, 0, len(matchedPeers)+1)
group.AllFindings = append(group.AllFindings, candidate.Spec.Finding)
for _, p := range matchedPeers {
    group.AllFindings = append(group.AllFindings, p.Spec.Finding)
}
```

### `transitionSuppressed`

Patches the `RemediationJob` status phase to `Suppressed`, sets
`status.correlationGroupID`, and patches the correlation labels onto the object metadata.
Uses two separate patches: one for status (`r.Status().Patch`) and one for labels
(`r.Patch`) to avoid overwriting other status fields.

### Wiring the `Correlator` in `cmd/watcher/main.go` (Gap 12 fix)

The `Correlator` is an optional field on the reconciler. When `cfg.DisableCorrelation`
is true, the field is left nil and the reconciler's `r.Correlator != nil` guard skips all
correlation logic:

```go
// cmd/watcher/main.go — construct Correlator conditionally
var corr *correlator.Correlator
if !cfg.DisableCorrelation {
    corr = &correlator.Correlator{
        Rules: []domain.CorrelationRule{
            correlator.SameNamespaceParentRule{},
            correlator.PVCPodRule{},
            correlator.MultiPodSameNodeRule{Threshold: cfg.MultiPodThreshold},
        },
    }
}

if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg:        cfg,
    Correlator: corr,  // nil when DisableCorrelation=true
}).SetupWithManager(mgr); err != nil {
    ...
}
```

This requires adding `import "github.com/lenaxia/k8s-mendabot/internal/correlator"` to
`cmd/watcher/main.go`.

The escape hatch check in the reconciler is `if r.Correlator != nil` — do not use
`r.Cfg.DisableCorrelation` inside the reconciler. The nil check is the single source of
truth and avoids the reconciler needing to know about the config field name.

---

## Tasks

- [ ] Write reconciler tests for window hold and correlation paths (TDD — must fail first)
- [ ] Add `case v1alpha1.PhaseSuppressed: return ctrl.Result{}, nil` to the `switch` in
      `remediationjob_controller.go:51`, immediately after the `PhaseCancelled` case (line 69)
- [ ] Write `internal/correlator/correlator.go` with `Correlator.Evaluate` and `CorrelationGroup`
      (including `AllFindings []v1alpha1.FindingSpec` populated from matched peer `Spec.Finding` fields)
- [ ] Write `internal/correlator/correlator_test.go`
- [ ] Add `Correlator *correlator.Correlator` field to `RemediationJobReconciler` struct
- [ ] Update `Reconcile()` with window hold, `pendingPeers` helper, and correlator call
      (placed after the phase switch, before step 3 — use `r.Correlator != nil` guard)
- [ ] Implement `transitionSuppressed` helper (status patch + label patch)
- [ ] Add conditional `Correlator` construction in `cmd/watcher/main.go` with
      `if !cfg.DisableCorrelation { ... }` block
- [ ] Add `CorrelationWindowSeconds int`, `DisableCorrelation bool`, and `MultiPodThreshold int`
      (default: 3) to `config.Config` and `config.FromEnv()` in `internal/config/config.go`
- [ ] Run `go test -timeout 30s -race ./...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (domain types), STORY_01 (built-in rules)
**Blocks:** STORY_05 (integration tests validate this story end-to-end)

---

## Definition of Done

- [ ] Reconciler holds `Pending` jobs for the window duration
- [ ] Correlated jobs get `Suppressed` phase with correct labels
- [ ] `DISABLE_CORRELATION=true` restores original behaviour
- [ ] All tests pass
