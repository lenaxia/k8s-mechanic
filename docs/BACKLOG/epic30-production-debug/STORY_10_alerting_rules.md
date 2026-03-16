# STORY_10: Alerting Rules Configuration

## Overview

Configure Prometheus alerting rules to detect queue blockages, high failure rates, and other critical issues in the mechanic system.

## Priority

**Critical** - Required for early detection of production issues

## Effort Estimate

**3 hours**

## Dependencies

**STORY_09** - Requires Prometheus metrics to be implemented first

## Context

During the production incident (2026-03-11 to 2026-03-15), the queue blockage was not detected for ~5 days because there were no alerting rules configured. This resulted in a complete operational deadlock that could have been prevented with proper alerting.

## Objectives

1. Create critical alerts for queue blockages
2. Create alerts for high failure rates
3. Create alerts for other production issues
4. Configure appropriate thresholds and durations
5. Provide actionable alert annotations

## Alerting Rules

### Critical Alerts

#### 1. Mechanic Queue Blocked

Detects when Dispatched jobs occupy all available concurrent slots (likely due to failures):

```yaml
groups:
  - name: mechanic_critical
    interval: 30s
    rules:
      - alert: MechanicQueueBlocked
        expr: |
          mechanic_queue_depth{phase="Dispatched"} >= max(concurrent_jobs_available) or
          mechanic_queue_depth{phase="Dispatched"} >= 3
        for: 10m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Mechanic queue is blocked by failed jobs"
          description: |
            {{ $value }} Dispatched jobs are occupying all available concurrent slots.
            This likely indicates failed jobs are not being cleaned up.
          runbook: "https://docs.internal/mechanic/runbooks/queue-blocked"
          impact: "New investigations cannot run until queue is unblocked"
```

#### 2. Mechanic High Failure Rate

Detects spike in job failures:

```yaml
      - alert: MechanicHighFailureRate
        expr: |
          rate(mechanic_remediation_jobs_failed_total[5m]) > 0.1
        for: 10m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "High rate of mechanic job failures"
          description: |
            {{ $value }} jobs are failing per minute (average over 5m).
            This may indicate a systemic issue with agents or infrastructure.
          runbook: "https://docs.internal/mechanic/runbooks/high-failure-rate"
```

### Warning Alerts

#### 3. Mechanic Job Timeout Rate

Detects high rate of job timeouts:

```yaml
  - name: mechanic_warnings
    interval: 30s
    rules:
      - alert: MechanicJobTimeoutRateHigh
        expr: |
          rate(mechanic_remediation_jobs_failed_total{failure_reason="timeout"}[5m]) > 0.05
        for: 15m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High rate of agent job timeouts"
          description: |
            {{ $value }} jobs are timing out per minute (average over 5m).
            This may indicate performance issues with agents or LLM APIs.
```

#### 4. Mechanic Queue Depth High

Detects growing queue (may indicate performance issues):

```yaml
      - alert: MechanicQueueDepthHigh
        expr: |
          mechanic_queue_depth{phase="Pending"} > 50
        for: 30m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "Mechanic queue depth is high"
          description: |
            {{ $value }} RemediationJobs are pending dispatch.
            This may indicate the system cannot keep up with findings.
```

#### 5. Mechanic Failed Jobs Accumulating

Detects failed jobs not being cleaned up:

```yaml
      - alert: MechanicFailedJobsAccumulating
        expr: |
          mechanic_failed_jobs_pending_cleanup > 5
        for: 1h
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "Failed jobs are accumulating"
          description: |
            {{ $value }} failed jobs are pending cleanup.
            This may indicate cleanup mechanism is not working.
```

### Info Alerts

#### 6. Mechanic Job Duration Long

Tracks unusually long-running jobs:

```yaml
  - name: mechanic_info
    interval: 30s
    rules:
      - alert: MechanicJobDurationLong
        expr: |
          histogram_quantile(0.95, rate(mechanic_job_duration_seconds_bucket[10m])) > 900
        for: 15m
        labels:
          severity: info
          team: platform
        annotations:
          summary: "95th percentile job duration exceeds 15 minutes"
          description: |
            {{ $value }}s is the 95th percentile job duration.
            This may indicate some jobs are taking longer than expected.
```

## Implementation Details

### 1. Create Alert Rules File

Create `config/prometheus/alerts.yaml`:

```yaml
groups:
  # Critical alerts (above) go here
  # Warning alerts (above) go here
  # Info alerts (above) go here
```

### 2. Add to Prometheus Configuration

Update Prometheus config to include alert rules:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yaml: |
    global:
      scrape_interval: 30s
      evaluation_interval: 30s

    rule_files:
      - /etc/prometheus/rules/mechanic.yaml

    scrape_configs:
      # Existing scrape configs...
      - job_name: 'mechanic-controller'
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
            action: keep
            regex: mechanic
          - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_component]
            action: keep
            regex: controller
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
          - source_labels: [__meta_kubernetes_pod_ip]
            target_label: __address__
            replacement: $1:8080
```

### 3. Create Alertmanager Routes

Create `config/alertmanager/alertmanager.yaml`:

```yaml
global:
  resolve_timeout: 5m

route:
  receiver: 'default'
  group_by: ['alertname', 'cluster']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
  routes:
    # Critical alerts - immediate notification
    - match:
        severity: critical
      receiver: 'pagerduty'
      continue: false

    # Warning alerts - email notification
    - match:
        severity: warning
      receiver: 'email'
      continue: false

    # Info alerts - slack notification
    - match:
        severity: info
      receiver: 'slack'
      continue: false

receivers:
  - name: 'default'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/...'
        channel: '#mechanic-alerts'

  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: 'YOUR_PAGERDUTY_SERVICE_KEY'

  - name: 'email'
    email_configs:
      - to: 'platform-team@company.com'
        from: 'alertmanager@company.com'

  - name: 'slack'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/...'
        channel: '#mechanic-alerts'
```

### 4. Apply Configurations

```bash
kubectl apply -f config/prometheus/alerts.yaml
kubectl apply -f config/alertmanager/alertmanager.yaml
kubectl create configmap prometheus-config --from-file=prometheus.yaml=prometheus.yaml
kubectl create configmap alertmanager-config --from-file=alertmanager.yaml=alertmanager.yaml
```

## Testing

### 1. Test Alert Rules

```bash
# Verify alert rules are loaded
kubectl exec -it prometheus-0 -n monitoring -- promtool check rules /etc/prometheus/rules/mechanic.yaml

# Check for syntax errors
kubectl exec -it prometheus-0 -n monitoring -- wget http://localhost:9090/-/ready
```

### 2. Simulate Queue Blockage

```bash
# Create failed jobs to trigger alert
kubectl apply -f test/failed-jobs.yaml

# Wait for alert to fire (10m)
kubectl get prometheusrules -n monitoring
kubectl logs prometheus-0 -n monitoring | grep MechanicQueueBlocked
```

### 3. Test Alert Delivery

```bash
# Check alertmanager logs for delivery
kubectl logs alertmanager-0 -n monitoring | grep pagerduty
kubectl logs alertmanager-0 -n monitoring | grep email
```

## Success Criteria

- [ ] All critical alerts configured and tested
- [ ] All warning alerts configured and tested
- [ ] All info alerts configured and tested
- [ ] Alert syntax validated
- [ ] Alert delivery tested (pagerduty, email, slack)
- [ ] Documentation updated
- [ ] Runbooks created for each critical alert

## Rollback Plan

If alerts are too noisy:
1. Increase alert thresholds
2. Increase alert durations (for: X minutes)
3. Disable specific alerts by commenting out

## Related Stories

- **STORY_09**: Prometheus metrics (dependency)
- **STORY_11**: GC policy improvements (alerts will help monitor GC effectiveness)
- **STORY_12**: Evidence collection (alerts will trigger evidence collection)
