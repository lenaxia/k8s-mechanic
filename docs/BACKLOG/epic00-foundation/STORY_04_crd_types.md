# Story: Vendored CRD Types

**Epic:** [Foundation](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the k8sgpt-operator `Result` CRD types available in
`api/v1alpha1/` without depending on the full operator module so that the watcher can
watch and read Result objects with full type safety.

---

## Acceptance Criteria

- [ ] `api/v1alpha1/result_types.go` defines `Result`, `ResultList`, `ResultSpec`,
  `ResultStatus`, `Failure`, `Sensitive`
  (`AutoRemediationStatus` is intentionally omitted — the watcher does not use it;
  see CONTROLLER_LLD.md §3.1)
- [ ] Both `Result` and `ResultList` implement `runtime.Object` via `DeepCopyObject()`
- [ ] `DeepCopyInto()` performs a true deep copy (no shared slice references)
- [ ] `AddToScheme` registers both types with a `runtime.Scheme`
- [ ] `GroupVersion` is `core.k8sgpt.ai/v1alpha1`
- [ ] Unit tests verify `DeepCopyObject()` produces an independent copy (mutating the
  copy does not affect the original)

---

## Tasks

- [ ] Write tests in `api/v1alpha1/result_types_test.go` first (TDD)
- [ ] Implement `result_types.go` with all types
- [ ] Implement `DeepCopyObject()` and `DeepCopyInto()` for both `Result` and `ResultList`
- [ ] Verify scheme registration compiles and registers correctly

---

## Dependencies

**Depends on:** STORY_01 (module setup)
**Blocks:** Controller epic STORY_01 (scheme registration)

---

## Definition of Done

- [ ] All deep copy tests pass with `-race`
- [ ] `go vet` clean
- [ ] No import of the full k8sgpt-operator module
