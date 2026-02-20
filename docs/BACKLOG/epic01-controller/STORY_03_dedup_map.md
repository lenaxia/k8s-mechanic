# Story: In-Memory Processed Map

**Epic:** [Controller](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the `ResultReconciler` to maintain a mutex-protected in-memory
map of processed fingerprints so that the same finding is not dispatched twice within a
single watcher lifetime.

---

## Acceptance Criteria

- [ ] `ResultReconciler` has a `mu sync.Mutex` and `processed map[string]processedEntry`
- [ ] `processedEntry` holds fingerprint, dispatchedAt time, and jobName
- [ ] Map is initialised in `SetupWithManager`
- [ ] Read and write to the map are always within the mutex lock
- [ ] A fingerprint present in the map causes the reconcile to return nil immediately
  with no requeue — the Result is in a settled state; nothing more to do
- [ ] Unit test: first reconcile dispatches, second reconcile with same fingerprint skips

---

## Tasks

- [ ] Define `processedEntry` struct
- [ ] Add `mu` and `processed` fields to `ResultReconciler`
- [ ] Initialise map in `SetupWithManager`
- [ ] Add map check to reconcile loop (before Job creation)
- [ ] Add map write to reconcile loop (after Job creation)
- [ ] Write unit tests (TDD)

---

## Dependencies

**Depends on:** STORY_02 (fingerprint)
**Blocks:** STORY_04 (reconcile loop)

---

## Definition of Done

- [ ] Tests pass with `-race` (race detector must not flag the map access)
- [ ] `go vet` clean
