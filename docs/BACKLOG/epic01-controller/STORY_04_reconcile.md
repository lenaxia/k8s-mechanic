# Story: Reconcile Loop (Dispatch Path)

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want the `Reconcile()` method to fetch a Result, compute its
fingerprint, check the processed map, build a Job, and create it — handling all error
cases correctly.

---

## Acceptance Criteria

- [ ] `Reconcile()` fetches the Result; returns nil if NotFound
- [ ] Computes fingerprint and checks processed map; returns nil immediately if already seen (no requeue)
- [ ] Calls `jobBuilder.Build()` to produce the Job spec
- [ ] Calls `client.Create(ctx, job)` to create the Job
- [ ] On `IsAlreadyExists`: logs at Info level, records as processed, returns nil
- [ ] On any other create error: returns wrapped error (controller-runtime requeues)
- [ ] Records fingerprint+jobName in processed map after successful creation
- [ ] Logs with structured fields on every significant path

---

## Integration Test Cases (envtest — write tests first)

| Test | Expected |
|------|----------|
| `NewResult_CreatesJob` | New Result with errors → Job created in correct namespace |
| `AlreadyProcessed_SkipsJob` | Same fingerprint twice → second call skips |
| `NoErrors_Skipped` | Result with no errors → no Job (filtered by predicate) |
| `JobAlreadyExists_RecordsAsProcessed` | Job exists → recorded, no error |
| `ResultDeleted_NoOp` | Reconcile for deleted Result → nil return, no Job |
| `DifferentParents_TwoJobs` | Two Results with different parents → two Jobs |
| `ErrorTextChanges_NewJob` | Same parent, changed error text → new fingerprint, new Job |

---

## Tasks

- [ ] Write all 7 envtest integration tests first (must fail before implementation)
- [ ] Implement `Reconcile()` method body
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (dedup map), Job Builder STORY_01 (builder struct)
**Blocks:** STORY_06 (manager wiring)

---

## Definition of Done

- [ ] All 7 integration tests pass with `-race`
- [ ] `go vet` clean
