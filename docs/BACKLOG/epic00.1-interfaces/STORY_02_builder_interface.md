# Story: Builder Interface

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the job builder exposed as an interface so that the controller
can depend on the abstraction rather than the concrete implementation, enabling unit testing
without constructing real `batch/v1 Job` objects.

---

## Acceptance Criteria

- [ ] `internal/domain/interfaces.go` defines:
  ```go
  type JobBuilder interface {
      Build(result *v1alpha1.Result, fingerprint string) (*batchv1.Job, error)
  }
  ```
- [ ] The interface is the only thing the controller imports from the job builder package —
  the controller never references `jobbuilder.Builder` directly
- [ ] The concrete `*jobbuilder.Builder` satisfies this interface (verified by a compile-time
  assertion in `internal/jobbuilder/job.go`):
  ```go
  var _ domain.JobBuilder = (*Builder)(nil)
  ```
- [ ] No functional logic is added in this story — interface definition only

---

## Tasks

- [ ] Add `JobBuilder` interface to `internal/domain/interfaces.go` (TDD: write a
  compile-test in `interfaces_test.go` that references the interface by name)
- [ ] Add compile-time assertion to `internal/jobbuilder/job.go` once that package exists
  (add as a TODO comment for now if jobbuilder is not yet written)

---

## Dependencies

**Depends on:** STORY_01 (domain types — same file/package)
**Depends on:** epic00-foundation/STORY_04 (CRD types — interface signature references `*v1alpha1.Result`)
**Blocks:** epic01-controller (controller field `JobBuilder domain.JobBuilder`)
**Blocks:** STORY_05 (fake implementation satisfies this interface)

---

## Definition of Done

- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] Interface is in `internal/domain` — not in `internal/controller` or `internal/jobbuilder`
