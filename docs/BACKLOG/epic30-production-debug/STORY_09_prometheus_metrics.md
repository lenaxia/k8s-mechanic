# STORY_09: Prometheus Metrics Implementation

## Overview

Implement Prometheus metrics for RemediationJob lifecycle monitoring to enable visibility into the mechanic system's health and performance.

## Priority

**Critical** - Required for early detection of production issues

## Effort Estimate

**4 hours**

## Dependencies

None - Can be implemented immediately

## Context

The mechanic system currently has no Prometheus metrics or monitoring. During the production incident (2026-03-11 to 2026-03-15), the queue blockage was not detected until it had been ongoing for ~5 days because there were no alerts or dashboards.

## Objectives

1. Implement Prometheus metrics for RemediationJob lifecycle events
2. Expose metrics on the controller's /metrics endpoint
3. Cover all critical aspects of job lifecycle
4. Provide sufficient granularity for debugging

## Metrics to Implement

### Counter Metrics

```go
// RemediationJobs created by tier and severity
remediation_jobs_created_total{tier, severity}

// RemediationJobs completed successfully
remediation_jobs_completed_total{duration_seconds}

// RemediationJobs failed
remediation_jobs_failed_total{failure_reason, attempts}

// Retry attempts
remediation_job_retries_total{finding_kind, agent_type}
```

### Histogram Metrics

```go
// Job execution duration
job_duration_seconds{finding_kind, agent_type}

// Queue time (time from creation to dispatch)
queue_time_seconds{tier, severity}
```

### Gauge Metrics

```go
// Current queue depth by phase
queue_depth{phase}

// Active jobs by phase
active_jobs{phase}

// Failed jobs pending cleanup
failed_jobs_pending_cleanup
```

## Implementation Details

### 1. Add Prometheus Dependencies

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promauto
```

### 2. Create Metrics Package

Create `internal/metrics/metrics.go`:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counters
    RemediationJobsCreated = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_created_total",
            Help: "Total number of RemediationJobs created",
        },
        []string{"tier", "severity"},
    )

    RemediationJobsCompleted = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_completed_total",
            Help: "Total number of RemediationJobs completed successfully",
        },
        []string{"duration_seconds"},
    )

    RemediationJobsFailed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_jobs_failed_total",
            Help: "Total number of RemediationJobs failed",
        },
        []string{"failure_reason", "attempts"},
    )

    RemediationJobRetries = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mechanic_remediation_job_retries_total",
            Help: "Total number of retry attempts",
        },
        []string{"finding_kind", "agent_type"},
    )

    // Histograms
    JobDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_job_duration_seconds",
            Help:    "Duration of agent job execution",
            Buckets: prometheus.DefBuckets,
        },
        []string{"finding_kind", "agent_type"},
    )

    QueueTime = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "mechanic_queue_time_seconds",
            Help:    "Time from RemediationJob creation to dispatch",
            Buckets: prometheus.DefBuckets,
        },
        []string{"tier", "severity"},
    )

    // Gauges
    QueueDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mechanic_queue_depth",
            Help: "Current number of pending RemediationJobs by phase",
        },
        []string{"phase"},
    )

    ActiveJobs = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mechanic_active_jobs",
            Help: "Current number of active jobs by phase",
        },
        []string{"phase"},
    )

    FailedJobsPendingCleanup = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "mechanic_failed_jobs_pending_cleanup",
            Help: "Number of failed jobs awaiting cleanup",
        },
    )
)

// Helper functions for common metrics
func RecordRemediationJobCreated(tier, severity string) {
    RemediationJobsCreated.WithLabelValues(tier, severity).Inc()
}

func RecordRemediationJobCompleted(durationSeconds float64) {
    RemediationJobsCompleted.WithLabelValues(fmt.Sprintf("%.0f", durationSeconds)).Inc()
}

func RecordRemediationJobFailed(failureReason string, attempts int) {
    RemediationJobsFailed.WithLabelValues(failureReason, fmt.Sprintf("%d", attempts)).Inc()
}

func RecordJobDuration(findingKind, agentType string, durationSeconds float64) {
    JobDuration.WithLabelValues(findingKind, agentType).Observe(durationSeconds)
}

func RecordQueueTime(tier, severity string, durationSeconds float64) {
    QueueTime.WithLabelValues(tier, severity).Observe(durationSeconds)
}

func UpdateQueueDepth(phase string, count int) {
    QueueDepth.WithLabelValues(phase).Set(float64(count))
}

func UpdateActiveJobs(phase string, count int) {
    ActiveJobs.WithLabelValues(phase).Set(float64(count))
}

func UpdateFailedJobsPendingCleanup(count int) {
    FailedJobsPendingCleanup.Set(float64(count))
}
```

### 3. Expose Metrics in Controller

Add to `internal/controller/remediationjob_controller.go`:

```go
import (
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
)

func (r *RemediationJobReconciler) SetupMetricsServer() {
    http.Handle("/metrics", promhttp.Handler())
    go func() {
        log.Info("Starting metrics server on :8080")
        if err := http.ListenAndServe(":8080", nil); err != nil {
            log.Error(err, "metrics server failed")
        }
    }()
}
```

Call in main.go:

```go
if err := ctrl.NewWebhookManagedBy(mgr).For(&remediationmechaniciov1alpha1.RemediationJob{}).Complete(r); err != nil {
    setupLog.Error(err, "unable to create webhook", "webhook", "RemediationJob")
    os.Exit(1)
}

// Start metrics server
r.SetupMetricsServer()
```

### 4. Instrument Controller Logic

Update reconciliation loop to record metrics:

```go
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Get RemediationJob
    rjob := &remediationmechaniciov1alpha1.RemediationJob{}
    if err := r.Get(ctx, req.NamespacedName, rjob); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Record queue time for first reconciliation
    if rjob.Status.DispatchedAt.IsZero() {
        queueTime := time.Since(rjob.CreationTimestamp.Time).Seconds()
        metrics.RecordQueueTime(rjob.Spec.Tier, rjob.Spec.Severity, queueTime)
    }

    // Track phase transitions
    switch rjob.Status.Phase {
    case remediationmechaniciov1alpha1.PhaseDispatched:
        // Track active dispatched jobs
        r.updateQueueMetrics(ctx)
        
        // Record start time for duration tracking
        if rjob.Status.DispatchedAt.IsZero() {
            rjob.Status.DispatchedAt = &metav1.Time{Time: time.Now()}
        }
        
    case remediationmechaniciov1alpha1.PhaseCompleted:
        // Calculate and record job duration
        if !rjob.Status.DispatchedAt.IsZero() {
            duration := time.Since(rjob.Status.DispatchedAt.Time).Seconds()
            metrics.RecordJobDuration(
                rjob.Spec.Finding.Kind,
                rjob.Spec.Agent.Type,
                duration,
            )
            metrics.RecordRemediationJobCompleted(duration)
        }
        
    case remediationmechaniciov1alpha1.PhaseFailed:
        // Record failure
        metrics.RecordRemediationJobFailed(
            rjob.Status.FailureReason,
            rjob.Status.RetryCount,
        )
    }

    // Update queue depth metrics
    r.updateQueueMetrics(ctx)

    // Rest of reconciliation logic...
}

func (r *RemediationJobReconciler) updateQueueMetrics(ctx context.Context) {
    rjobs := &remediationmechaniciov1alpha1.RemediationJobList{}
    if err := r.List(ctx, rjobs, client.InNamespace(r.Namespace)); err != nil {
        log.Error(err, "failed to list RemediationJobs for metrics")
        return
    }

    phaseCounts := make(map[string]int)
    for _, rjob := range rjobs.Items {
        phaseCounts[string(rjob.Status.Phase)]++
    }

    for phase, count := range phaseCounts {
        metrics.UpdateQueueDepth(phase, count)
        metrics.UpdateActiveJobs(phase, count)
    }
}
```

### 5. Service Definition

Create `config/manager/metrics-service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mechanic-controller-metrics
  namespace: mechanic-system
  labels:
    app.kubernetes.io/name: mechanic
    app.kubernetes.io/component: controller
spec:
  ports:
  - name: metrics
    port: 8080
    targetPort: 8080
  selector:
    app.kubernetes.io/name: mechanic
    app.kubernetes.io/component: controller
```

## Testing

1. **Unit Tests**: Test metric recording in isolation
2. **Integration Tests**: Verify metrics are exposed on /metrics endpoint
3. **Smoke Tests**: Create test RemediationJob and verify metrics appear
4. **Load Tests**: Verify metrics under concurrent job load

```bash
# Test metrics endpoint
kubectl port-forward svc/mechanic-controller-metrics 8080:8080
curl http://localhost:8080/metrics
```

## Success Criteria

- [ ] All counter, histogram, and gauge metrics implemented
- [ ] Metrics exposed on /metrics endpoint
- [ ] Service created for metrics scraping
- [ ] Metrics recorded for all lifecycle events
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Documentation updated

## Rollback Plan

If metrics cause performance issues:
1. Remove metric recording from reconciliation loop
2. Keep metrics package for future use
3. Disable metrics server

## Related Stories

- **STORY_10**: Alerting rules (depends on these metrics)
- **STORY_11**: GC policy improvements (metrics will help monitor GC effectiveness)
