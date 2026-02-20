# Story: Environment Variable Injection

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want all FINDING_* and GITOPS_REPO environment variables injected
into the main container spec so the OpenCode agent has full finding context available at
runtime.

---

## Acceptance Criteria

- [ ] All variables from JOBBUILDER_LLD.md §4 are present in the main container env
- [ ] `FINDING_ERRORS` is valid compact JSON (from `json.Marshal(result.Spec.Error)`)
- [ ] `FINDING_ERRORS` with a nil error slice marshals to `null` or `[]` without error
- [ ] Secret-sourced vars (GITHUB_APP_*, OPENAI_*) use `valueFrom.secretKeyRef`
- [ ] Config-sourced vars (GITOPS_REPO) use literal `value`
- [ ] Unit tests verify all variables are present and correctly sourced

---

## Tasks

- [ ] Write tests first (TDD) — test helper to find env var by name in container spec
- [ ] Implement env var building in `Build()`
- [ ] Handle `json.Marshal` error on `result.Spec.Error` (wrap and return)

---

## Dependencies

**Depends on:** STORY_02 (job name)
**Blocks:** STORY_04 (init container), STORY_05 (main container)

---

## Definition of Done

- [ ] All env var tests pass with `-race`
- [ ] `go vet` clean
