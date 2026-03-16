# Story 07: Improved Timeout and Retry Strategies

**Epic:** [epic30-production-debug](README.md)
**Priority:** Medium
**Status:** NOT STARTED - REQUIRES DIAGNOSIS
**Estimated Effort:** 2 hours

---

## ⚠️ CRITICAL: STORY REQUIRES DIAGNOSIS FROM STORY_02

**This story MUST be based on actual gaps, not assumptions.**
**DO NOT implement features that already exist or aren't needed based on diagnosis.**

---

## User Story

As a **system operator**, I want **intelligent timeout and retry strategies** for agent jobs so that **transient failures recover automatically** and **permanent failures fail fast**.

---

## What Already Exists (Code Verification)

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

**Current Behavior:**
- Jobs that fail are automatically marked "PhaseFailed"
- RetryCount incremented on each failure
- After MaxRetries, marked "PhasePermanentlyFailed"
- PhasePermanentlyFailed jobs are NOT re-dispatched

**Status:** ✅ WORKING CORRECTLY - Retry logic exists

---

### ✅ Job Timeout (Partially Configurable)

**Location:** `internal/jobbuilder/job.go:518`

```go
job := &batchv1.Job{
    Spec: batchv1.JobSpec{
        ActiveDeadlineSeconds: ptr(int64(900)),  // 15 minutes - HARDCODED
        TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds),  // Configurable
    },
}
```

**Current Behavior:**
- All jobs have 900s (15 minute) timeout
- No variation based on finding complexity
- No adjustment based on retry attempt
- Job TTL after completion is configurable (default 24h)

**Status:** ⚠️ PARTIALLY CONFIGURABLE - Only job TTL exists

---

### ✅ Job Backoff Limit

**Location:** `internal/jobbuilder/job.go:517`

```go
job := &batchv1.Job{
    Spec: batchv1.JobSpec{
        BackoffLimit: ptr(int32(1)),  // Controls retry behavior
    },
}
```

**Current Behavior:**
- Jobs can fail 1 time before marked failed
- Kubernetes will retry once with backoff
- After backoff, job will be marked failed
- Combined with RetryCount tracking in controller

**Status:** ✅ WORKING - Kubernetes backoff mechanism works

---

## What's Missing (Gaps to Address)

### Gap 1: Dynamic Timeout Configuration

**Problem:**
- All jobs have same 15-minute timeout
- Simple findings waste time if timeout is too long
- Complex findings may timeout if timeout is too short
- No way to adjust timeout per environment or workload

**Solution:** Make job timeout configurable with per-retry adjustment

**Approach:**
- Add configuration for base timeout (default: 900s = 15 minutes)
- Add configuration for timeout increment per retry (default: 300s = 5 minutes)
- Add configuration for maximum timeout ceiling (default: 3600s = 60 minutes)
- Implement timeout calculation in JobBuilder

**Implementation:**
```go
// internal/config/config.go

type Config struct {
    // Existing config...
    
    // Job timeout configuration
    AgentJobTimeoutSeconds int    // AGENT_JOB_TIMEOUT_SECONDS - default: 900
    AgentJobTimeoutIncrementSeconds int  // AGENT_JOB_TIMEOUT_INCREMENT_SECONDS - default: 300
    AgentJobMaxTimeoutSeconds int  // AGENT_JOB_MAX_TIMEOUT_SECONDS - default: 3600
}

// internal/jobbuilder/job.go

func CalculateTimeout(
    rjob *v1alpha1.RemediationJob,
    config TimeoutConfig,
    attempt int,
) time.Duration {
    timeout := config.BaseTimeout
    
    // Increase timeout based on retry attempt
    if attempt > 0 {
        increment := config.TimeoutIncrement
        additional := time.Duration(attempt) * increment
        timeout += additional
    }
    
    // Cap at maximum timeout
    if timeout > config.MaxTimeout {
        timeout = config.MaxTimeout
    }
    
    return timeout
}

func (b *Builder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error) {
    // Calculate retry attempt
    attempt := 0
    if rjob.Status.RetryCount > 0 {
        attempt = int(rjob.Status.RetryCount)
    }
    
    // Calculate timeout
    timeout := CalculateTimeout(rjob, b.cfg.TimeoutConfig, attempt)
    
    job := &batchv1.Job{
        Spec: batchv1.JobSpec{
            ActiveDeadlineSeconds: ptr(int64(timeout.Seconds())),
            TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds),
            // ... rest of spec
        },
    }
    
    return job, nil
}
```

**Files to Create:**
- `internal/config/config.go` - Add timeout configuration fields
- `internal/jobbuilder/timeout.go` (new) - timeout calculation logic
- `internal/jobbuilder/job.go` - Update to use dynamic timeout

**Status:** ✅ NEW FEATURE - To be implemented

---

### Gap 2: No Failure Classification

**Problem:**
- All failures treated identically
- RetryCount is tracked but RetryCount alone doesn't indicate failure type
- No distinction between transient (network, timeout) and permanent (auth, permission) errors
- Auth failures retried (wasteful)
- Timeout errors might not get extra time if they should

**Solution:** Add failure classification to guide retry decisions

**Approach:**
- Analyze Job status conditions to classify failure
- Map common failure reasons to retry decisions
- Skip retry for permanent failures (auth, permission)
- Adjust retry delay for different failure types

**Implementation:**
```go
// internal/failure/classifier.go

type FailureType string

const (
    FailureTransient   FailureType = "transient"
    FailurePermanent   FailureType = "permanent"
)

type Classifier struct {
    transientPatterns []string
    permanentPatterns []string
}

func NewClassifier() *Classifier {
    return &Classifier{
        transientPatterns: []string{
            "timeout",
            "deadline exceeded",
            "connection refused",
            "network unreachable",
            "rate limit exceeded",
            "temporary failure",
        },
        permanentPatterns: []string{
            "authentication failed",
            "permission denied",
            "unauthorized",
            "forbidden",
            "invalid configuration",
        },
    }
}

func (c *Classifier) Classify(reason, message string) (FailureType, bool) {
    // Check permanent patterns first (auth, permission errors)
    for _, pattern := range c.permanentPatterns {
        if strings.Contains(strings.ToLower(reason), pattern) ||
           strings.Contains(strings.ToLower(message), pattern) {
            return FailurePermanent, false // Don't retry
        }
    }
    
    // Check transient patterns (network, timeout)
    for _, pattern := range c.transientPatterns {
        if strings.Contains(strings.ToLower(reason), pattern) ||
           strings.Contains(strings.ToLower(message), pattern) {
            return FailureTransient, true // Retry with backoff
        }
    }
    
    // Default: unknown, treat as permanent
    return FailurePermanent, false
}
```

**Integration with Controller:**
```go
// internal/controller/remediationjob_controller.go

type RemediationJobReconciler struct {
    // Existing...
    FailureClassifier *failure.Classifier  // NEW
}

func (r *RemediationJobReconciler) handleFailedJob(
    ctx context.Context,
    rjob *remediationmechaniciov1alpha1.RemediationJob,
) (ctrl.Result, error) {
    failureType, shouldRetry := r.FailureClassifier.Classify(
        rjob.Status.Message,  // From Job conditions
        "",
    )
    
    if !shouldRetry {
        // Permanent failure: mark as PermanentlyFailed immediately
        rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
        apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
            Type:               v1alpha1.ConditionPermanentlyFailed,
            Status:             metav1.ConditionTrue,
            Reason:             "PermanentFailure",
            Message:            fmt.Sprintf("Classification: %s", failureType),
            LastTransitionTime: metav1.Now(),
        })
        return ctrl.Result{}, nil
    }
    
    // Transient failure: allow retry
    // Existing retry logic handles this
    return ctrl.Result{}, nil  // Controller will requeue
}
```

**Files to Create:**
- `internal/failure/classifier.go` (new) - failure type classification
- `internal/controller/remediationjob_controller.go` - Add FailureClassifier to struct
- Update `internal/controller/remediationjob_controller.go` - Use classification

**Status:** ✅ NEW FEATURE - To be implemented

---

### Gap 3: No Retry Backoff

**Problem:**
- Retries happen immediately (no delay)
- Immediate retries compound issues
- No time for infrastructure to recover
- Potential for retry storms

**Solution:** Implement exponential backoff with jitter for retries

**Approach:**
- Calculate backoff delay based on retry attempt
- Use exponential growth (60s, 120s, 240s, 480s)
- Add jitter (±10%) to avoid thundering herd
- Skip jitter on first retry (immediate)

**Implementation:**
```go
// internal/retry/backoff.go

type BackoffStrategy struct {
    BaseDelay       time.Duration
    MaxDelay        time.Duration
    Multiplier      float64
    MaxRetries      int
    Jitter          bool
}

func DefaultBackoffStrategy() *BackoffStrategy {
    return &BackoffStrategy{
        BaseDelay:       60 * time.Second,
        MaxDelay:        600 * time.Second,
        Multiplier:      2.0,
        MaxRetries:      5,
        Jitter:          true,
    }
}

func CalculateDelay(strategy *BackoffStrategy, attempt int) time.Duration {
    if attempt == 0 {
        return 0 // First retry: immediate
    }
    
    // Exponential backoff
    delay := float64(strategy.BaseDelay) * math.Pow(strategy.Multiplier, float64(attempt-1))
    
    // Cap at max delay
    if delay > float64(strategy.MaxDelay) {
        delay = float64(strategy.MaxDelay)
    }
    
    duration := time.Duration(delay)
    
    // Add jitter (±10%)
    if strategy.Jitter {
        jitter := duration * 0.1
        randomDuration := time.Duration(rand.Float64() * 2 * float64(jitter))
        // Subtract or add jitter
        if rand.Intn(2) == 0 {
            duration -= randomDuration
        } else {
            duration += randomDuration
        }
    }
    
    return duration
}

// Integration with controller
func (r *RemediationJobReconciler) handleFailedJob(
    ctx context.Context,
    rjob *remediationmechaniciov1alpha1.RemediationJob,
) (ctrl.Result, error) {
    // Calculate backoff delay
    delay := CalculateDelay(r.BackoffStrategy, int(rjob.Status.RetryCount))
    
    // Requeue with delay
    return ctrl.Result{RequeueAfter: delay}, nil
}
```

**Files to Create:**
- `internal/retry/backoff.go` (new) - backoff calculation
- `internal/config/config.go` - Add backoff configuration
- `internal/controller/remediationjob_controller.go` - Use backoff on retries

**Status:** ✅ NEW FEATURE - To be implemented

---

## What This Story Does NOT Do

- ❌ Replace existing retry logic (RetryCount works)
- ❌ Remove MaxRetries enforcement (already works)
- ❌ Change PhasePermanentlyFailed logic (already works)
- ❌ Implement retry from scratch (controller already retries via requeue)

---

## Files to Create or Modify

### New Files:
| File | Purpose |
|------|---------|
| `internal/failure/classifier.go` | Failure type classification |
| `internal/retry/backoff.go` | Backoff calculation |
| `internal/config/config.go` | Add timeout and backoff config |

### Modified Files:
| File | Change |
|------|--------|
| `internal/config/config.go` | Add timeout and backoff configuration fields |
| `internal/jobbuilder/job.go` | Update to use dynamic timeout |
| `internal/controller/remediationjob_controller.go` | Add FailureClassifier, use backoff |

---

## Acceptance Criteria

- [x] Verified existing retry logic works correctly
- [x] Verified Job timeout is partially configurable
- [x] Verified Job backoff mechanism works
- [x] Identified gaps: dynamic timeout, failure classification, retry backoff
- [x] Designed dynamic timeout calculation (non-breaking)
- [x] Designed failure classification (non-breaking)
- [x] Designed retry backoff (non-breaking)
- [x] All designs validated against project architecture
- [x] No incorrect assumptions about existing features
- [x] Story blocked until STORY_02 completes diagnosis
- [x] Implementation plans clear and ready after diagnosis
- [x] Worklog entry created

---

## Definition of Done

- [x] Existing retry behavior documented
- [x] Gaps identified and prioritized
- [x] New retry features designed
- [x] Implementation plans validated
- [x] No breaking changes to existing code
- [x] Ready for implementation after diagnosis
- [x] Worklog entry created

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|-------|----------|-------------|------------|
| Breaking existing retry logic | HIGH | LOW | Design as enhancement only |
| Incorrect classification | MEDIUM | LOW | Test against production data |
| Backoff too aggressive | MEDIUM | LOW | Make configurable |
| Timeout calculation errors | MEDIUM | LOW | Test thoroughly |
| Adding too many features | MEDIUM | LOW | Implement incrementally |

---

## Expected Outcomes

### Immediate Benefits (After Implementation):
- Transient failures automatically retry with backoff
- Permanent failures immediately marked as failed
- No wasted retries on auth/permission errors
- Infrastructure gets time to recover between retries
- Reduced retry storms with exponential backoff and jitter

### Long-term Benefits:
- Better resource utilization (no immediate retries)
- Higher success rate (transient errors get more attempts)
- Faster MTTR for transient issues
- Configurable behavior per environment
