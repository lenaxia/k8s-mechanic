# Story: Integration Tests (envtest)

**Epic:** [Controller](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want a full envtest-based integration test suite for the controller
so that the reconcile loop is tested against a real (in-process) API server without
requiring a live cluster.

---

## Acceptance Criteria

- [ ] `envtest` environment set up in `TestMain`
- [ ] All 7 test cases from STORY_04 pass against the envtest API server
- [ ] Tests clean up created resources after each test
- [ ] Tests use `Eventually` with a timeout rather than `time.Sleep`
- [ ] Tests run in CI without a real cluster

---

## Tasks

- [ ] Add `sigs.k8s.io/controller-runtime/pkg/envtest` to `go.mod`
- [ ] Write `TestMain` with envtest setup/teardown
- [ ] Implement all 7 integration test cases
- [ ] Add `KUBEBUILDER_ASSETS` env var setup to `test.yaml` CI workflow

---

## Dependencies

**Depends on:** STORY_06 (manager wiring)
**Blocks:** Nothing — this is the last controller story

---

## Definition of Done

- [ ] All tests pass with `-race` and `-timeout 120s`
- [ ] Tests pass in CI (GitHub Actions `test.yaml`)
- [ ] No real cluster required
