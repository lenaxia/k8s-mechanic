# Story: Builder Struct and Config

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want a `Builder` struct with a `New(cfg Config)` constructor so the
job builder has a clean, testable entry point with all configuration injected at creation time.

---

## Acceptance Criteria

- [ ] `Config` struct defined with all fields from JOBBUILDER_LLD.md §3
- [ ] `Builder` struct holds a `Config`
- [ ] `New(cfg Config) *Builder` constructor validates that no required field is empty,
  returning an error if any is missing
- [ ] Unit tests: valid config constructs successfully; each required field missing returns
  a descriptive error

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Define `Config` and `Builder` in `internal/jobbuilder/job.go`
- [ ] Implement `New()` with validation

---

## Dependencies

**Depends on:** Foundation epic00 complete
**Blocks:** All other jobbuilder stories

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
