# STORY_12: Evidence Collection Documentation

## Overview

Create comprehensive documentation for evidence collection procedures to ensure diagnostic data is preserved during future production incidents.

## Priority

**Medium** - Required to ensure evidence collection is followed in next incident

## Effort Estimate

**2 hours**

## Dependencies

**STORY_11** - Evidence preservation policies should be implemented first

## Context

During the production incident (2026-03-11 to 2026-03-15), no evidence collection procedures existed. When queue blockage was discovered ~5 days later, all diagnostic evidence had already been garbage collected, making root cause analysis impossible.

## Objectives

1. Create evidence collection runbook
2. Document garbage collection policies
3. Create incident response checklist
4. Document evidence preservation timeline
5. Create training materials for on-call engineers

## Documentation to Create

### 1. Evidence Collection Runbook

Create `docs/runbooks/evidence-collection.md`:

```markdown
# Mechanic Evidence Collection Runbook

## Purpose

This runbook provides step-by-step instructions for collecting diagnostic evidence from failed mechanic jobs during production incidents.

## When to Use This Runbook

- Queue blockage detected (Dispatched jobs >= MAX_CONCURRENT_JOBS)
- High failure rate alert triggered
- Agent timeout rate high
- Job failures accumulating

## Pre-Requisites

- Kubernetes cluster access (kubectl configured)
- Access to mechanic-system namespace
- Access to agent namespace (e.g., mechanic-agents)
- jq and awk installed on local machine
- Sufficient disk space for log collection

## Evidence Collection Steps

### Step 1: Identify Failed Jobs

```bash
# Get all failed RemediationJobs
kubectl get remediationjobs -n mechanic-system -o wide | grep -i failed

# Get failed jobs with details
kubectl get remediationjobs -n mechanic-system -o json | \
  jq -r '.items[] | select(.status.phase == "Failed") | "\(.metadata.name) \(.status.phase) \(.status.failureReason)"'

# Get failed batch/v1 Jobs
kubectl get jobs -n mechanic-agents -o json | \
  jq -r '.items[] | select(.status.failed > 0) | "\(.metadata.name) \(.status.failed) \(.status.completionTime)"'
```

### Step 2: Collect RemediationJob Details

For each failed RemediationJob:

```bash
# Save RemediationJob spec and status
kubectl get remediationjob <name> -n mechanic-system -o yaml > evidence/rjob-<name>.yaml

# Save RemediationJob JSON (for easier processing)
kubectl get remediationjob <name> -n mechanic-system -o json > evidence/rjob-<name>.json
```

### Step 3: Collect Job Details

```bash
# Get Job details
kubectl get job <job-name> -n mechanic-agents -o yaml > evidence/job-<job-name>.yaml

# Get Job status
kubectl get job <job-name> -n mechanic-agents -o json > evidence/job-<job-name>.json
```

### Step 4: Collect Pod Information

```bash
# Get pods for the job
kubectl get pods -n mechanic-agents -l job-name=<job-name> -o json | \
  jq -r '.items[].metadata.name'

# For each pod:
for POD in $(kubectl get pods -n mechanic-agents -l job-name=<job-name> -o json | jq -r '.items[].metadata.name'); do
  # Pod spec
  kubectl get pod $POD -n mechanic-agents -o yaml > evidence/pod-${POD}.yaml

  # Pod status
  kubectl get pod $POD -n mechanic-agents -o json > evidence/pod-${POD}.json

  # Pod events
  kubectl get events -n mechanic-agents --field-selector involvedObject.name=$POD -o yaml > evidence/events-${POD}.yaml
done
```

### Step 5: Collect Pod Logs

```bash
# Get all logs from pod
kubectl logs <pod-name> -n mechanic-agents > evidence/logs-<pod-name>.txt

# Get previous container logs (if pod restarted)
kubectl logs <pod-name> -n mechanic-agents --previous > evidence/logs-<pod-name>-previous.txt

# Get logs from init containers
kubectl logs <pod-name> -n mechanic-agents -c <init-container-name> > evidence/logs-<pod-name>-init.txt
```

### Step 6: Collect Job Events

```bash
# Get events for the job
kubectl get events -n mechanic-agents --field-selector involvedObject.name=<job-name> -o yaml > evidence/events-job-<job-name>.yaml

# Get all events in namespace (for context)
kubectl get events -n mechanic-agents --sort-by='.lastTimestamp' > evidence/events-all.txt
```

### Step 7: Collect Controller Logs

```bash
# Get controller logs
kubectl logs -n mechanic-system -l app.kubernetes.io/name=mechanic,app.kubernetes.io/component=controller --tail=10000 > evidence/controller-logs.txt

# Get controller logs around the time of failure
kubectl logs -n mechanic-system -l app.kubernetes.io/name=mechanic,app.kubernetes.io/component=controller --since-time=<RFC3339> > evidence/controller-logs-since.txt
```

### Step 8: Collect Metrics

```bash
# Get current metrics
kubectl port-forward svc/mechanic-controller-metrics 8080:8080
curl http://localhost:8080/metrics > evidence/metrics.txt

# Get Prometheus metrics (if available)
curl http://prometheus:9090/api/v1/query?query=mechanic_remediation_jobs_failed_total > evidence/metrics-prometheus.json
```

### Step 9: Collect Configuration

```bash
# Get controller config
kubectl get configmap mechanic-config -n mechanic-system -o yaml > evidence/config.yaml

# Get controller deployment
kubectl get deployment mechanic-controller -n mechanic-system -o yaml > evidence/deployment.yaml

# Get environment variables
kubectl get pods -n mechanic-system -l app.kubernetes.io/component=controller -o json | \
  jq -r '.items[0].spec.containers[0].env[]' > evidence/controller-env.json
```

### Step 10: Archive Evidence

```bash
# Create tarball
tar czf evidence-mechanic-incident-$(date +%Y%m%d-%H%M%S).tar.gz evidence/

# Upload to secure storage (e.g., S3)
aws s3 cp evidence-*.tar.gz s3://mechanic-incident-evidence/

# Document evidence location
echo "Evidence archive: evidence-*.tar.gz" >> incident-summary.md
echo "S3 location: s3://mechanic-incident-evidence/" >> incident-summary.md
```

## Evidence Preservation Timeline

| Evidence Type | Default Retention | Action Required |
|--------------|------------------|-----------------|
| RemediationJob CRD | Until deleted | Export immediately |
| batch/v1 Job | TTLSecondsAfterFinished (7-30 days) | Export before TTL expires |
| Pod | Until deleted (1-30 days) | Export immediately |
| Pod Logs | Until pod deleted (1-30 days) | Export immediately |
| Events | EventTTL (1 hour default) | Export immediately (CRITICAL) |
| Controller Logs | Log retention policy | Export recent logs |
| Prometheus Metrics | Retention period (15 days default) | Export recent metrics |

## Critical Evidence Types

**Immediate Collection Required (within 1 hour):**
- Kubernetes Events (EventTTL = 1 hour) - **CRITICAL**
- Pod logs (ephemeral, lost on pod deletion)
- Job status (lost on job deletion)

**Urgent Collection (within 24 hours):**
- RemediationJob details (lost on manual deletion)
- Job spec and status (lost after TTL)
- Pod spec and status (lost after pod deletion)

**Important Collection (within 7 days):**
- Controller logs (lost after log rotation)
- Prometheus metrics (lost after retention period)

## Common Pitfalls

1. **Waiting too long** - Events disappear after 1 hour, don't delay!
2. **Not collecting init container logs** - Init failures leave no main container logs
3. **Not collecting pod spec** - Need to see what was actually scheduled
4. **Not collecting namespace-wide events** - Need context from all events
5. **Forgetting controller logs** - Controller decisions are logged there

## Contact

For questions about evidence collection:
- Platform Team: platform@company.com
- On-Call: +1-XXX-XXX-XXXX
```

### 2. Incident Response Checklist

Create `docs/runbooks/incident-response-checklist.md`:

```markdown
# Mechanic Incident Response Checklist

## Phase 1: Detection (0-15 minutes)

- [ ] Check alerts (Prometheus, PagerDuty, Slack)
- [ ] Verify alert details (affected namespace, severity)
- [ ] Check Grafana dashboard for mechanic metrics
- [ ] Identify affected jobs (Dispatched, Failed, Pending)
- [ ] Determine severity and urgency

## Phase 2: Immediate Mitigation (15-30 minutes)

- [ ] If queue blocked: identify stuck jobs
- [ ] If high failure rate: check recent job failures
- [ ] If resource issues: check pod evictions, node pressure
- [ ] Escalate to platform team if severity is critical

## Phase 3: Evidence Collection (30-90 minutes)

**CRITICAL: Complete within 1 hour to preserve Events!**

- [ ] Create evidence directory with timestamp
- [ ] Collect all failed RemediationJob specs
- [ ] Collect all failed Job specs and status
- [ ] Collect pod specs and status for all pods
- [ ] Collect pod logs for all pods
- [ ] Collect Kubernetes Events (highest priority!)
- [ ] Collect controller logs
- [ ] Collect Prometheus metrics
- [ ] Collect configuration files
- [ ] Archive and upload to secure storage

## Phase 4: Investigation (90 minutes - 4 hours)

- [ ] Review Events for failure patterns
- [ ] Review pod logs for error messages
- [ ] Review controller logs for decisions
- [ ] Check for common failure modes (timeout, auth, network)
- [ ] Identify root cause if possible

## Phase 5: Resolution (4-8 hours)

- [ ] Implement short-term fix if needed
- [ ] Test fix in staging environment
- [ ] Deploy fix to production
- [ ] Verify fix resolves issue
- [ ] Clear backlog of pending jobs

## Phase 6: Documentation (8-24 hours)

- [ ] Create incident summary
- [ ] Document root cause
- [ ] Document resolution
- [ ] Update runbooks if needed
- [ ] Schedule post-incident review
- [ ] Create worklog entry

## Phase 7: Follow-up (1-7 days)

- [ ] Post-incident review meeting
- [ ] Identify long-term improvements
- [ ] Implement improvements
- [ ] Update documentation
- [ ] Train team on lessons learned
```

### 3. Garbage Collection Policies Document

Create `docs/production/garbage-collection-policies.md`:

```markdown
# Mechanic Garbage Collection Policies

## Overview

This document describes Kubernetes garbage collection policies and TTL configuration for mechanic jobs and related resources.

## Resource TTL Configuration

| Resource | TTL | Configurable | Reason |
|----------|-----|--------------|--------|
| RemediationJob CRD | Manual deletion | Yes | Preserved until manually deleted |
| batch/v1 Job | 7-30 days | Yes | Via TTLSecondsAfterFinished field |
| Pods | 1-30 days | Yes | Via Job TTL and ownerReferences |
| Events | 1 hour | No | Kubernetes EventTTL is hardcoded |
| Pod Logs | 1-30 days | Yes | Ephemeral, lost on pod deletion |
| Controller Logs | 15 days | Yes | Log retention policy |

## TTL Values

### Successful Jobs
- **TTL:** 7 days (604800 seconds)
- **Reason:** Successful jobs don't need long-term preservation
- **Config:** `ttl_seconds_after_finished_success`

### Failed Jobs
- **TTL:** 30 days (2592000 seconds)
- **Reason:** Preserve evidence for investigation
- **Config:** `ttl_seconds_after_finished_failure`

### Retried Jobs
- **TTL:** 30 days (2592000 seconds)
- **Reason:** Preserve evidence for investigation
- **Config:** `ttl_seconds_after_finished_retry`

### Events
- **TTL:** 1 hour (3600 seconds)
- **Reason:** Kubernetes EventTTL is hardcoded, not configurable
- **Critical:** Events must be collected immediately!

## Evidence Preservation Timeline

```
T=0 hours:   Job created
T=0 hours:   Job starts running (pod created)
T=0 hours:   Events being generated
T=0.5 hours: Job fails
T=1 hour:    ⚠️ Events deleted by Kubernetes GC (COLLECT NOW!)
T=1 day:     Pod deleted by Job TTL
T=7 days:    Successful job deleted by TTL
T=30 days:   Failed/retried job deleted by TTL
```

## Recommendations

1. **Immediate Collection (within 1 hour):**
   - Events (CRITICAL - will be deleted)
   - Pod logs (will be lost on pod deletion)

2. **Urgent Collection (within 24 hours):**
   - Job spec and status
   - Pod spec and status
   - RemediationJob details

3. **Important Collection (within 7 days):**
   - Controller logs
   - Prometheus metrics

4. **Monitor Resource Usage:**
   - Failed jobs may accumulate
   - Set up alerts for accumulation
   - Implement resource quotas

## Configuration

TTL values are configured via ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mechanic-config
  namespace: mechanic-system
data:
  ttl_seconds_after_finished_success: "604800"   # 7 days
  ttl_seconds_after_finished_failure: "2592000"  # 30 days
  ttl_seconds_after_finished_retry: "2592000"    # 30 days
  termination_grace_period_seconds: "600"         # 10 minutes
  retain_logs: "true"
```

## Monitoring

Use Prometheus metrics to monitor TTL effectiveness:

- `mechanic_failed_jobs_pending_cleanup` - Gauge of failed jobs awaiting cleanup
- `mechanic_queue_depth{phase="Failed"}` - Failed jobs by phase

Alerts configured in STORY_10 will notify when failed jobs accumulate.

## Changing TTL Values

To change TTL values:

1. Update ConfigMap:
   ```bash
   kubectl edit configmap mechanic-config -n mechanic-system
   ```

2. Update `ttl_seconds_after_finished_*` values

3. Restart controller:
   ```bash
   kubectl rollout restart deployment mechanic-controller -n mechanic-system
   ```

4. New jobs will use new TTL values

## Resource Quotas

Consider implementing resource quotas to prevent bloat:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: mechanic-resource-quota
  namespace: mechanic-agents
spec:
  hard:
    pods: "100"
    requests.cpu: "20"
    requests.memory: "40Gi"
    limits.cpu: "40"
    limits.memory: "80Gi"
```
```

### 4. Training Materials

Create `docs/training/evidence-collection-training.md`:

```markdown
# Evidence Collection Training

## Target Audience

On-call engineers, platform team members

## Objectives

After this training, you will be able to:
- Identify when evidence collection is needed
- Collect diagnostic evidence from mechanic jobs
- Preserve evidence for future analysis
- Understand garbage collection timelines

## Scenario

You receive an alert: "Mechanic queue is blocked by failed jobs"

### Step 1: Identify the Issue

Check the Grafana dashboard:
- Queue depth: Dispatched = 3 (all slots occupied)
- Failure rate: High
- Recent job failures: 3 in last hour

### Step 2: Trigger Evidence Collection

**Time is critical!** Kubernetes Events have a 1-hour TTL.

1. Create evidence directory:
   ```bash
   mkdir -p evidence-$(date +%Y%m%d-%H%M%S)
   cd evidence-$(date +%Y%m%d-%H%M%S)
   ```

2. Follow evidence collection runbook:
   - Step 1: Identify failed jobs
   - Step 2: Collect RemediationJob details
   - Step 3: Collect Job details
   - Step 4: Collect Pod information
   - Step 5: Collect Pod logs
   - Step 6: Collect Job events (CRITICAL - do first!)
   - Step 7: Collect Controller logs
   - Step 8: Collect Metrics
   - Step 9: Collect Configuration
   - Step 10: Archive evidence

### Step 3: Preserve Evidence

Upload to secure storage:
```bash
tar czf evidence.tar.gz .
aws s3 cp evidence.tar.gz s3://mechanic-incident-evidence/
```

### Step 4: Document

Create incident summary:
```bash
cat > incident-summary.md << EOF
# Incident Summary

Date: $(date)
Severity: Critical
Issue: Queue blockage

## Evidence Collected
- RemediationJob specs: 3
- Job specs: 3
- Pods: 3
- Pod logs: 3
- Events: All
- Controller logs: Yes
- Metrics: Yes

## Next Steps
- Investigate root cause
- Implement fix
- Document findings
EOF
```

## Quiz

1. **What is the TTL for Kubernetes Events?**
   - Answer: 1 hour (3600 seconds)

2. **What is the TTL for failed mechanic jobs?**
   - Answer: 30 days (2592000 seconds)

3. **Which evidence must be collected immediately?**
   - Answer: Kubernetes Events (will be deleted after 1 hour)

4. **Where should evidence be archived?**
   - Answer: Secure storage (S3) with proper access controls

5. **What happens if you wait too long to collect evidence?**
   - Answer: Events will be deleted, pod logs will be lost, root cause analysis impossible

## Hands-On Exercise

1. Simulate a failed job in staging
2. Practice evidence collection runbook
3. Archive evidence to S3
4. Review evidence with a peer

## Resources

- Evidence Collection Runbook: [link]
- Incident Response Checklist: [link]
- Garbage Collection Policies: [link]
- Platform Team: platform@company.com
```

## Implementation Details

### 1. Create Documentation Structure

```bash
mkdir -p docs/runbooks
mkdir -p docs/training
mkdir -p docs/production
```

### 2. Create Documentation Files

Create files as outlined above.

### 3. Add to Repository

```bash
git add docs/runbooks/evidence-collection.md
git add docs/runbooks/incident-response-checklist.md
git add docs/production/garbage-collection-policies.md
git add docs/training/evidence-collection-training.md
git commit -m "Add evidence collection documentation"
```

### 4. Internal Links

Add cross-references to documentation:
- Link from main README to evidence collection runbook
- Link from worklogs to runbooks
- Link from incident tickets to evidence archives

## Success Criteria

- [ ] Evidence collection runbook created
- [ ] Incident response checklist created
- [ ] Garbage collection policies documented
- [ ] Training materials created
- [ ] Documentation reviewed by platform team
- [ ] Training scheduled for on-call engineers
- [ ] Documentation published to internal wiki

## Rollback Plan

None - documentation only

## Related Stories

- **STORY_11**: GC policy improvements (documents the policies implemented)
- **STORY_09**: Prometheus metrics (documents metrics to collect)
- **STORY_10**: Alerting rules (references alerts that trigger evidence collection)
