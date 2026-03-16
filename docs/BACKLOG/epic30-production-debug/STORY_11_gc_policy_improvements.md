# STORY_11: Garbage Collection Policy Improvements

## Overview

Improve Kubernetes garbage collection policies and TTL configuration to preserve diagnostic evidence from failed jobs for future incident investigations.

## Priority

**High** - Required to prevent evidence loss in future incidents

## Effort Estimate

**2 hours**

## Dependencies

None - Can be implemented immediately

## Context

During the production incident (2026-03-11 to 2026-03-15), all diagnostic evidence (logs, events, pod status) was permanently lost to Kubernetes garbage collection. Jobs had TTL of 7 days, but all evidence was garbage collected ~5 days after creation, making root cause analysis impossible.

## Objectives

1. Increase TTL for failed jobs to preserve evidence
2. Configure pod termination grace period
3. Document evidence preservation policies
4. Test policies in non-production environment

## Root Cause Analysis

**Why was evidence lost?**
- Jobs had `spec.ttlSecondsAfterFinished` set to 7 days (default)
- Kubernetes garbage collector deleted pods and events before TTL
- No pod termination grace period configured
- Logs stored in pod containers (ephemeral, not preserved)

**Why did GC happen early?**
- Kubernetes EventTTL is 1 hour (default)
- Pods may be evicted or deleted due to resource pressure
- No persistent storage for logs

## Current Configuration

### Job TTL (from code review)

From `internal/jobbuilder/job.go:64-127`:
```go
// Default TTL: 7 days (604800 seconds)
TTLSecondsAfterFinished: int64(604800),
```

### Pod Termination (from manifests)

No `terminationGracePeriodSeconds` configured (defaults to 30s)

### Event TTL (Kubernetes default)

1 hour (not configurable in current setup)

## Proposed Changes

### 1. Increase Job TTL for Failed Jobs

Create separate TTL policies for successful vs failed jobs:

```go
// internal/jobbuilder/job.go

func (b *JobBuilder) GetJobTTL() int64 {
    // Check if this is a retry or first attempt
    if b.rjob.Status.RetryCount > 0 {
        // Longer TTL for retried jobs (preserve evidence for investigation)
        return int64(30 * 24 * 60 * 60) // 30 days
    }

    // Check current job status (if known)
    if b.rjob.Status.Phase == remediationmechaniciov1alpha1.PhaseFailed {
        // Longer TTL for failed jobs
        return int64(30 * 24 * 60 * 60) // 30 days
    }

    // Default TTL for successful jobs
    return int64(7 * 24 * 60 * 60) // 7 days
}
```

### 2. Configure Pod Termination Grace Period

Add to `internal/jobbuilder/job.go`:

```go
// Ensure logs are preserved during pod termination
terminationGracePeriodSeconds: int64(600), // 10 minutes (up from 30s default)
```

### 3. Enable Pod-Level Retention

Add annotation to prevent premature deletion:

```go
metadata: {
    annotations: {
        "kubernetes.io/change-cause": fmt.Sprintf("RemediationJob %s", b.rjob.Name),
        "mechanic.io/retain-logs": "true", // Custom annotation for future hook
    },
},
```

### 4. Add ConfigMap for TTL Configuration

Create `config/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mechanic-config
  namespace: mechanic-system
data:
  # TTL for successful jobs (days)
  ttl_seconds_after_finished_success: "604800"  # 7 days

  # TTL for failed jobs (days) - longer to preserve evidence
  ttl_seconds_after_finished_failure: "2592000" # 30 days

  # TTL for retried jobs (days) - longest to preserve evidence
  ttl_seconds_after_finished_retry: "2592000"   # 30 days

  # Pod termination grace period (seconds)
  termination_grace_period_seconds: "600"      # 10 minutes

  # Enable log retention
  retain_logs: "true"
```

### 5. Update Controller to Read Config

Update `internal/config/config.go` to read TTL from ConfigMap:

```go
type Config struct {
    // Existing config...
    TTLSecondsAfterFinishedSuccess int64
    TTLSecondsAfterFinishedFailure int64
    TTLSecondsAfterFinishedRetry   int64
    TerminationGracePeriodSeconds  int64
    RetainLogs                     bool
}

func LoadConfig() (*Config, error) {
    config := &Config{
        TTLSecondsAfterFinishedSuccess: 604800, // 7 days
        TTLSecondsAfterFinishedFailure: 2592000, // 30 days
        TTLSecondsAfterFinishedRetry:   2592000, // 30 days
        TerminationGracePeriodSeconds:  600,    // 10 minutes
        RetainLogs:                     true,
    }

    // Read from ConfigMap if available
    // ... implementation ...
    return config, nil
}
```

### 6. Implement Log Collection Hook (Future)

For future improvement, implement a pre-delete hook to collect logs:

```go
// Future: Implement post-delete webhook to collect logs
// This requires additional infrastructure (PVC, log storage)
```

## Implementation Details

### 1. Update Job Builder

Modify `internal/jobbuilder/job.go`:

```go
func (b *JobBuilder) Build() (*batchv1.Job, error) {
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      b.getJobName(),
            Namespace: b.rjob.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":    "mechanic-agent",
                "app.kubernetes.io/created-by": "mechanic-controller",
            },
            Annotations: map[string]string{
                "kubernetes.io/change-cause": fmt.Sprintf("RemediationJob %s", b.rjob.Name),
                "mechanic.io/retain-logs":   "true",
            },
        },
        Spec: batchv1.JobSpec{
            TTLSecondsAfterFinished:      b.GetJobTTL(),
            TerminationGracePeriodSeconds: pointer.Int64Ptr(b.config.TerminationGracePeriodSeconds),
            // ... rest of spec ...
        },
    }

    return job, nil
}

func (b *JobBuilder) GetJobTTL() int64 {
    // Check if this is a retry
    if b.rjob.Status.RetryCount > 0 {
        return b.config.TTLSecondsAfterFinishedRetry
    }

    // Check if job is failed (may be set during retry)
    if b.rjob.Status.Phase == remediationmechaniciov1alpha1.PhaseFailed {
        return b.config.TTLSecondsAfterFinishedFailure
    }

    // Default TTL for successful jobs
    return b.config.TTLSecondsAfterFinishedSuccess
}
```

### 2. Add ConfigMap

Create `config/manifests/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mechanic-config
  namespace: mechanic-system
data:
  ttl_seconds_after_finished_success: "604800"  # 7 days
  ttl_seconds_after_finished_failure: "2592000" # 30 days
  ttl_seconds_after_finished_retry: "2592000"   # 30 days
  termination_grace_period_seconds: "600"      # 10 minutes
  retain_logs: "true"
```

### 3. Update Deployment

Update `config/manager/deployment.yaml` to mount ConfigMap:

```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        envFrom:
        - configMapRef:
            name: mechanic-config
```

### 4. Apply Changes

```bash
# Apply ConfigMap
kubectl apply -f config/manifests/configmap.yaml

# Rebuild and deploy controller
make docker-build
make deploy
```

## Testing

### 1. Test TTL Configuration

```bash
# Create a test RemediationJob
kubectl apply -f test/test-job.yaml

# Wait for job to complete/fail
kubectl wait --for=condition=complete --timeout=600s job/test-agent-job -n mechanic-agents

# Check job TTL
kubectl get job test-agent-job -n mechanic-agents -o yaml | grep ttlSecondsAfterFinished
```

### 2. Test Graceful Termination

```bash
# Delete a running pod and observe termination time
kubectl delete pod test-agent-job-xxx -n mechanic-agents
watch kubectl get pod test-agent-job-xxx -n mechanic-agents

# Verify termination takes 10 minutes (grace period)
```

### 3. Test Evidence Preservation

```bash
# Create failed job
kubectl apply -f test/failed-job.yaml

# Wait for failure
kubectl wait --for=condition=failed --timeout=600s job/test-failed-job -n mechanic-agents

# Verify pod still exists (not garbage collected)
kubectl get pods -n mechanic-agents -l job-name=test-failed-job

# Verify logs are accessible
kubectl logs test-failed-job-xxx -n mechanic-agents
```

## Success Criteria

- [ ] ConfigMap created with TTL configuration
- [ ] Job TTL updated for failed jobs (30 days)
- [ ] Job TTL updated for retried jobs (30 days)
- [ ] Job TTL preserved for successful jobs (7 days)
- [ ] Pod termination grace period set to 10 minutes
- [ ] Logs annotation added to pods
- [ ] Tests pass in non-production
- [ ] Documentation updated

## Rollback Plan

If increased TTL causes resource issues:
1. Revert TTL to 7 days for all jobs
2. Delete failed jobs manually to free resources
3. Monitor resource usage

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Increased TTL causes resource bloat | Medium | Medium | Monitor storage usage, implement resource quotas |
| Failed jobs accumulate | Medium | Medium | STORIES 09-10 alerts will detect accumulation |
| Longer grace period slows scale-down | Low | Low | 10 minutes is reasonable for graceful shutdown |

## Related Stories

- **STORY_09**: Prometheus metrics (will monitor TTL effectiveness)
- **STORY_10**: Alerting rules (alerts for accumulating failed jobs)
- **STORY_12**: Evidence collection documentation (documents evidence preservation)
