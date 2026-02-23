# Story 05: Integration Tests and DISABLE_CORRELATION Escape Hatch

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot developer**, I want end-to-end integration tests that exercise the full
correlation path from `RemediationJob` creation through window hold, correlator
evaluation, and `Suppressed` phase transition, and I want a `DISABLE_CORRELATION=true`
escape hatch that restores the pre-epic dispatch behaviour completely.

---

## Background

Unit tests in STORY_01 and STORY_02 cover individual components. This story adds
integration tests using `envtest` that exercise the full flow: multiple `RemediationJob`
objects created in the same namespace, the window hold, the correlator running, and the
correct phase transitions resulting.

The escape hatch is critical for operators who hit unexpected correlation behaviour in
production and need an immediate rollback without re-deploying a different binary.

---

## Acceptance Criteria

- [ ] `internal/controller/suite_test.go` (existing envtest suite) is extended with
      correlation integration tests:
  - **TC-01 — No correlation, single finding:** one `RemediationJob` created, window
    elapses, job dispatched without `CorrelationGroupID`
  - **TC-02 — SameNamespaceParent correlation:** two jobs with matching parent prefix
    created, window elapses, primary dispatched with group label, secondary `Suppressed`
  - **TC-03 — PVCPod correlation:** PVC finding + Pod finding in same namespace with
    matching volume reference, window elapses, PVC job primary, Pod job `Suppressed`
  - **TC-04 — No correlation across namespaces:** two jobs with identical parent names
    but different namespaces, each dispatched independently (no group)
  - **TC-05 — DISABLE_CORRELATION=true:** two correlated jobs, escape hatch enabled,
    both dispatched immediately without any hold or grouping
- [ ] `DISABLE_CORRELATION=true` skips the window requeue and calls no correlator code
- [ ] `go test -timeout 60s -race ./internal/controller/...` passes (envtest tests need
      60s timeout to allow for the 30s window in TC-01 through TC-04; tests should use
      a reduced window via `CORRELATION_WINDOW_SECONDS=1` in test config)
- [ ] `go test -timeout 30s -race ./...` passes for all non-envtest packages

---

## Technical Implementation

### Test configuration

All correlation integration tests set `cfg.CorrelationWindowSeconds = 1` to keep test
runtime short. The envtest suite injects the config via the reconciler constructor.

### TC-02 setup

```go
// Create two RemediationJobs with parent prefix match
rjob1 := makeRJob("my-app-deploy", "default", "Deployment", "my-app")
rjob2 := makeRJob("my-app-pod", "default", "Pod", "my-app-xyz-abc")
// Create both, wait 2 seconds (> window of 1s), then check phases
```

### TC-03 setup

Requires a `Pod` object to exist in the envtest cluster so `PVCPodRule` can call
`client.Get` and inspect `spec.volumes`. Create a minimal Pod object with a
`persistentVolumeClaim` volume referencing the PVC name in the PVC finding.

### Escape hatch verification (TC-05)

Set `r.Config.DisableCorrelation = true` on the reconciler. Create two correlated jobs.
Verify both reach `Dispatched` phase before the window would have elapsed (use a 500ms
timeout — if they are not dispatched within 500ms, the test fails, indicating the
window was incorrectly applied).

### `SourceProviderReconciler` treatment of `Suppressed`

Add `Suppressed` to the set of non-failed terminal phases that `SourceProviderReconciler`
treats as "already handled" — a finding whose `RemediationJob` is `Suppressed` must not
trigger a new `RemediationJob` creation. This is a one-line addition to the existing
phase check in `internal/provider/provider.go`.

---

## Tasks

- [ ] Extend `internal/controller/suite_test.go` with TC-01 through TC-05 (TDD)
- [ ] Add `Suppressed` to the non-failed phase check in `internal/provider/provider.go`
- [ ] Verify `go test -timeout 60s -race ./internal/controller/...` passes
- [ ] Verify `go test -timeout 30s -race ./...` passes for all other packages
- [ ] Run `go build ./...` and `go vet ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_00, STORY_01, STORY_02, STORY_03, STORY_04 (all prior stories)
**Blocks:** Epic Definition of Done

---

## Definition of Done

- [ ] All five test cases pass in envtest
- [ ] `DISABLE_CORRELATION=true` verified to bypass all correlation logic
- [ ] `Suppressed` phase treated correctly by `SourceProviderReconciler`
- [ ] Full repository test suite passes: `go test -timeout 60s -race ./...`
- [ ] `go build ./...` and `go vet ./...` clean
- [ ] Worklog entry written in `docs/WORKLOGS/`
