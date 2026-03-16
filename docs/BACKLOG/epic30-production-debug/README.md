# Epic 30: Production Debug and Incident Recovery

## Status: PREVENTION AND MONITORING FOCUS

## ⚠️ IMPORTANT: EPIC DIRECTION CHANGED

This epic was originally designed to investigate a production incident through artifact collection and root cause analysis. However, **STORY_02 (artifact collection) is BLOCKED** because all diagnostic evidence was permanently lost to Kubernetes garbage collection ~5 days ago.

**NEW DIRECTION - OPTION A: Prevention and Monitoring**
1. Implement monitoring/alerting to catch next incident early
2. Improve garbage collection policies to preserve diagnostic evidence
3. Document evidence collection procedures for future incidents
4. Accept that root cause cannot be determined for this incident

**Before starting any work:**
1. Review [WORKLOGS/0092_2026-03-16_epic30-review.md](../../WORKLOGS/0092_2026-03-16_epic30-review.md)
2. Review [APPROACHES.md](../../APPROACHES.md) for all available options
3. Verify all proposed changes against actual codebase
4. Test all assumptions in a non-production environment
5. Focus on monitoring and evidence preservation, not reinventing existing functionality

---

## Incident Summary

**Date Identified:** 2026-03-15  
**Severity:** Critical  
**Impact:** Complete operational deadlock - no new investigations can run

### Current State

```
RemediationJobs by Phase:
- Dispatched: 3  (failed after 15min timeout, blocking queue)
- Pending:    58 (waiting indefinitely for MAX_CONCURRENT_JOBS=3 slots)
- Suppressed: 10 (intentionally blocked)
```

### Blocked Operations

- **Watcher Running:** Healthy (1/1 pods, 13 days uptime)
- **Finding Detection:** Working (detecting Deployments, StatefulSets, Pods, Nodes, Jobs)
- **Deduplication:** Working (parent hierarchy suppression active)
- **Circuit Breaker:** Working (self-remediation cascade prevention active)
- **Job Dispatch:** **BLOCKED** - 3 failed jobs occupying all 3 concurrent slots

---

## Problem Statement

The K8s mechanic system is in a **complete operational deadlock**. The agent investigation jobs are failing with a 15-minute timeout (900s activeDeadlineSeconds), but the **root cause is UNKNOWN** and needs to be diagnosed before implementing fixes.

---

## Dependencies

- epic01-controller complete (RemediationJobReconciler)
- epic02-jobbuilder complete (Job construction)
- epic04-deploy complete (manifests and RBAC)

---

## Success Criteria

### Monitoring and Alerting (2 weeks)
- [ ] Prometheus metrics implemented for RemediationJob lifecycle
- [ ] Alerting rules configured for queue blockages and failures
- [ ] Dashboard/grafana panels created for monitoring
- [ ] Alert latency < 5 minutes for critical issues

### Evidence Preservation (1 week)
- [ ] TTL increased for failed jobs to preserve evidence
- [ ] Pod termination grace period configured
- [ ] Log retention policies documented
- [ ] Evidence collection procedures documented

### Documentation (3 days)
- [ ] Evidence collection runbook created
- [ ] Garbage collection policies documented
- [ ] Post-incident review completed
- [ ] All findings documented in worklog

---


## Stories

| Story | File | Title | Status | Priority | Effort | Dependencies |
|-------|------|-------|--------|----------|-------------|
| Failed job artifact collection and analysis | [STORY_02_failure_artifacts.md](STORY_02_failure_artifacts.md) | **BLOCKED** | Critical | 2h | None | **PERMANENTLY BLOCKED - Evidence Lost** |
| Job lifecycle management improvements | [STORY_04_lifecycle_management.md](STORY_04_lifecycle_management.md) | Not Started | Medium | 1h | None | Documentation only |
| Automatic cleanup and recovery mechanism | [STORY_05_auto_cleanup.md](STORY_05_auto_cleanup.md) | Not Started | Medium | 1h | None | Documentation only |
| Enhanced observability and debugging | [STORY_06_observability.md](STORY_06_observability.md) | Not Started | Medium | 2h | None | Documentation only |
| Improved timeout and retry strategies | [STORY_07_timeout_retry.md](STORY_07_timeout_retry.md) | Not Started | Low | 1h | None | Documentation only |
| Configuration tuning and defaults | [STORY_08_config_tuning.md](STORY_08_config_tuning.md) | Not Started | Low | 1h | None | Documentation only |
| **NEW: Prometheus metrics implementation** | [STORY_09_prometheus_metrics.md](STORY_09_prometheus_metrics.md) | Not Started | **Critical** | 4h | None | **New - Monitoring** |
| **NEW: Alerting rules configuration** | [STORY_10_alerting_rules.md](STORY_10_alerting_rules.md) | Not Started | **Critical** | 3h | STORY_09 | **New - Monitoring** |
| **NEW: Garbage collection policy improvements** | [STORY_11_gc_policy_improvements.md](STORY_11_gc_policy_improvements.md) | Not Started | **High** | 2h | None | **New - Evidence Preservation** |
| **NEW: Evidence collection documentation** | [STORY_12_evidence_collection.md](STORY_12_evidence_collection.md) | Not Started | **Medium** | 2h | None | **New - Documentation** |

---

## Story Execution Order

```
STORY_09 (Prometheus metrics) → STORY_10 (Alerting rules) → STORY_11 (GC policy) → STORY_12 (Evidence collection docs)
                                                       ↓
STORY_04 (lifecycle) → STORY_05 (auto-cleanup) → STORY_06 (observability) → STORY_07 (timeout/retry) → STORY_08 (config)
```

**Critical Path:** STORY_09 → STORY_10 (monitoring and alerting)

**Parallel Path:**
- STORY_11 (GC policy) - can run in parallel
- STORY_12 (evidence docs) - can run in parallel
- STORY_04-08 (documentation) - can run after STORY_09 completes

**Blocked:**
- STORY_02: PERMANENTLY BLOCKED - Evidence lost to garbage collection

---

## Technical Design

### Current Architecture Gaps

**Gap 1: RemediationJob Controller Does Not Watch Jobs**
- Controller only watches RemediationJob CRDs
- No reconciliation loop for batch/v1 Job status
- Cannot detect Job failures or cleanup after completion

**Gap 2: No Failure Analysis Pipeline**
- Failed jobs leave no artifacts
- No post-mortem data collection
- Logs and snapshots lost on pod termination

**Gap 3: Insufficient Error Classification**
- All failures treated identically
- No distinction between transient and permanent errors
- No classification by failure type (timeout, auth, LLM, etc.)

**Gap 4: No Automatic Recovery**
- Stuck Dispatched phase never resolves
- No automatic retry or cleanup
- Manual intervention required for recovery

### Proposed Solution Architecture

#### 1. Enhanced Job Lifecycle Management

**Add Job Watcher to RemediationJob Controller:**
```go
// internal/controller/remediationjob_controller.go

func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Existing RemediationJob watch
    ctrl.NewControllerManagedBy(mgr).
        For(&remediationmechaniciov1alpha1.RemediationJob{}).
        Owns(&batchv1.Job{}).  // ← NEW: Watch owned Jobs
        Complete(r)
}
```

**Job Status Reconciliation:**
```go
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get RemediationJob
    rjob := &remediationmechaniciov1alpha1.RemediationJob{}
    if err := r.Get(ctx, req.NamespacedName, rjob); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // If Job exists, check status
    if rjob.Status.JobRef != "" {
        job := &batchv1.Job{}
        jobKey := types.NamespacedName{
            Namespace: rjob.Namespace,
            Name:      rjob.Status.JobRef,
        }
        
        if err := r.Get(ctx, jobKey, job); err == nil {
            // Job exists - check status
            if job.Status.Failed > 0 {
                // Job failed - mark RemediationJob
                rjob.Status.Phase = "Failed"
                rjob.Status.FailureReason = getFailureReason(job)
                r.Status().Update(ctx, rjob)
                
                // Schedule cleanup based on TTL
                return ctrl.Result{}, nil
            }
            
            if job.Status.Succeeded > 0 {
                // Job succeeded - mark RemediationJob
                rjob.Status.Phase = "Completed"
                r.Status().Update(ctx, rjob)
                
                // Schedule cleanup
                return ctrl.Result{}, nil
            }
        } else if !errors.IsNotFound(err) {
            // Real error
            return ctrl.Result{}, err
        }
        // Job not found - may have been cleaned up
    }
    
    // Existing logic...
}
```

#### 2. Failure Analysis and Artifact Collection

**New CRD: FailureAnalysis**
```yaml
apiVersion: remediation.mechanic.io/v1alpha1
kind: FailureAnalysis
metadata:
  name: mechanic-fa-<timestamp>
  namespace: <agent-namespace>
spec:
  remediationJobRef: <remediation-job-name>
  failureType: timeout | auth | llm | network | resource | permission
  jobStatus: Failed | Succeeded | DeadlineExceeded
  startTime: <RFC3339>
  endTime: <RFC3339>
  durationSeconds: <int>
  artifactRefs:
    - type: PodLogs
      name: mechanic-fa-<timestamp>-logs
    - type: PodEvents
      name: mechanic-fa-<timestamp>-events
    - type: JobSpec
      name: mechanic-fa-<timestamp>-jobspec
    - type: AgentConfig
      name: mechanic-fa-<timestamp>-config
status:
  rootCause: <string>
  classification: transient | permanent | configuration
  recommendations: <string[]>
```

**Artifact Storage Strategy:**
- **Logs:** Store in ConfigMap (size < 1MB) or PVC (size > 1MB)
- **Events:** Store as annotations on FailureAnalysis CRD
- **Job Spec:** Store as annotation (small YAML)
- **Agent Config:** Redact sensitive values, store as annotation

#### 3. Enhanced Retry Logic

**Exponential Backoff:**
```go
type RetryStrategy struct {
    MaxRetries      int
    BaseDelay       time.Duration
    MaxDelay        time.Duration
    Multiplier      float64
    Jitter          bool
}

func (r *RetryStrategy) ShouldRetry(rjob *RemediationJob, attempts int) bool {
    if attempts >= r.MaxRetries {
        return false
    }
    
    // Check failure type
    if rjob.Status.FailureReason == "auth" {
        // Auth failures - no retry
        return false
    }
    
    if rjob.Status.FailureReason == "timeout" {
        // Timeout - retry with longer timeout
        return attempts < 2
    }
    
    // Transient errors - retry with backoff
    return true
}
```

**Dynamic Timeout Adjustment:**
```go
func calculateTimeout(attempt int) time.Duration {
    // Base timeout: 15 minutes
    base := 15 * time.Minute
    
    // Add 5 minutes per retry attempt
    additional := time.Duration(attempt) * 5 * time.Minute
    
    return base + additional
}
```

#### 4. Observability and Metrics

**New Prometheus Metrics:**
```go
// RemediationJob lifecycle
var (
    remediationJobsCreated = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_created_total",
            Help: "Total number of RemediationJobs created",
        },
        []string{"tier", "severity"},
    )
    
    remediationJobsCompleted = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_completed_total",
            Help: "Total number of RemediationJobs completed successfully",
        },
        []string{"duration_seconds"},
    )
    
    remediationJobsFailed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_failed_total",
            Help: "Total number of RemediationJobs failed",
        },
        []string{"failure_reason", "attempts"},
    )
    
    jobDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_job_duration_seconds",
            Help:    "Duration of agent job execution",
            Buckets: prometheus.DefBuckets,
        },
        []string{"finding_kind", "agent_type"},
    )
    
    queueDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mechanic_queue_depth",
            Help: "Number of pending RemediationJobs",
        },
        []string{"phase"},
    )
)
```

**Alerting Rules:**
```yaml
groups:
  - name: mechanic
    rules:
      - alert: MechanicQueueBlocked
        expr: mechanic_queue_depth{phase="Dispatched"} >= MAX_CONCURRENT_JOBS
        for: 15m
        labels:
          severity: critical
        annotations:
          summary: "Mechanic queue is blocked by failed jobs"
          
      - alert: MechanicJobTimeoutHigh
        expr: rate(mechanic_remediation_jobs_failed_total{failure_reason="timeout"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High rate of agent job timeouts"
```

---

## Configuration Changes

### Emergency (Immediate)
```bash
# Reduce TTL for failed jobs (emergency only)
REMEDIATION_JOB_TTL_SECONDS=3600  # 1 hour instead of 7 days

# Increase concurrent jobs (emergency only)
MAX_CONCURRENT_JOBS=6  # Allow more slots while investigating
```

### Short-term (After Investigation)
```bash
# Job retry configuration
MAX_INVESTIGATION_RETRIES=3
RETRY_BASE_DELAY_SECONDS=60
RETRY_MAX_DELAY_SECONDS=600

# Job timeout configuration
AGENT_JOB_TIMEOUT_SECONDS=900
AGENT_JOB_TIMEOUT_INCREMENT_SECONDS=300

# Artifact retention
PRESERVE_FAILURE_ARTIFACTS=true
ARTIFACT_RETENTION_HOURS=24
```

---

## Definition of Done

### Monitoring and Alerting
- [ ] Prometheus metrics implemented in controller
- [ ] Metrics exposed on /metrics endpoint
- [ ] Alerting rules configured for queue blockages
- [ ] Alerting rules configured for high failure rates
- [ ] Grafana dashboard panels created
- [ ] Alerts tested and verified working
- [ ] Alert latency < 5 minutes

### Evidence Preservation
- [ ] TTL increased for failed jobs (7+ days)
- [ ] Pod termination grace period configured (30s)
- [ ] Log retention policies documented
- [ ] Garbage collection policies documented
- [ ] Policies tested in non-production

### Documentation
- [ ] Evidence collection runbook created
- [ ] Post-incident review completed
- [ ] Worklog entry created
- [ ] All STORY_04-08 documentation completed

### Quality
- [ ] All tests pass: `go test -timeout 30s -race ./...`
- [ ] Code review completed
- [ ] Changes deployed to production

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Monitoring/Alerting implementation is insufficient | High | Medium | Test alerts with simulated failures |
| Alert fatigue from too many alerts | Medium | Medium | Use conservative alerting thresholds |
| Increased TTL causes resource bloat | Medium | Medium | Implement resource quotas |
| Evidence collection not followed in next incident | High | Medium | Create clear runbook and training |
| Root cause remains unknown | High | **100%** | **ACCEPTED - Cannot be determined** |

---

## Expected Outcomes

| Improvement | Metric | Target | Timeline |
|------------|---------|--------|-----------|
| Alert latency | Time to detect queue blockages | < 5 minutes | 2 weeks |
| Evidence preservation | TTL for failed jobs | 7+ days | 1 week |
| Alert effectiveness | False positive rate | < 10% | 2 weeks |
| Response time | Time from alert to action | < 15 minutes | 2 weeks |
| Evidence collection compliance | Runbook followed in next incident | 100% | Ongoing |
| Root cause identified | Diagnostic confidence | **N/A - Evidence lost** | N/A |

---

## Post-Incident Review Questions

1. Why were failed jobs not detected automatically? **(Answered: Controller watches Jobs, but no alerts configured)**
2. Why was TTL set to 7 days instead of something more reasonable? **(Answered: Default TTL too short for evidence collection)**
3. Why is there no automatic cleanup mechanism? **(Answered: TTL cleanup exists, but too short)**
4. Why were no alerts triggered for the queue blockage? **(Answered: No monitoring/alerting implemented)**
5. What monitoring gaps allowed this to go undetected for ~5 days? **(Answered: No Prometheus metrics or alerting)**
6. What changes to SRE procedures are needed? **(Answered: Evidence collection procedures being created)**
7. What runbook improvements are required? **(Answered: Monitoring and evidence collection runbooks being created)**
8. What automated tests would have caught this? **(Answered: Alerting tests being implemented)**
9. **NEW: How can we prevent evidence loss in future incidents?** (Answered: STORY_11)
10. **NEW: How can we catch queue blockages faster?** (Answered: STORY_09 + STORY_10)
11. **NEW: What evidence collection procedures are needed?** (Answered: STORY_12)
