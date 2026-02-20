# Story: Error-Filter Predicate

**Epic:** [Controller](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want Results with zero errors filtered out before entering the
reconcile queue so the controller never dispatches an investigation for a healthy resource.

---

## Acceptance Criteria

- [ ] `predicate.NewPredicateFuncs` filter applied in `SetupWithManager`
- [ ] Results with `len(spec.error) == 0` do not enter the reconcile queue
- [ ] Results with `len(spec.error) > 0` pass through normally
- [ ] Non-Result objects (e.g. owned Jobs) pass through unchanged
- [ ] Unit test verifies the predicate function directly

---

## Tasks

- [ ] Write predicate unit test (TDD)
- [ ] Implement predicate in `SetupWithManager`
- [ ] Add `Owns(&batchv1.Job{})` so Job events from owned Jobs are tracked

---

## Dependencies

**Depends on:** STORY_02 (fingerprint)
**Blocks:** STORY_07 (integration tests)

---

## Definition of Done

- [ ] Test passes with `-race`
- [ ] `go vet` clean
