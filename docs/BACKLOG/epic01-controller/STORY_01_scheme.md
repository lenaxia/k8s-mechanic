# Story: Result CRD Scheme Registration

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the controller-runtime manager to have the Result CRD types
registered in its scheme so the informer can deserialise Result objects from the API server.

---

## Acceptance Criteria

- [ ] `v1alpha1.AddToScheme` is called in the scheme setup block in `main.go`
- [ ] `batchv1.AddToScheme` is called (needed for Job creation)
- [ ] `clientgoscheme.AddToScheme` is called (core types)
- [ ] The manager starts without scheme-related errors
- [ ] Test verifies scheme contains `Result` and `ResultList` kinds

---

## Tasks

- [ ] Write test verifying scheme registration
- [ ] Add scheme setup to `cmd/watcher/main.go`
- [ ] Confirm manager Start does not panic on missing types

---

## Dependencies

**Depends on:** Foundation STORY_04 (CRD types)
**Blocks:** STORY_02 (fingerprint)

---

## Definition of Done

- [ ] Test passes with `-race`
- [ ] Manager compiles and starts cleanly
