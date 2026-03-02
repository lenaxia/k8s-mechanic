# Prometheus Metrics

Custom Prometheus metrics for the `mechanic-watcher` controller.
All metrics are registered in `internal/metrics/metrics.go` with the
controller-runtime default registry and exposed on `:8080/metrics`.

Enable the Helm service and optional ServiceMonitor with:

```yaml
metrics:
  enabled: true          # creates the metrics Service
serviceMonitor:
  enabled: true          # creates the Prometheus ServiceMonitor (opt-in)
```

---

## Metrics Reference

### `mechanic_findings_dispatched_total` — CounterVec

How many findings resulted in a `RemediationJob` being created.

| Label | Values |
|---|---|
| `kind` | Kubernetes resource kind — e.g. `Deployment`, `Pod`, `Node`, `StatefulSet`, `PersistentVolumeClaim`, `Job` |
| `severity` | `critical`, `high`, `medium`, `low` |

**Increment site:** `internal/provider/provider.go` — after `r.Create(ctx, rjob)` succeeds.

---

### `mechanic_findings_suppressed_total` — CounterVec

Findings that were filtered before a `RemediationJob` was created.

| Label | Values |
|---|---|
| `reason` | see table below |

| Reason | Description |
|---|---|
| `min_severity` | Below `MIN_SEVERITY` threshold |
| `stabilisation_window` | Within the stabilisation window (first-seen or window still open) |
| `duplicate` | Fingerprint already covered by an existing `RemediationJob` |
| `parent_hierarchy` | Higher-hierarchy peer `RemediationJob` already exists |
| `permanently_failed` | Fingerprint has a `PermanentlyFailed` tombstone |
| `correlation_suppressed` | Suppressed as a non-primary member of a correlation group |
| `injection_detected` | Prompt-injection pattern detected in finding fields |
| `self_remediation_depth` | `ChainDepth` exceeds `SELF_REMEDIATION_MAX_DEPTH` |
| `circuit_breaker` | Self-remediation circuit breaker is in cooldown |

**Increment sites:** `internal/provider/provider.go` (all suppression paths),
`internal/controller/remediationjob_controller.go` (`transitionSuppressed`).

---

### `mechanic_agent_jobs_active` — Gauge

Current number of active (non-terminal) agent `batch/v1` Jobs.

No labels. Set on every `concurrencyGate()` call in `RemediationJobReconciler`.

---

### `mechanic_agent_jobs_pending` — Gauge

Current number of `RemediationJob` objects in `Pending` phase (waiting for a
concurrency slot).

No labels. Set alongside `agent_jobs_active` in `concurrencyGate()`.

---

### `mechanic_agent_job_duration_seconds` — GaugeVec

Wall-clock duration of each agent Job from `DispatchedAt` to `CompletedAt`.
Set once per terminal phase transition. The value persists in the time series
until the watcher restarts, allowing post-processing of per-job durations.

| Label | Values |
|---|---|
| `fingerprint` | 12-character short fingerprint |
| `outcome` | `succeeded`, `failed`, `permanently_failed` |

**Set site:** `internal/controller/remediationjob_controller.go` — on first
transition into `PhaseSucceeded`, `PhaseFailed`, or `PhasePermanentlyFailed`.

---

### `mechanic_prs_opened_total` — Counter

Total GitHub PRs opened by agent Jobs. Incremented exactly once per
`RemediationJob` when `SinkRef.URL` transitions from empty to non-empty.
Uses a `ConditionPRRecorded` condition on the CRD as an idempotency gate.

No labels.

**Increment site:** `internal/controller/remediationjob_controller.go` —
`Reconcile()` PR-recorded guard block.

---

### `mechanic_prs_closed_total` — Counter

Total GitHub PRs closed by the watcher's auto-close mechanism.

No labels.

**Increment site:** `internal/sink/github/closer.go` — after `closeItem`
returns nil.

---

### `mechanic_circuit_breaker_activations_total` — Counter

How many times the self-remediation circuit breaker blocked a cascade finding.
A sustained high rate indicates the system is thrashing on its own fixes.

No labels.

**Increment site:** `internal/provider/provider.go` — in the `!allowed` branch
after `r.CircuitBreaker.ShouldAllow()`.

---

## Example PromQL Queries

```promql
# Dispatch rate over the last 5 minutes, by resource kind
rate(mechanic_findings_dispatched_total[5m])

# Suppression breakdown
sum by (reason) (rate(mechanic_findings_suppressed_total[5m]))

# Ratio of suppressions to dispatches
sum(rate(mechanic_findings_suppressed_total[5m]))
  / sum(rate(mechanic_findings_dispatched_total[5m]))

# Jobs currently blocked waiting for a concurrency slot
mechanic_agent_jobs_pending

# P95 job duration (approximation from GaugeVec — requires recording rule or
# external processing; histograms are not used for this metric by design)
topk(10, mechanic_agent_job_duration_seconds)

# PR close rate
rate(mechanic_prs_closed_total[1h])

# Circuit breaker trip rate
rate(mechanic_circuit_breaker_activations_total[1h])
```
