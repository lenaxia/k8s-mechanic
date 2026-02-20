# Story: Fake and Stub Implementations

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want fake implementations of the `JobBuilder` interface so that
controller unit tests (specifically `fingerprintFor` and the dedup map logic) can run
without constructing real `batch/v1 Job` objects or touching the Kubernetes API.

---

## Acceptance Criteria

- [ ] `internal/controller/fakes_test.go` (package `controller_test`) defines:

  ```go
  // fakeJobBuilder records calls to Build() for assertion in tests.
  type fakeJobBuilder struct {
      calls     []fakeJobBuilderCall
      returnJob *batchv1.Job
      returnErr error
  }

  type fakeJobBuilderCall struct {
      Result      *v1alpha1.Result
      Fingerprint string
  }

  func (f *fakeJobBuilder) Build(result *v1alpha1.Result, fp string) (*batchv1.Job, error) {
      f.calls = append(f.calls, fakeJobBuilderCall{result, fp})
      return f.returnJob, f.returnErr
  }
  ```

- [ ] `fakeJobBuilder` satisfies `domain.JobBuilder` — verified by a compile-time
  assertion in `fakes_test.go`:
  ```go
  var _ domain.JobBuilder = (*fakeJobBuilder)(nil)
  ```
- [ ] A `defaultFakeJob()` helper returns a minimal valid `*batchv1.Job` usable as the
  return value in tests that need the controller to proceed past the `Build()` call
- [ ] Unit tests in `fakes_test.go` verify:
  - `fakeJobBuilder.Build()` records each call
  - `fakeJobBuilder` with `returnErr != nil` propagates the error
- [ ] No fake of `client.Client` — the controller integration tests use envtest's real
  client. Hand-rolled client fakes introduce more bugs than they prevent.

---

## Scope Boundary

This story defines only the `fakeJobBuilder`. It does not define fake loggers (use
`zap.NewNop()` from the zap package directly), fake schemes (use `runtime.NewScheme()`),
or fake managers (not needed — unit tests for pure functions do not need a manager).

---

## Tasks

- [ ] Create `internal/controller/fakes_test.go` with `fakeJobBuilder`, `fakeJobBuilderCall`,
  `defaultFakeJob()`, and the compile-time assertion
- [ ] Write tests for the fake itself (TDD — even fakes need tests)

---

## Dependencies

**Depends on:** STORY_02 (JobBuilder interface)
**Depends on:** epic00-foundation/STORY_04 (CRD types — fakeJobBuilderCall stores `*v1alpha1.Result`)
**Blocks:** epic01-controller/STORY_02 (fingerprintFor tests use fakeJobBuilder indirectly)
**Blocks:** epic01-controller/STORY_03 (dedup map tests inject fakeJobBuilder)
**Blocks:** epic01-controller/STORY_04 (reconcile loop unit tests)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] Compile-time interface assertion present
