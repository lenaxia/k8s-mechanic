# Story: envtest Suite Setup

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want a working envtest suite that starts a real Kubernetes API server,
installs the k8sgpt Result CRD, and tears everything down after each test suite so that
integration tests in epic01 can run without a live cluster.

---

## Acceptance Criteria

- [ ] `internal/controller/suite_test.go` contains the `TestMain` / Ginkgo/testenv bootstrap
  that:
  - Starts `envtest.Environment` with the CRD path pointing at the k8sgpt Result CRD
  - Creates a `ctrl.Manager` backed by the test API server
  - Registers the Result scheme
  - Tears down cleanly after all tests
- [ ] The suite compiles and `go test -timeout 300s ./internal/controller/...` succeeds
  (with no test cases yet — the suite just starts and stops)
- [ ] A minimal smoke test `TestSuite_StartsAndStops` verifies the envtest API server is
  reachable by listing namespaces
- [ ] The envtest binary path is resolved via `controller-runtime/pkg/envtest` — no
  hardcoded paths
- [ ] The CRD manifest for `results.core.k8sgpt.ai` is placed at
  `testdata/crds/result_crd.yaml` — sourced from the k8sgpt-operator repo and pinned

---

## Why envtest, Not Mocks

The controller interacts with the Kubernetes API in non-trivial ways: it lists Jobs by
label, creates Jobs, and handles `AlreadyExists`. These behaviours are not safely testable
with a hand-rolled fake client — subtle API semantics (list pagination, resource version
conflicts, watch events) require a real API server. envtest provides this with zero cluster
dependency.

---

## CRD Source

The `result_crd.yaml` is sourced from the k8sgpt-operator repository:
`https://github.com/k8sgpt-ai/k8sgpt-operator/blob/main/config/crd/bases/core.k8sgpt.ai_results.yaml`

It must be committed to `testdata/crds/result_crd.yaml` verbatim and treated as a
vendored dependency. If the CRD schema changes in a future k8sgpt-operator release, the
file is updated deliberately — not automatically.

---

## Framework Choice

Use the standard `controller-runtime` envtest setup with Go's `testing` package and
`TestMain`. Do **not** introduce Ginkgo/Gomega — the rest of the test suite uses plain
`testing` and table-driven tests. Consistency matters more than DSL expressiveness.

---

## Tasks

- [ ] Add `sigs.k8s.io/controller-runtime/pkg/envtest` setup binary to the module
- [ ] Create `testdata/crds/result_crd.yaml`
- [ ] Create `internal/controller/suite_test.go` with `TestMain` that starts/stops envtest
- [ ] Write `TestSuite_StartsAndStops` smoke test
- [ ] Document how to install envtest binaries locally (`setup-envtest use`) in a comment
  at the top of `suite_test.go`

---

## Dependencies

**Depends on:** STORY_03 (reconciler skeleton — suite needs a manager to start)
**Depends on:** epic00-foundation/STORY_04 (CRD types — scheme registration)
**Blocks:** epic01-controller/STORY_07 (integration tests)

---

## Definition of Done

- [ ] `go test -timeout 300s -race ./internal/controller/...` passes
- [ ] `go vet` clean
- [ ] Suite starts and stops without leaving stale processes
- [ ] `testdata/crds/result_crd.yaml` is committed
