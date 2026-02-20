# Story: Domain Interfaces

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want the job builder and source provider exposed as interfaces so
that reconcilers depend on abstractions rather than concrete implementations, enabling
unit testing without real Kubernetes objects or external signal sources.

---

## Acceptance Criteria

- [ ] `internal/domain/interfaces.go` defines:
  ```go
  type JobBuilder interface {
      Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)
  }
  ```
- [ ] `internal/provider/interface.go` defines:
  ```go
  // SourceProvider is implemented by each signal source (k8sgpt, Prometheus, etc.)
  // Each provider registers its own reconciler(s) with the manager at startup.
  type SourceProvider interface {
      SetupWithManager(mgr ctrl.Manager) error
  }
  ```
- [ ] The `JobBuilder` interface is the only thing the `RemediationJobReconciler` imports
  from the job builder package — the reconciler never references `jobbuilder.Builder` directly
- [ ] The concrete `*jobbuilder.Builder` satisfies `domain.JobBuilder` (verified by a
  compile-time assertion in `internal/jobbuilder/job.go`):
  ```go
  var _ domain.JobBuilder = (*Builder)(nil)
  ```
- [ ] `K8sGPTSourceProvider` satisfies `provider.SourceProvider` (verified by a
  compile-time assertion in `internal/provider/k8sgpt/provider.go`):
  ```go
  var _ provider.SourceProvider = (*K8sGPTSourceProvider)(nil)
  ```
- [ ] No functional logic is added in this story — interface definitions only

---

## Tasks

- [ ] Add `JobBuilder` interface to `internal/domain/interfaces.go`
- [ ] Create `internal/provider/interface.go` with `SourceProvider` interface
- [ ] Add compile-time assertion comments/TODOs in `internal/jobbuilder/` and
  `internal/provider/k8sgpt/` for when those packages are created
- [ ] Write compile-level test in `internal/domain/interfaces_test.go` that references
  both interfaces by name (ensures they stay importable)

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob type — interface signatures use `*v1alpha1.RemediationJob`)
**Blocks:** STORY_03 (ReconcilerSkeleton has a `JobBuilder domain.JobBuilder` field and uses provider loop)
**Blocks:** STORY_05 (fake implementations satisfy these interfaces)

---

## Definition of Done

- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] `JobBuilder` interface lives in `internal/domain`, not in `internal/controller` or `internal/jobbuilder`
- [ ] `SourceProvider` interface lives in `internal/provider`, not in `internal/provider/k8sgpt`
