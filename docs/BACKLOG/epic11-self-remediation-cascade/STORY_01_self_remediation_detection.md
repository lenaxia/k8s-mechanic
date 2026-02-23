# Story: Self-Remediation Detection in JobProvider

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot operator**, I want the system to automatically detect when mendabot's own agent jobs fail, so that self-remediation investigations can be triggered without manual intervention.

---

## Acceptance Criteria

- [x] JobProvider detects mendabot agent jobs via label `app.kubernetes.io/managed-by: mendabot-watcher`
- [x] Detected jobs are marked with `IsSelfRemediation: true` in the Finding
- [x] Non-mendabot jobs are not marked as self-remediations
- [x] CronJob-owned jobs are excluded from detection (transient by design)
- [x] Suspended jobs are excluded from detection (deliberate pauses)
- [x] Only failed jobs trigger findings (failed > 0, active == 0, completionTime == nil)
- [x] Error messages include job failure context (attempt count, condition reason/message)
- [x] Unit tests verify detection logic for both mendabot and non-mendabot jobs
- [x] Integration tests simulate mendabot job failure scenarios

---

## Technical Implementation

### Location: `internal/provider/native/job.go`

**Key Implementation Details:**

1. **Detection Logic** (lines 76-80):
   ```go
   isMendabotJob := false
   if job.Labels != nil && job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher" {
       isMendabotJob = true
   }
   ```

2. **Failure Detection** (lines 71-73):
   ```go
   if job.Status.Failed == 0 || job.Status.Active != 0 || job.Status.CompletionTime != nil {
       return nil, nil
   }
   ```

3. **Exclusion Rules**:
   - CronJob-owned jobs (lines 54-58)
   - Suspended jobs (lines 61-65)

4. **Error Context** (lines 99-118):
   ```go
   baseText := fmt.Sprintf("job %s: failed (%d attempts, 0 active)", job.Name, job.Status.Failed)
   errors = append(errors, errorEntry{Text: baseText})
   ```

5. **Finding Creation** (lines 127-141):
   ```go
   finding := &domain.Finding{
       IsSelfRemediation: isMendabotJob,
       ChainDepth:        chainDepth,
       // ... other fields
   }
   ```

### Integration Points

- **JobBuilder**: Labels agent jobs with `app.kubernetes.io/managed-by: mendabot-watcher`
- **SourceProviderReconciler**: Applies circuit breaker to self-remediations
- **Config**: `SELF_REMEDIATION_MAX_DEPTH` limits cascade depth

### Testing Requirements

**Unit Tests** (`internal/provider/native/job_test.go`):
- Mendabot job detection via label
- Non-mendabot job exclusion
- CronJob and suspended job exclusion
- Failure condition detection

**Integration Tests** (`internal/provider/native/chaindepth_integration_test.go`):
- Cascade simulation with multiple levels
- Concurrent reconciliation handling
- Max depth enforcement

---

## Tasks

- [x] Implement detection logic in JobProvider.ExtractFinding
- [x] Add error context for job failures
- [x] Write unit tests for detection scenarios
- [x] Write integration tests for cascade scenarios
- [x] Verify integration with JobBuilder labels

---

## Dependencies

**Depends on:** epic02-jobbuilder (agent job labeling)
**Blocks:** STORY_02_chain_depth_tracking, STORY_03_circuit_breaker

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] Detection logic handles edge cases (CronJob, suspended jobs)
- [x] Error messages provide actionable context
- [x] Integration with existing JobBuilder labeling