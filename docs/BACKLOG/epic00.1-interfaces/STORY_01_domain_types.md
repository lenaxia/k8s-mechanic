# Story: Core Domain Types

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want all shared domain types defined in one place so that the
controller and job builder packages share a single, authoritative definition with no
duplication.

---

## Acceptance Criteria

- [ ] `internal/domain/types.go` defines `processedEntry` with all fields used by the
  controller reconcile loop:
  ```go
  type processedEntry struct {
      fingerprint  string
      dispatchedAt time.Time
      jobName      string
      resultRef    types.NamespacedName
  }
  ```
- [ ] `internal/domain/types.go` defines `JobBuilderConfig` — the typed config passed to
  the job builder:
  ```go
  type JobBuilderConfig struct {
      GitOpsRepo         string
      GitOpsManifestRoot string
      AgentImage         string
      AgentNamespace     string
      AgentSA            string
  }
  ```
  (This is derived from `Config` but is its own type — the job builder must not depend
  on the full `Config` struct or the config package.)
- [ ] Unit tests in `internal/domain/types_test.go` verify:
  - `processedEntry` zero value is safe to use
  - `JobBuilderConfig` fields are all string types (compile-time — no runtime test needed)
- [ ] No other package (config, controller, jobbuilder) duplicates these types

---

## Why a Separate `domain` Package

The controller imports `config` and the job builder also uses config values. If both
packages define their own config struct they diverge. A shared `domain` package with no
dependencies on either `config` or `controller` breaks the circular import risk and makes
the types independently testable.

---

## Tasks

- [ ] Create `internal/domain/types.go`
- [ ] Create `internal/domain/types_test.go` (TDD — write tests first)
- [ ] Verify `go build ./...` still clean

---

## Dependencies

**Depends on:** epic00-foundation/STORY_01 (module setup)
**Blocks:** STORY_02 (Builder interface), STORY_03 (Reconciler skeleton)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] No duplicate type definitions elsewhere in the codebase
