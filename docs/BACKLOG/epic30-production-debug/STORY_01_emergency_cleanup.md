# Story 01: Emergency Cleanup and Queue Unblock

**Epic:** [epic30-production-debug](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **site reliability engineer**, I need to **manually clean up failed agent jobs** to **unblock the RemediationJob queue** so that pending investigations can be dispatched.

---

## Current Situation

**CRITICAL: This must be done AFTER STORY_02 (artifact collection) or we lose all diagnostic evidence!

**Actual Problem:**
- 3 jobs have been in "Dispatched" phase for 5+ days
- Jobs show status: Failed with conditions including DeadlineExceeded
- MAX_CONCURRENT_JOBS=3 limit is fully occupied by failed jobs
- **RemediationJob controller DOES watch Jobs and DOES sync status** (remediationjob_controller.go:724, syncPhaseFromJob at lines 381-396)
- TTL cleanup logic exists but takes 7 days (REMEDIATION_JOB_TTL_SECONDS=604800)
- **Unknown why jobs are failing** - need artifact collection first!

**Architecture Assessment:**
The controller already has:
- Job watch via `.Owns(&batchv1.Job{})` (line 724)
- Status sync via `syncPhaseFromJob` (lines 381-396)
- TTL cleanup logic (lines 64-127, 113-127)
- Retry logic (lines 312-359, 717-774)
- Correlation window hold (lines 738-786)
- Concurrency gate (lines 947-1017)

**Potential Issues:**
- RemediationJobStatus does NOT have FailureReason or FailureMessage fields to explain job failures
- Jobs may be failing in init container (git clone/auth) - pods would show errors
- Jobs may be stuck at main container (OpenCode startup) - pods would show logs
- Jobs may never be created (dispatch issue) - would stay Pending forever
MAX_CONCURRENT_JOBS=3 (fully occupied by failed jobs)

Failed Jobs (blocking queue):
- mechanic-agent-06a0faa76989 (DeadlineExceeded, created 2026-03-11T01:06:54Z)
- mechanic-agent-d97f1305ef43 (DeadlineExceeded, created 2026-03-11T00:44:59Z)
- mechanic-agent-ff4e33319df2 (DeadlineExceeded, created 2026-03-11T00:46:31Z)

Pending RemediationJobs (waiting for slots):
- 58 jobs stuck in Pending phase
- Some waiting for ~5 days (mechanic-638a33e24c0f created 2026-03-11T00:44:29Z)

TTL Configuration:
- REMEDIATION_JOB_TTL_SECONDS=604800 (7 days)
- Jobs won't auto-cleanup until ~2 days from now
```

**RemediationJob Status:**
```yaml
status:
  phase: Dispatched  # ← Stuck here, never transitions to Failed
  dispatchedAt: "2026-03-11T01:06:54Z"
  jobRef: mechanic-agent-06a0faa76989
  conditions:
    - lastTransitionTime: "2026-03-11T01:06:54Z"
      message: ""
      reason: JobCreated
      status: "True"
      type: JobDispatched
```

**batch/v1 Job Status:**
```yaml
status:
  conditions:
    - lastProbeTime: "2026-03-11T01:21:54Z"
      lastTransitionTime: "2026-03-11T01:21:54Z"
      message: Job was active longer than specified deadline
      reason: DeadlineExceeded
      status: "True"
      type: FailureTarget
    - lastProbeTime: "2026-03-11T01:22:25Z"
      lastTransitionTime: "2026-03-11T01:22:25Z"
      message: Job was active longer than specified deadline
      reason: DeadlineExceeded
      status: "True"
      type: Failed
  failed: 1
  startTime: "2026-03-11T01:06:54Z"
```

---

## Problem

The RemediationJob controller does **not watch batch/v1 Job status**. When a Job fails, the RemediationJob remains in "Dispatched" phase forever because:

1. **No reconciliation trigger:** Only RemediationJob CRD changes trigger reconciliation
2. **No Job status check:** Controller never inspects owned Job's `.status.failed` or `.status.conditions`
3. **No state transition:** "Dispatched" → "Failed" transition never happens
4. **No cleanup:** Failed Jobs persist until TTL expires (7 days)
5. **Queue blocked:** MAX_CONCURRENT_JOBS=3 slots occupied indefinitely

---

## Acceptance Criteria

### Immediate Actions (Emergency)

- [ ] **Collect failure artifacts BEFORE deletion** (see STORY_02 for details)
  - Capture Job specs for all 3 failed Jobs
  - Capture Pod logs (if any pods still exist)
  - Capture Events related to Jobs
  - Capture RemediationJob CRD specs

- [ ] **Delete failed batch/v1 Jobs:**
  ```bash
  kubectl delete job mechanic-agent-06a0faa76989 -n default
  kubectl delete job mechanic-agent-d97f1305ef43 -n default
  kubectl delete job mechanic-agent-ff4e33319df2 -n default
  ```
  Verify deletion: `kubectl get jobs -n default`

- [ ] **Update RemediationJob status to "Failed":**
  ```bash
  # This requires manual kubectl patch since controller doesn't do it
  kubectl patch remediationjob mechanic-06a0faa76989 -n default \
    --type='merge' \
    -p='{"status":{"phase":"Failed","failureReason":"DeadlineExceeded","failureMessage":"Agent job exceeded 900s deadline"}}'
  ```
  Verify update: `kubectl get remediationjob mechanic-06a0faa76989 -o yaml`

- [ ] **Verify queue unblocks:**
  - Watch watcher logs for new Job dispatches
  - Verify pending RemediationJobs transition to "Dispatched"
  - Confirm at least 3 new Jobs created within 10 minutes

### Verification Steps

- [ ] Confirm 0 failed Jobs remain: `kubectl get jobs -n default`
- [ ] Confirm RemediationJob status updated: `kubectl get remediationjobs -n default -o wide`
- [ ] Confirm new Jobs being dispatched: `kubectl logs -n default deployment/mechanic --tail=100 | grep "Creating Job"`
- [ ] Confirm no errors in watcher logs: `kubectl logs -n default deployment/mechanic --tail=100 | grep -i error`

### Documentation

- [ ] Create worklog entry documenting:
  - Exact steps taken
  - Timestamps of each action
  - Verification results
  - Evidence collected
  - Lessons learned

---

## What This Story Does NOT Do

- Fix the underlying cause of agent timeouts (STORY_03)
- Implement automatic cleanup (STORY_05)
- Improve job lifecycle management (STORY_04)
- Add metrics or alerts (STORY_06)

---

## Pre-Requisites

**IMPORTANT:** Complete STORY_02 first to collect failure artifacts before deletion!

Deleting Jobs will destroy all evidence. You MUST collect artifacts first.

---

## Emergency Rollback Plan

If queue does not unblock after manual cleanup:

1. **Restart watcher deployment:**
   ```bash
   kubectl rollout restart deployment mechanic -n default
   ```

2. **Force reconcile pending RemediationJobs:**
   ```bash
   kubectl annotate remediationjobs --all requeue=true -n default
   kubectl annotate remediationjobs --all requeue- -n default
   ```

3. **Increase MAX_CONCURRENT_JOBS temporarily:**
   ```bash
   kubectl set env deployment/mechanic MAX_CONCURRENT_JOBS=6 -n default
   ```

4. **If all else fails:** Scale watcher to 0, then back to 1

---

## Files to Check

| Resource | Command | Purpose |
|----------|----------|---------|
| Failed Jobs | `kubectl get jobs -n default -o yaml` | Capture Job specs |
| RemediationJobs | `kubectl get remediationjobs -n default -o yaml` | Capture job status |
| Job Pods | `kubectl get pods -l job-name=mechanic-agent-* -n default` | Check if pods exist |
| Job Logs | `kubectl logs job/<job-name> -n default` | Capture agent logs |
| Events | `kubectl get events -n default --sort-by='.lastTimestamp'` | Capture failure events |
| Watcher Logs | `kubectl logs deployment/mechanic -n default` | Check for errors |

---

## Estimated Timeline

| Task | Time |
|------|------|
| Artifact collection | 15 minutes |
| Job deletion | 5 minutes |
| RemediationJob patching | 10 minutes |
| Verification | 15 minutes |
| Documentation | 15 minutes |
| **Total** | **1 hour** |

---

## Dependencies

**Blocks:** STORY_03 (cannot investigate if queue blocked)  
**Unblocked By:** STORY_02 (must collect artifacts first)

---

## Definition of Done

- [ ] All 3 failed Jobs deleted
- [ ] All 3 corresponding RemediationJobs updated to "Failed" status
- [ ] Queue unblocked (pending jobs dispatching)
- [ ] At least 3 new Jobs created and running
- [ ] Failure artifacts collected and saved
- [ ] Worklog entry created
- [ ] No errors in watcher logs after cleanup

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Deletion loses critical evidence | High | Low | Collect artifacts first |
| Queue does not unblock | High | Low | Have rollback plan ready |
| New jobs also fail | High | Unknown | Monitor closely, ready to repeat cleanup |
| RemediationJob patch fails | Medium | Low | Use `kubectl edit` if patch fails |

---

## Success Indicators

- **Immediate:** Jobs deleted within 5 minutes
- **Short-term:** New Jobs dispatching within 10 minutes
- **Long-term:** No recurring deadlocks (prevented by subsequent stories)
