# Story 05: Automatic Cleanup and Recovery Mechanism

**Epic:** [epic30-production-debug](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 3 hours

---

## User Story

As a **system operator**, I want the system to **automatically clean up failed investigations and recover from deadlocks** so that **manual intervention is never required**.

---

## Current Problems

### Problem 1: No Automatic Job Cleanup

**Situation:**
- Failed Jobs persist for 7 days (REMEDIATION_JOB_TTL_SECONDS=604800)
- Even after RemediationJob CRD is cleaned up by TTL, Jobs may remain
- Manual cleanup required (as done in STORY_01)

**Why:**
- RemediationJob controller has cleanup for CRD TTL but not for Jobs
- **NO CHANGE NEEDED** - Job cleanup already handled by existing mechanisms:
  - Jobs are owned by RemediationJob via ownerReference (set by JobBuilder)
  - Kubernetes garbage collector cleans up orphaned Jobs when owner is deleted
  - Jobs have TTLSecondsAfterFinished (default 86400s = 24h)

**Status:** ✅ Already implemented in production code

**Action Required:** None
- No finalizer cascade to clean up child resources

### Problem 2: No Deadlock Recovery

**Situation:**
- If MAX_CONCURRENT_JOBS slots fill with failed Jobs, queue blocks
- No automatic mechanism to detect and unblock deadlocks
- System stays deadlocked until manual intervention

**Why:**
- No alerting on queue blockage
- No automatic cleanup of failed Jobs blocking queue
- No circuit breaker for stuck Dispatched phase

### Problem 3: No Failure Artifact Collection

**Situation:**
- When Jobs fail, logs and events are lost
- No post-mortem data for debugging
- Hard to diagnose root cause (as seen in STORY_03)

**Why:**
- No FailureAnalysis CRD (proposed in STORY_02)
- No artifact collection before Job deletion
- Logs not preserved after pod termination

---

## Proposed Solution

### 1. Finalizer Cascade for Job Cleanup

**Change:** `internal/controller/remediationjob_controller.go`

```go
const (
    RemediationJobFinalizer = "remediation.mechanic.io/cleanup"
)

func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // Get RemediationJob
    rjob := &remediationmechaniciov1alpha1.RemediationJob{}
    if err := r.Get(ctx, req.NamespacedName, rjob); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Handle deletion
    if !rjob.DeletionTimestamp.IsZero() {
        // Object is being deleted - clean up owned Job
        return r.handleDeletion(ctx, rjob)
    }
    
    // Add finalizer if not present
    if !containsString(rjob.Finalizers, RemediationJobFinalizer) {
        rjob.Finalizers = append(rjob.Finalizers, RemediationJobFinalizer)
        return ctrl.Result{}, r.Update(ctx, rjob)
    }
    
    // Rest of reconciliation logic...
    return ctrl.Result{}, nil
}

func (r *RemediationJobReconciler) handleDeletion(
    ctx context.Context,
    rjob *remediationmechaniciov1alpha1.RemediationJob,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // Clean up owned Job if exists
    if rjob.Status.JobRef != "" {
        job := &batchv1.Job{
            ObjectMeta: metav1.ObjectMeta{
                Name:      rjob.Status.JobRef,
                Namespace: rjob.Namespace,
            },
        }
        
        if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
            log.Error(err, "failed to delete job during remediationjob deletion", "job", rjob.Status.JobRef)
            return ctrl.Result{}, err
        }
        
        log.Info("Cleaned up owned job during deletion", "job", rjob.Status.JobRef)
    }
    
    // Remove finalizer
    rjob.Finalizers = removeString(rjob.Finalizers, RemediationJobFinalizer)
    return ctrl.Result{}, r.Update(ctx, rjob)
}

func containsString(slice []string, s string) bool {
    for _, item := range slice {
        if item == s {
            return true
        }
    }
    return false
}

func removeString(slice []string, s string) []string {
    result := []string{}
    for _, item := range slice {
        if item != s {
            result = append(result, item)
        }
    }
    return result
}
```

---

### 2. Failed Job Queue Detector

**Change:** `internal/controller/remediationjob_controller.go`

```go
// Queue depth tracking
type QueueDepthTracker struct {
    mutex sync.RWMutex
    depthByPhase map[string]int
}

func NewQueueDepthTracker() *QueueDepthTracker {
    return &QueueDepthTracker{
        depthByPhase: make(map[string]int),
    }
}

func (q *QueueDepthTracker) Update(phase RemediationJobPhase) {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    q.depthByPhase[string(phase)]++
}

func (q *QueueDepthTracker) Remove(phase RemediationJobPhase) {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    count := q.depthByPhase[string(phase)]
    if count > 0 {
        q.depthByPhase[string(phase)] = count - 1
    }
}

func (q *QueueDepthTracker) GetPhaseDepth(phase RemediationJobPhase) int {
    q.mutex.RLock()
    defer q.mutex.RUnlock()
    return q.depthByPhase[string(phase)]
}

// Add to reconciler
type RemediationJobReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    QueueTracker *QueueDepthTracker
    MaxConcurrentJobs int
}

// In Reconcile
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // Get RemediationJob
    rjob := &remediationmechaniciov1alpha1.RemediationJob{}
    if err := r.Get(ctx, req.NamespacedName, rjob); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    oldPhase := rjob.Status.Phase
    
    // ... reconciliation logic ...
    
    // Update queue tracker
    if oldPhase != rjob.Status.Phase {
        r.QueueTracker.Remove(oldPhase)
        r.QueueTracker.Update(rjob.Status.Phase)
    }
    
    // Detect queue blockage
    if r.detectQueueBlockage() {
        log.Error(nil, "Queue blockage detected", "dispatched", r.QueueTracker.GetPhaseDepth("Dispatched"))
        r.alertQueueBlockage()
    }
    
    return ctrl.Result{}, nil
}

func (r *RemediationJobReconciler) detectQueueBlockage() bool {
    dispatched := r.QueueTracker.GetPhaseDepth("Dispatched")
    max := r.MaxConcurrentJobs
    
    // Blockage detected if all concurrent slots are occupied by Dispatched jobs
    // and none have transitioned in last 30 minutes
    return dispatched >= max
}

func (r *RemediationJobReconciler) alertQueueBlockage() {
    // Create Kubernetes Event
    event := &corev1.Event{
        ObjectMeta: metav1.ObjectMeta{
            GenerateName: "mechanic-queue-blockage-",
            Namespace:    r.namespace,
        },
        InvolvedObject: &corev1.ObjectReference{
            Kind:      "RemediationJob",
            Namespace: r.namespace,
        },
        Type:    "Warning",
        Reason:  "QueueBlockage",
        Message: "All MAX_CONCURRENT_JOBS slots occupied by Dispatched jobs - queue is blocked",
        Source: corev1.EventSource{
            Component: "mechanic-watcher",
        },
    }
    
    if err := r.Create(context.TODO(), event); err != nil {
        log.Error(err, "failed to create queue blockage event")
    }
    
    // TODO: STORY_06 will add Prometheus alert
}
```

---

### 3. Stuck Dispatched Job Recovery

**Change:** `internal/controller/remediationjob_controller.go`

```go
const (
    StuckDispatchedThreshold = 30 * time.Minute // 30 minutes
)

func (r *RemediationJobReconciler) reconcileJobStatus(
    ctx context.Context,
    rjob *remediationmechaniciov1alpha1.RemediationJob,
    job *batchv1.Job,
) (bool, error) {
    updated := false
    
    // Check for stuck Dispatched job
    if rjob.Status.Phase == "Dispatched" && rjob.Status.DispatchedAt != nil {
        dispatchedAt := rjob.Status.DispatchedAt.Time
        stuckDuration := time.Since(dispatchedAt)
        
        // Job should be active but check if it's stuck
        jobActive := isJobActive(job)
        
        if !jobActive && stuckDuration > StuckDispatchedThreshold {
            // Job appears stuck - force failure
            log.Error(nil, "Detected stuck Dispatched job", "job", rjob.Status.JobRef, "stuck_duration", stuckDuration)
            
            rjob.Status.Phase = "Failed"
            rjob.Status.FailureReason = "StuckDispatched"
            rjob.Status.FailureMessage = fmt.Sprintf("Job stuck in Dispatched phase for %v", stuckDuration)
            rjob.Status.FailedAt = &metav1.Time{Time: time.Now()}
            
            // Trigger Job cleanup
            if err := r.cleanupStuckJob(ctx, rjob.Status.JobRef); err != nil {
                return false, err
            }
            
            return true, nil
        }
    }
    
    // ... rest of existing status reconciliation logic ...
    return updated, nil
}

func isJobActive(job *batchv1.Job) bool {
    if job == nil {
        return false
    }
    
    // Check if job has any active pods
    if job.Status.Active > 0 {
        return true
    }
    
    // Check if job is running but hasn't completed
    return job.Status.Succeeded == 0 && job.Status.Failed == 0
}

func (r *RemediationJobReconciler) cleanupStuckJob(
    ctx context.Context,
    jobName string,
) error {
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      jobName,
            Namespace: r.namespace,
        },
    }
    
    // Force delete stuck Job
    if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
        return err
    }
    
    log.Info("Cleaned up stuck Dispatched job", "job", jobName)
    return nil
}
```

---

### 4. Enhanced Metrics for Queue Health

**Change:** `internal/metrics/controller_metrics.go` (or create)

```go
var (
    // Queue depth metrics
    queueDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mechanic_queue_depth",
            Help: "Number of RemediationJobs in each phase",
        },
        []string{"phase"},
    )
    
    // Queue blockage detection
    queueBlockageDetected = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "mechanic_queue_blockage_detected_total",
            Help: "Number of times queue blockage was detected",
        },
    )
    
    // Stuck job detection
    stuckJobDetected = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_stuck_job_detected_total",
            Help: "Number of stuck jobs detected and cleaned up",
        },
        []string{"phase", "reason"},
    )
)
```

**Register in main.go:**
```go
func main() {
    // ... setup ...
    
    // Register metrics
    metrics.MustRegister(queueDepth)
    metrics.MustRegister(queueBlockageDetected)
    metrics.MustRegister(stuckJobDetected)
    
    // Expose metrics endpoint
    http.Handle("/metrics", promhttp.Handler())
    go func() {
        log.Error(http.ListenAndServe(":8080", nil), "metrics server error")
    }()
}
```

---

## Acceptance Criteria

### Finalizer Cleanup
- [ ] RemediationJob has finalizer added on creation
- [ ] Finalizer removed after Job cleanup
- [ ] Jobs deleted when RemediationJob is deleted
- [ ] Unit tests for finalizer lifecycle

### Queue Blockage Detection
- [ ] Queue depth tracking implemented
- [ ] Blockage detection triggers alert
- [ ] Kubernetes Event created for blockage
- [ ] Metrics exposed for queue depth

### Stuck Job Recovery
- [ ] Stuck Dispatched jobs detected after 30 minutes
- [ ] Stuck jobs marked as Failed with reason
- [ ] Stuck jobs automatically cleaned up
- [ ] Unit tests for stuck detection logic

### Metrics
- [ ] Queue depth gauge exposed
- [ ] Queue blockage counter exposed
- [ ] Stuck job counter exposed
- [ ] Metrics accessible at :8080/metrics

### Integration
- [ ] All new code tested with envtest
- [ ] Finalizer cascade works end-to-end
- [ ] Queue blockage alert tested

---

## What This Story Does NOT Do

- Implement retry logic (that's STORY_07)
- Add detailed failure analysis CRD (that's future work)
- Implement Prometheus alerting rules (that's STORY_06)

---

## Files to Modify

| File | Change |
|------|--------|
| `internal/controller/remediationjob_controller.go` | Add finalizer, queue tracking, stuck detection |
| `api/v1alpha1/remediationjob_types.go` | No changes needed (finalizer is standard k8s field) |
| `internal/metrics/controller_metrics.go` | Create or enhance with queue metrics |
| `cmd/watcher/main.go` | Initialize QueueTracker, register metrics |
| `internal/controller/remediationjob_controller_test.go` | Add unit tests |

---

## Testing Requirements

### Unit Tests

**Test 1: Finalizer Added on Creation**
```go
func TestFinalizer_AddedOnCreation(t *testing.T) {
    // Given - Create RemediationJob
    rjob := &remediationmechaniciov1alpha1.RemediationJob{
        // ... minimal spec
    }
    expect.NoError(t, k8sClient.Create(ctx, rjob))
    
    // Reconcile
    result, err := reconciler.Reconcile(ctx, request)
    
    // Then
    expect.NoError(t, err)
    expect.True(t, result.Requeue)
    
    updatedRjob := &remediationmechaniciov1alpha1.RemediationJob{}
    expect.NoError(t, k8sClient.Get(ctx, key, updatedRjob))
    expect.True(t, containsString(updatedRjob.Finalizers, RemediationJobFinalizer))
}
```

**Test 2: Job Cleanup on Deletion**
```go
func TestFinalizer_JobCleanup(t *testing.T) {
    // Given - Create RemediationJob with Job
    rjob := createTestRemediationJob(t)
    job := createTestJob(t, rjob)
    
    // When - Delete RemediationJob
    expect.NoError(t, k8sClient.Delete(ctx, rjob))
    
    // Reconcile - should handle deletion
    reconciler.Reconcile(ctx, request)
    
    // Then - Job should be deleted
    foundJob := &batchv1.Job{}
    err := k8sClient.Get(ctx, jobKey, foundJob)
    expect.True(t, errors.IsNotFound(err))
    
    // And finalizer removed
    updatedRjob := &remediationmechaniciov1alpha1.RemediationJob{}
    expect.NoError(t, k8sClient.Get(ctx, key, updatedRjob))
    expect.False(t, containsString(updatedRjob.Finalizers, RemediationJobFinalizer))
}
```

**Test 3: Stuck Job Detection**
```go
func TestStuckJobDetection(t *testing.T) {
    // Given - RemediationJob stuck in Dispatched for > 30 min
    dispatchedAt := metav1.Now().Add(-35 * time.Minute)
    rjob := &remediationmechaniciov1alpha1.RemediationJob{
        Status: remediationmechaniciov1alpha1.RemediationJobStatus{
            Phase:       "Dispatched",
            DispatchedAt: &dispatchedAt,
            JobRef:       "stuck-job",
        },
    }
    // Job is not active (no pods)
    job := &batchv1.Job{
        Status: batchv1.JobStatus{
            Active: 0,
        },
    }
    
    // When - Reconcile job status
    updated, _ := reconciler.reconcileJobStatus(ctx, rjob, job)
    
    // Then - Should be marked failed
    expect.True(t, updated)
    expect.Equal(t, "Failed", rjob.Status.Phase)
    expect.Equal(t, "StuckDispatched", rjob.Status.FailureReason)
}
```

---

## Definition of Done

- [ ] Finalizer cascade implemented and tested
- [ ] Queue blockage detection implemented
- [ ] Stuck job recovery implemented
- [ ] Queue metrics exposed
- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `go vet` clean
- [ ] `go build ./...` clean

---

## Success Indicators

- **No Manual Cleanup:** Failed Jobs cleaned up automatically
- **No Deadlocks:** Queue blockage detected and recovered
- **Visibility:** Queue depth metrics provide operational insight
- **Alerting:** Kubernetes Events notify on blockages
- **Test Coverage:** > 80% coverage of new code

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Finalizer prevents deletion | High | Low | Test deletion path thoroughly |
| Stuck job detection too aggressive | Medium | Low | 30-minute threshold provides buffer |
| Queue blockage detection false positive | Low | Medium | Require MAX_CONCURRENT_JOBS full + time threshold |
| Finalizer cleanup fails | High | Low | Use Background propagation, ignore NotFound errors |