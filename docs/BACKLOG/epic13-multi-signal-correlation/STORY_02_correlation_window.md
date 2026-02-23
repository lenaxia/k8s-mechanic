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
      `Evaluate(ctx, candidate, peers, client) ([]CorrelationGroup, error)`
- [ ] When `Correlator.Evaluate` returns a match:
  - Primary `RemediationJob` proceeds to dispatch (phase → `Dispatched`)
  - Non-primary jobs transition to `Suppressed` phase with `CorrelationGroupID` set on status
  - `mendabot.io/correlation-group-id` and `mendabot.io/correlation-role` labels are
    patched onto all jobs in the group
- [ ] When `DISABLE_CORRELATION=true`:
  - Window hold is skipped entirely
  - No correlator is called
  - Existing dispatch behaviour is unchanged
- [ ] `internal/controller/remediationjob_controller_test.go` covers:
  - Window hold: job created, reconcile returns `RequeueAfter`, job still `Pending`
  - Window elapsed: correlator returns no match, job dispatched normally
  - Window elapsed: correlator matches two jobs, primary dispatched, secondary suppressed
  - `DISABLE_CORRELATION=true`: job dispatched immediately without hold
- [ ] `go test -timeout 30s -race ./internal/controller/...` passes

---

## Technical Implementation

### Window hold logic

```go
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... fetch rjob ...

    if rjob.Status.Phase != v1alpha1.PhasePending {
        // handle non-pending phases as before
    }

    if !r.Config.DisableCorrelation {
        window := time.Duration(r.Config.CorrelationWindowSeconds) * time.Second
        age := time.Since(rjob.CreationTimestamp.Time)
        if age < window {
            return ctrl.Result{RequeueAfter: window - age}, nil
        }

        // window elapsed — run correlator
        result, err := r.runCorrelation(ctx, rjob)
        if err != nil {
            return ctrl.Result{}, err
        }
        if result.Suppressed {
            return ctrl.Result{}, r.transitionSuppressed(ctx, rjob, result.GroupID)
        }
        // fall through to dispatch with result.CorrelatedFindings
    }

    return ctrl.Result{}, r.dispatch(ctx, rjob, correlatedFindings)
}
```

### `Correlator` struct

```go
// internal/correlator/correlator.go

type CorrelationGroup struct {
    GroupID            string
    PrimaryUID         types.UID
    CorrelatedUIDs     []types.UID
    Rule               string
    AllFindings        []v1alpha1.FindingSpec
}

type Correlator struct {
    Rules []domain.CorrelationRule
}

func (c *Correlator) Evaluate(
    ctx context.Context,
    candidate *v1alpha1.RemediationJob,
    peers []*v1alpha1.RemediationJob,
    cl client.Client,
) (CorrelationGroup, bool, error)
```

The correlator iterates `c.Rules` in order, returning on the first match. If no rule
matches, it returns `(CorrelationGroup{}, false, nil)`.

### `transitionSuppressed`

Patches the `RemediationJob` status phase to `Suppressed`, sets
`status.correlationGroupID`, and patches the labels. Uses `client.Patch` with
`client.MergeFrom` to avoid overwriting other status fields.

### Wiring the Correlator into the reconciler

The `RemediationJobReconciler` struct gains a `Correlator *correlator.Correlator` field.
The correlator is constructed in `cmd/watcher/main.go` with the three built-in rules and
injected into the reconciler at setup time.

---

## Tasks

- [ ] Write reconciler tests for window hold and correlation paths (TDD — must fail first)
- [ ] Write `internal/correlator/correlator.go` with `Correlator.Evaluate`
- [ ] Write `internal/correlator/correlator_test.go`
- [ ] Update `RemediationJobReconciler` with window hold and correlator call
- [ ] Implement `transitionSuppressed` helper
- [ ] Wire `Correlator` construction in `cmd/watcher/main.go`
- [ ] Add `CorrelationWindowSeconds int`, `DisableCorrelation bool` to `config.Config`
      and `config.FromEnv()`
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
