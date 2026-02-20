# Story: fingerprintFor Implementation and Tests

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1.5 hours

---

## User Story

As a **developer**, I want a `fingerprintFor()` function that produces a stable,
parent-resource-aware SHA256 hash from a `ResultSpec` so that multiple pods from the same
Deployment produce one fingerprint, not many.

---

## Acceptance Criteria

- [ ] Same `kind` + `parentObject` + same error texts → same fingerprint, regardless of
  resource name or error order
- [ ] Different `parentObject` → different fingerprint (even with identical errors)
- [ ] Different `kind` → different fingerprint
- [ ] Different error texts → different fingerprint
- [ ] nil error slice and empty error slice produce the same fingerprint
- [ ] Function is deterministic — same input always produces same output
- [ ] Returns a 64-character lowercase hex string

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `SameParentDifferentPods` | Same kind/parent/errors, different Name | Same fingerprint |
| `DifferentErrors` | Same kind/parent, different error text | Different fingerprint |
| `ErrorOrderIndependent` | Same errors in different order | Same fingerprint |
| `DifferentParents` | Same errors, different parentObject | Different fingerprint |
| `EmptyErrors` | nil vs `[]Failure{}` | Same fingerprint |
| `DifferentKinds` | Same parent/errors, different kind | Different fingerprint |
| `Deterministic` | Same spec called twice | Same output both times |

---

## Tasks

- [ ] Write all 7 tests in `internal/provider/k8sgpt/reconciler_test.go` (TDD — tests first, must fail)
- [ ] Implement `fingerprintFor()` in `internal/provider/k8sgpt/reconciler.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_01 (scheme)
**Blocks:** STORY_03 (dedup map), STORY_04 (reconcile loop)

---

## Definition of Done

- [ ] All 7 tests pass with `-race`
- [ ] `go vet` clean
