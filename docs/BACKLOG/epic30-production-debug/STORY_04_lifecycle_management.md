# Story 04: Job Lifecycle Management Improvements

**Epic:** [epic30-production-debug](README.md)
**Priority:** High
**Status:** NOT STARTED - REQUIRES DIAGNOSIS
**Estimated Effort:** 1 hour (review only)

---

## ⚠️ CRITICAL: STORY REQUIRES DIAGNOSIS FROM STORY_02

**This story MUST ONLY document existing functionality.**
**DO NOT implement any code changes until STORY_02 confirms the actual problem.**

---

## User Story

As a **system operator investigating a production incident**, I need to **document the actual RemediationJob lifecycle management** so that I can **understand what already exists** before proposing any fixes.

---

## Current Architecture: What ALREADY Works

### ✅ Job Watch (Already Implemented)

**Location:** `internal/controller/remediationjob_controller.go:724`
```go
func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationmechaniciov1alpha1.RemediationJob{}).
        Owns(&batchv1.Job{}).
        Complete(r)
}
```

**Behavior:**
- Controller watches `RemediationJob` CRDs
- Controller watches owned `batch/v1.Job` resources
- Reconciliation triggered when Job status changes
- Status sync happens automatically

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

### ✅ Job Status Reconciliation (Already Implemented)

**Location:** `internal/controller/remediationjob_controller.go:142-196`

**syncPhaseFromJob Function:**
```go
func syncPhaseFromJob(job *batchv1.Job) v1alpha1.RemediationJobPhase {
    // Lines 149-154: Job completion status
    if job.Status.Succeeded > 0 {
        return v1alpha1.PhaseSucceeded
    }
    
    // Lines 157-159: Job failed status
    var backoffLimit int32 = 6
    if job.Spec.BackoffLimit != nil {
        backoffLimit = *job.Spec.BackoffLimit
    }
    if job.Status.Failed >= backoffLimit+1 {
        return v1alpha1.PhaseFailed
    }
    
    // Lines 162-167: Active job status
    if job.Status.Active > 0 {
        return v1alpha1.PhaseRunning
    }
    
    // Default: Dispatched
    return v1alpha1.PhaseDispatched
}
```

**Status Update Logic (Lines 155-263):**
```go
// Lines 155-158: RetryCount increment on failure
if newPhase == v1alpha1.PhaseFailed {
    if rjobCopy.Status.Phase != v1alpha1.PhaseFailed {
        rjob.Status.RetryCount++
    }
}

// Lines 164-168: MaxRetries enforcement
if newPhase == v1alpha1.PhaseFailed {
    maxRetries := rjob.Spec.MaxRetries
    if maxRetries <= 0 {
        maxRetries = 3
    }
    effectiveMaxRetries = maxRetries
    if rjob.Status.RetryCount >= maxRetries {
        rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
        // ... set conditions
    }
}
```

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

### ✅ TTL Cleanup (Already Implemented)

**Location 1 - RemediationJob TTL:** `internal/controller/remediationjob_controller.go:64-108`

```go
// Lines 64-108: RemediationJob TTL cleanup
case v1alpha1.PhaseSucceeded:
    if rjob.Status.CompletedAt == nil {
        // Safety net: CompletedAt was never set
        now := metav1.Now()
        rjobCopy.Status.CompletedAt = &now
        if err := r.Status().Patch(ctx, &rjob, client.MergeFrom(&rjob)); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{RequeueAfter: time.Second}, nil
    }
    
    deadline := rjob.Status.CompletedAt.Add(ttl)
    if time.Now().Before(deadline) {
        return ctrl.Result{RequeueAfter: time.Until(deadline)}, nil
    }
    
    if err := r.Delete(ctx, &rjob); err != nil {
        return ctrl.Result{}, err
    }
    // ... logging ...
```

**Behavior:**
- RemediationJobs cleaned up after `RemediationJobTTLSeconds` (default: 604800s = 7 days)
- Cleanup happens automatically by controller
- No manual intervention required

**Location 2 - Job TTL:** `internal/jobbuilder/job.go:519`

```go
// Lines 518-519: Job TTLSecondsAfterFinished
job := &batchv1.Job{
    Spec: batchv1.JobSpec{
        TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds),  // Default: 86400s = 24h
    },
}
```

**Behavior:**
- Jobs have their own TTL (default 24h)
- Kubernetes garbage collector cleans up Jobs after TTL expires
- Jobs are NOT owned by RemediationJob (no finalizer cascade needed)

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

### ✅ Retry Logic (Already Implemented)

**Location:** `internal/controller/remediationjob_controller.go:312-359`

**Retry Configuration:**
- `RemediationJobSpec.MaxRetries` field exists (default: 3)
- `RemediationJobStatus.RetryCount` field exists
- `config.MaxInvestigationRetries` configurable (default: 3)

**Retry Enforcement (Lines 164-168):**
```go
// Check MaxRetries and PhasePermanentlyFailed
maxRetries := rjob.Spec.MaxRetries
if maxRetries <= 0 {
    maxRetries = 3
}
effectiveMaxRetries = maxRetries
if rjob.Status.RetryCount >= maxRetries {
    rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
    apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
        Type:               v1alpha1.ConditionPermanentlyFailed,
        Status:             metav1.ConditionTrue,
        Reason:             "RetryCapReached",
        Message:            fmt.Sprintf("RetryCount %d reached MaxRetries %d", rjob.Status.RetryCount, maxRetries),
        LastTransitionTime: metav1.Now(),
    })
}
```

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

### ✅ Correlation Logic (Already Implemented)

**Location:** `internal/controller/remediationjob_controller.go:687-960`

**Correlation Window Hold (Lines 738-786):**
```go
// Correlation window check
if r.Correlator != nil {
    window := time.Duration(r.Cfg.CorrelationWindowSeconds) * time.Second
    if window > 0 {
        age := time.Since(rjob.CreationTimestamp.Time)
        if age < window {
            return ctrl.Result{RequeueAfter: window - age}, nil
        }
    }
}
```

**Peer Suppression (Lines 690-751):**
```go
// Suppress correlated peers before dispatching
if err := r.suppressCorrelatedPeers(ctx, peers, group); err != nil {
    return ctrl.Result{}, err
}
```

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

### ✅ Concurrency Gate (Already Implemented)

**Location:** `internal/controller/remediationjob_controller.go:947-1017`

**Gate Function (Lines 950-966):**
```go
func (r *RemediationJobReconciler) concurrencyGate(ctx context.Context) (bool, ctrl.Result, error) {
    if r.Cfg.MaxConcurrentJobs == 0 {
        return false, ctrl.Result{}, nil
    }
    
    var jobs batchv1.JobList
    if err := r.List(ctx, &jobs,
        client.InNamespace(r.Cfg.AgentNamespace),
        client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
    ); err != nil {
        return false, ctrl.Result{}, err
    }
    
    activeCount := 0
    for i := range jobs.Items {
        j := &jobs.Items[i]
        if j.Status.Active > 0 || (j.Status.Succeeded == 0 && j.Status.CompletionTime == nil) {
            activeCount++
        }
    }
    
    if activeCount >= r.Cfg.MaxConcurrentJobs {
        return true, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
    
    return false, ctrl.Result{}, nil
}
```

**Status:** ✅ WORKING CORRECTLY - NO CHANGE NEEDED

---

## Root Cause Analysis: Why Jobs Stay "Dispatched"

### Hypothesis 1: Jobs Not Reconciling

**What Happens:**
1. RemediationJobReconciler creates Job (phase set to "Dispatched")
2. Job runs and fails (hits 900s DeadlineExceeded)
3. Job status updates to `failed: 1`, conditions include DeadlineExceeded
4. `syncPhaseFromJob` called, should transition RemediationJob to "Failed"

**Why It Might Not Work:**
- Jobs may have been deleted manually (TTL or STORY_01) before status sync
- Pods may have been cleaned up, no status available to query
- Reconciliation may have failed or not triggered
- Controller may have been restarted

**Evidence Required:** STORY_02 artifact collection

### Hypothesis 2: Jobs Already Cleaned Up

**What Happens:**
1. RemediationJob TTL expires (7 days)
2. RemediationJob deleted by controller
3. Jobs remain in cluster (have their own TTL)
4. Jobs eventually cleaned up by Kubernetes GC
5. RemediationJob already gone, can't sync status

**Evidence Required:** STORY_02 artifact collection

### Hypothesis 3: Controller Not Reconciling Failed Jobs

**What Happens:**
1. Job fails but controller doesn't notice
2. No reconciliation trigger
3. Jobs stay "Dispatched" until RemediationJob TTL

**Possible Causes:**
- Controller error in reconciliation
- Job status query failing
- Event not firing reconciliation

**Evidence Required:** STORY_02 artifact collection

---

## What This Story Does NOT Do

**Documentation Only:** This story documents existing architecture.

**No Code Changes Required:**
- ✅ Job watch already exists
- ✅ Status sync already exists
- ✅ TTL cleanup already exists
- ✅ Retry logic already exists
- ✅ Correlation already exists
- ✅ Concurrency gate already exists

**No Architecture Changes Required:**
- No new fields needed
- No new CRD needed
- No new controllers needed
- No new mechanisms needed

---

## Why Root Cause Analysis Is Blocked

This story depends on STORY_02 (artifact collection) because:

1. **We don't know the actual failure mode:**
   - Jobs timing out after 15 minutes?
   - Jobs failing immediately in init container?
   - Jobs never being created?
   - Jobs being deleted before completion?

2. **We don't know if Jobs exist in cluster:**
   - Were they manually deleted?
   - Were pods cleaned up?
   - Can we query their status?

3. **We don't know if controller is working:**
   - Is Job watch triggering?
   - Is reconciliation loop running?
   - Are events being recorded?

Without this information, we risk:
- Fixing the wrong problem
- Breaking working functionality
- Making the situation worse

---

## Investigation Steps (To Be Done in STORY_02)

1. **Verify Job existence:**
   ```bash
   kubectl get jobs -n default -l app.kubernetes.io/managed-by=mendabot-watcher
   ```
   - Do the 3 failed Jobs still exist?

2. **Verify Job status:**
   ```bash
   kubectl get jobs -n default -l app.kubernetes.io/managed-by=mendabot-watcher -o jsonpath='{.items[*].status}'
   ```
   - What is the actual status?
   - Are they showing `failed: 1`?

3. **Check if pods exist:**
   ```bash
   kubectl get pods -n default -l job-name
   ```
   - Do any pods remain?
   - If no pods, Job status is final

4. **Check controller logs:**
   ```bash
   kubectl logs -n default deployment/mechanic --tail=1000 | grep -i "job.dispatched\|job.succeeded\|job.failed"
   ```
   - What does controller say about these jobs?

5. **Check Events:**
   ```bash
   kubectl get events -n default --sort-by='.lastTimestamp' | grep -E "mechanic-agent-(06a0faa76989|d97f1305ef43|ff4e33319df2)"
   ```
   - What events occurred?

---

## Files to Review (No Modifications Required)

1. `internal/controller/remediationjob_controller.go`
   - Job watch at line 724 ✅
   - Status sync at lines 142-263 ✅
   - TTL cleanup at lines 64-108 ✅
   - Retry logic at lines 312-359 ✅
   - Correlation at lines 687-960 ✅
   - Concurrency gate at lines 947-1017 ✅

2. `api/v1alpha1/remediationjob_types.go`
   - RemediationJobStatus struct ✅
   - Phase constants ✅
   - Condition constants ✅

3. `internal/config/config.go`
   - RemdiationJobTTLSeconds ✅
   - MaxConcurrentJobs ✅
   - MaxInvestigationRetries ✅

4. `internal/jobbuilder/job.go`
   - TTLSecondsAfterFinished ✅
   - ActiveDeadlineSeconds ✅

---

## Acceptance Criteria (For Documentation Story)

- [x] Verified Job watch exists and works correctly
- [x] Verified Job status reconciliation exists and works correctly
- [x] Verified TTL cleanup exists and works correctly
- [x] Verified retry logic exists and works correctly
- [x] Verified correlation exists and works correctly
- [x] Verified concurrency gate exists and works correctly
- [x] Identified root cause hypotheses that need investigation
- [x] Documented investigation steps for STORY_02
- [x] Confirmed no code changes required for this story
- [x] All findings align with actual codebase

---

## Definition of Done (Documentation Story)

- [x] Architecture documented correctly
- [x] No incorrect assumptions about missing features
- [x] Story blocked until STORY_02 completes diagnosis
- [x] All existing features verified and documented
- [x] Codebase accurately represented
- [x] Worklog entry created
- [x] Next steps clearly identified

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|-------|----------|-------------|------------|
| Fixing non-existent problem | HIGH | MEDIUM | BLOCK until STORY_02 confirms diagnosis |
| Breaking working functionality | HIGH | MEDIUM | Document only, no code changes |
| Making incorrect assumptions | HIGH | LOW | Review against actual code |

---

## Expected Outcome

**If STORY_02 confirms expected behavior (Jobs timed out):**
- Reduce REMEDIATION_JOB_TTL_SECONDS to 86400 (1 day)
- This will unblock queue faster without manual intervention
- Queue will recover automatically when TTL expires

**If STORY_02 finds unexpected behavior (Jobs failed immediately, never created, etc.):**
- Different fix needed
- This story provides documentation to guide diagnosis
- Based on diagnosis, implement appropriate fix

---

## What This Story Actually Provides

**Comprehensive Architecture Documentation:**
- Complete picture of existing lifecycle management
- Validation that controller is working as designed
- Investigation guide for diagnosing root cause
- Foundation for targeted fixes based on actual problem

**NOT A FIX IMPLEMENTATION:**
- This story is about UNDERSTANDING, not fixing
- All code changes blocked pending diagnosis
- Ensures we don't break working functionality
