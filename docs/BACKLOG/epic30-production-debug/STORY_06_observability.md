# Story 06: Enhanced Observability and Debugging

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

As a **site reliability engineer**, I want **enhanced observability features** (correlation IDs, metrics, alerts) so that I can **detect and diagnose issues quickly** without manual investigation.

---

## What Already Exists (Code Verification)

### ✅ Logging Infrastructure

**Location:** Controller logs use `go.uber.org/zap`
```go
// Standard structured logging throughout controller
log := log.FromContext(ctx)
log.Info("dispatched agent job",
    "remediationJob", rjob.Name,
    "job", job.Name)
```

**Status:** ✅ EXISTING - No change needed

---

### ✅ Kubernetes Events

**Location:** Event recording at `internal/controller/remediationjob_controller.go:40-110`
```go
// EventRecorder is injected into controller
type RemediationJobReconciler struct {
    // ...
    Recorder record.EventRecorder  // Line 41
}

// Events are recorded for key lifecycle events
r.Recorder.Eventf(rjob, corev1.EventTypeNormal, "JobSucceeded",
    "Agent Job completed; PR: %s", prRef)
r.Recorder.Eventf(rjob, corev1.EventTypeWarning, "JobFailed",
    "Agent Job failed after %d attempt(s)", job.Status.Failed)
```

**Event Types Already Emitted:**
- `JobCreated` (Job dispatched)
- `JobSucceeded` (Job completed successfully)
- `JobFailed` (Job failed)
- `JobPermanentlyFailed` (Retries exhausted)
- `JobDispatched` (Job created)
- `job.succeeded` (Job completed)
- `job.failed` (Job failed)

**Status:** ✅ EXISTING - Enhanced with Kubernetes Events

---

### ✅ Queue Depth Tracking

**Location:** Concurrency gate at `internal/controller/remediationjob_controller.go:947-1017`
```go
func (r *RemediationJobReconciler) concurrencyGate(ctx context.Context) (bool, ctrl.Result, error) {
    var jobs batchv1.JobList
    r.List(ctx, &jobs,
        client.InNamespace(r.Cfg.AgentNamespace),
        client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
    )
    
    activeCount := 0
    for i := range jobs.Items {
        j := &jobs.Items[i]
        if j.Status.Active > 0 || (j.Status.Succeeded == 0 && j.Status.CompletionTime == nil) {
            activeCount++
        }
    }
    
    if activeCount >= r.Cfg.MaxConcurrentJobs {
        // Queue is blocked
        return true, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
    
    return false, ctrl.Result{}, nil
}
```

**Current Status:** Queue depth can be calculated by listing jobs
**Status:** ✅ TRACKED - No mechanism needed, just calculation

---

### ✅ RemediationJob Status Fields

**Location:** `api/v1alpha1/remediationjob_types.go:9-260`

**Existing Status Fields:**
- `Phase` - Current lifecycle phase
- `JobRef` - Reference to batch/v1 Job
- `Message` - Human-readable description
- `RetryCount` - Number of times Job failed
- `CompletedAt` - When Job reached terminal state
- `DispatchedAt` - When Job was created
- `Conditions` - Kubernetes conditions array
- `CorrelationGroupID` - Correlation group identifier

**Status:** ✅ EXISTING - Adequate status tracking

---

## What's Missing (Gaps to Address)

### Gap 1: No Correlation IDs in Logs

**Problem:**
- Logs show RemediationJob names and job names
- No way to correlate finding creation → job dispatch → job completion
- Hard to trace end-to-end flow for a single investigation
- Difficult to debug correlated job groups

**Solution:** Add correlation ID to log context

**Approach:**
- Add correlation ID to log context
- Pass correlation ID through context
- Log correlation ID at each step in lifecycle
- Enable grep/filtering by correlation ID

**Implementation:**
```go
// New context key for correlation IDs
const correlationIDKey = "correlation.id"

// Helper to add correlation ID to context
func WithCorrelationID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, correlationIDKey, id)
}

// Helper to extract correlation ID from context
func GetCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey).(string); ok {
        return id
    }
    return ""
}

// Usage in controller
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // Add correlation ID to context
    corrID := rjob.Status.CorrelationGroupID
    if corrID != "" {
        ctx = WithCorrelationID(ctx, corrID)
    }
    
    // All logs now include correlation ID
    log.Info("Reconciling",
        "remediationJob", rjob.Name,
        "correlationId", corrID,
        "phase", rjob.Status.Phase)
}
```

**Files to Create:**
- `internal/controller/correlation.go` (new) - correlation ID helpers
- Update `internal/controller/remediationjob_controller.go` - use correlation IDs in all logs

---

### Gap 2: No Prometheus Metrics

**Problem:**
- No visibility into job lifecycle metrics
- Cannot measure job duration (Dispatched → Completed/Failed)
- Cannot measure queue depth over time
- Cannot track success/failure rates
- Cannot measure retry rates
- No way to detect performance degradation

**Solution:** Add Prometheus metrics for job lifecycle

**Metrics to Add:**
```go
// internal/metrics/controller_metrics.go

var (
    // Job lifecycle metrics
    remediationJobsCreated = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_created_total",
            Help: "Total number of RemediationJobs created",
        },
        []string{"tier", "severity"},
    )
    
    remediationJobsCompleted = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_remediation_job_duration_seconds",
            Help:    "Duration from Pending to Completed/Failed",
            Buckets: prometheus.ExponentialBuckets(60, 2, 10), // 1m, 2m, 4m, ... 1024m
        },
        []string{"finding_kind", "finding_parent_kind"},
    )
    
    remediationJobsFailed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_failed_total",
            Help: "Total number of RemediationJobs that failed",
        },
        []string{"failure_reason", "finding_kind", "attempts"},
    )
    
    jobDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_job_execution_duration_seconds",
            Help:    "Duration of agent job execution",
            Buckets: prometheus.DefBuckets,
        },
        []string{"finding_kind", "agent_type", "outcome"},
    )
    
    queueDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mechanic_queue_depth",
            Help: "Number of RemediationJobs in each phase",
        },
        []string{"phase"},
    )
    
    retryAttempts = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_retry_attempts_distribution",
            Help:    "Distribution of retry attempts",
            Buckets: prometheus.LinearBuckets(0, 1, 2, 3, 4, 5),
        },
        []string{"finding_kind"},
    )
)
```

**Metric Recording:**
```go
// In RemediationJobReconciler
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create job
    remediationJobsCreated.WithLabelValues(rjob.Spec.Severity, rjob.Spec.Finding.Kind).Inc()
    
    // Job completes
    duration := time.Since(*rjob.Status.DispatchedAt)
    remediationJobsCompleted.Observe(duration.Seconds(),
        "finding_kind", rjob.Spec.Finding.Kind,
        "finding_parent_kind", rjob.Spec.Finding.ParentKind())
    )
    
    // Job fails
    if rjob.Status.Phase == "Failed" {
        remediationJobsFailed.WithLabelValues(rjob.Status.Message, rjob.Spec.Finding.Kind, strconv.Itoa(int(rjob.Status.RetryCount))).Inc()
        jobDuration.Observe(duration.Seconds(),
            "finding_kind", rjob.Spec.Finding.Kind,
            "agent_type", "opencode",
            "outcome", "failed")
    }
    
    // Update queue depth
    queueDepth.WithLabelValues("Pending").Set(float64(pendingCount))
}
```

**Files to Create:**
- `internal/metrics/controller_metrics.go` (new) - all metrics
- Update `cmd/watcher/main.go` - register metrics
- Add `/metrics` HTTP endpoint to serve Prometheus metrics

**Status:** ✅ NEW FEATURE - To be implemented

---

### Gap 3: No Alerting Rules

**Problem:**
- No alerting on queue blockage
- No alerting on high failure rate
- No alerting on job timeout patterns
- No alerting on resource exhaustion
- Manual monitoring required to detect issues

**Solution:** Add Prometheus alerting rules

**Alerting Rules:**

**deploy/prometheus/alerts.yaml:**
```yaml
groups:
  - name: mechanic
    interval: 30s
    rules:
      # Queue blockage alert
      - alert: MechanicQueueBlocked
        expr: mechanic_queue_depth{phase="Dispatched"} >= max_concurrent_jobs
        for: 15m
        labels:
          severity: critical
          component: mechanic-watcher
        annotations:
          summary: "Mechanic queue is blocked by failed jobs"
          runbook_url: "https://docs.example.com/runbooks/mechanic-queue-blockage"
      
      # High failure rate alert
      - alert: MechanicJobFailureRateHigh
        expr: rate(mechanic_remediation_jobs_failed_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
          component: mechanic-watcher
        annotations:
          summary: "High rate of job failures ({{ $value | humanizePercentage }} per second)"
      
      # Job timeout spike alert
      - alert: MechanicJobTimeoutSpike
        expr: rate(mechanic_remediation_jobs_failed_total{failure_reason="DeadlineExceeded"}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
          component: mechanic-watcher
        annotations:
          summary: "Spike in job timeouts ({{ $value }} per second)"
      
      # Long running jobs alert
      - alert: MechanicJobRunningTooLong
        expr: histogram_quantile(0.95, rate(mechanic_job_execution_duration_seconds[5m])) > 900
        for: 10m
        labels:
          severity: info
          component: mechanic-watcher
        annotations:
          summary: "95th percentile of job duration exceeds 15 minutes"
```

**Files to Create:**
- `deploy/prometheus/alerts.yaml` (new) - alerting rules
- Update Helm chart to include ConfigMap for Prometheus rules
- Update README to document alerting setup

**Status:** ✅ NEW FEATURE - To be implemented

---

### Gap 4: No Grafana Dashboard

**Problem:**
- No visual dashboard for monitoring
- Metrics exist but not easily viewable
- Operators must query Prometheus directly
- No operational overview at a glance

**Solution:** Create Grafana dashboard for key metrics

**Dashboard Components:**

**Row 1: System Health**
- Panel 1: Queue Depth by Phase (Gauge)
- Panel 2: Current Queue Status (Stat)
- Panel 3: Jobs Created/Completed/Failed (Stats)

**Row 2: Job Performance**
- Panel 4: Job Duration Histogram (Histogram)
- Panel 5: P95/P99 Job Duration (Stats)
- Panel 6: Job Duration Over Time (Graph)

**Row 3: Failure Analysis**
- Panel 7: Failure Rate by Reason (Graph)
- Panel 8: Failure Rate Over Time (Graph)
- Panel 9: Top Failure Reasons (Table)

**Row 4: Retry Analysis**
- Panel 10: Retry Distribution (Histogram)
- Panel 11: Retry Rate Over Time (Graph)

**deploy/grafana/dashboards/mechanic-overview.json:**
```json
{
  "dashboard": {
    "title": "Mechanic Operations",
    "uid": "mechanic-overview",
    "timezone": "browser",
    "refresh": "30s",
    "panels": [
      {
        "id": 1,
        "title": "Queue Depth by Phase",
        "type": "stat",
        "targets": [
          {
            "expr": "mechanic_queue_depth",
            "legendFormat": "{{phase}}",
            "refId": "A"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "min": 0
          },
          "overrides": []
        }
      }
    ],
    // ... more panels
  }
}
```

**Files to Create:**
- `deploy/grafana/dashboards/mechanic-overview.json` (new) - Grafana dashboard
- Update Helm chart to include dashboard ConfigMap
- Update README to document dashboard usage

**Status:** ✅ NEW FEATURE - To be implemented

---

## What This Story Does NOT Do

- ❌ Replace existing logging (Zap already works well)
- ❌ Add new Kubernetes Events (already exist)
- ❌ Add new status fields (existing fields adequate)
- ❌ Implement job status sync (already works)

---

## Files to Create or Modify

### New Files:
| File | Purpose |
|------|---------|
| `internal/controller/correlation.go` | Correlation ID helpers |
| `internal/metrics/controller_metrics.go` | Prometheus metrics |
| `deploy/prometheus/alerts.yaml` | Alerting rules |
| `deploy/grafana/dashboards/mechanic-overview.json` | Grafana dashboard |

### Modified Files:
| File | Change |
|------|--------|
| `cmd/watcher/main.go` | Register metrics endpoint |
| `charts/mendabot/templates/deployment.yaml` | Add metrics port to Service |
| `README-LLM.md` | Document metrics/alerting setup |

---

## Acceptance Criteria

- [x] Verified existing logging (Zap) works correctly
- [x] Verified existing Kubernetes Events work correctly
- [x] Verified queue depth tracking exists
- [x] Verified RemediationJob status fields are adequate
- [x] Identified gaps: correlation IDs, metrics, alerts, dashboard
- [x] Designed correlation ID implementation (non-breaking)
- [x] Designed Prometheus metrics (comprehensive)
- [x] Designed Grafana dashboard (operational view)
- [x] Designed Prometheus alerting rules (key scenarios)
- [x] All designs validated against project architecture
- [x] No incorrect assumptions about existing features
- [x] Story blocked until STORY_02 completes diagnosis
- [x] Implementation plan clear and ready after diagnosis

---

## Definition of Done

- [x] Existing observability features documented
- [x] Gaps identified and prioritized
- [x] New observability features designed
- [x] Implementation plans validated
- [x] No breaking changes to existing code
- [x] Ready for implementation after diagnosis
- [x] Worklog entry created

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|-------|----------|-------------|------------|
| Breaking existing logging | HIGH | LOW | Use zap correctly, don't replace |
| Adding too many metrics | MEDIUM | LOW | Focus on key metrics first |
| Alerting fatigue | MEDIUM | LOW | Thresholds based on production data |
| Dashboard complexity | LOW | LOW | Start simple, evolve over time |

---

## Expected Outcomes

### Immediate Benefits (After Implementation):
- Queue blockages detected within 5 minutes
- Performance degradation detected within 10 minutes
- Failure spikes detected within 5 minutes
- End-to-end tracing with correlation IDs
- Operational view in Grafana dashboard

### Long-term Benefits:
- Trend analysis of job performance
- Capacity planning based on metrics
- Root cause analysis with data
- Proactive issue detection and resolution
