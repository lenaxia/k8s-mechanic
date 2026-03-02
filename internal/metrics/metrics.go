// Package metrics defines and registers all custom Prometheus metrics for
// the mechanic-watcher controller. All metric objects are registered with
// the controller-runtime default registry (ctrlmetrics.Registry), which is
// the same registry exposed on the :8080 /metrics endpoint by the manager.
//
// Call sites must import only this package; no direct prometheus imports
// are needed outside of internal/metrics.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Suppression reason constants — used as the `reason` label value on
// FindingsSuppressedTotal. Values match the `event:` zap fields in
// provider.go so operators can cross-reference logs and metrics.
const (
	ReasonMinSeverity           = "min_severity"
	ReasonStabilisationWindow   = "stabilisation_window"
	ReasonDuplicate             = "duplicate"
	ReasonParentHierarchy       = "parent_hierarchy"
	ReasonPermanentlyFailed     = "permanently_failed"
	ReasonCorrelationSuppressed = "correlation_suppressed"
	ReasonInjectionDetected     = "injection_detected"
	ReasonSelfRemediationDepth  = "self_remediation_depth"
	ReasonCircuitBreaker        = "circuit_breaker"
)

// Job outcome constants — used as the `outcome` label on
// AgentJobDurationSeconds and AgentJobsActive.
const (
	OutcomeSucceeded         = "succeeded"
	OutcomeFailed            = "failed"
	OutcomePermanentlyFailed = "permanently_failed"
)

var (
	// FindingsDispatchedTotal counts RemediationJobs successfully created.
	// Labels: kind (Deployment/Pod/Node/…), severity (critical/high/medium/low).
	FindingsDispatchedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "mechanic",
			Name:      "findings_dispatched_total",
			Help:      "Total number of findings that resulted in a RemediationJob being created.",
		},
		[]string{"kind", "severity"},
	)

	// FindingsSuppressedTotal counts findings that were filtered before a
	// RemediationJob was created.
	// Labels: reason (see Reason* constants above).
	FindingsSuppressedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "mechanic",
			Name:      "findings_suppressed_total",
			Help:      "Total number of findings suppressed before a RemediationJob was created, by reason.",
		},
		[]string{"reason"},
	)

	// AgentJobsActive is the current number of active (non-terminal) agent
	// batch/v1 Jobs. Set on every concurrencyGate call in
	// RemediationJobReconciler.
	AgentJobsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "mechanic",
			Name:      "agent_jobs_active",
			Help:      "Current number of active (non-terminal) agent batch/v1 Jobs.",
		},
	)

	// AgentJobsPending is the current number of RemediationJobs in the Pending
	// phase — i.e. waiting for a concurrency slot to open.
	AgentJobsPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "mechanic",
			Name:      "agent_jobs_pending",
			Help:      "Current number of RemediationJobs in the Pending phase (waiting for a concurrency slot).",
		},
	)

	// AgentJobDurationSeconds records the wall-clock duration of each agent Job
	// from DispatchedAt to CompletedAt.
	// Labels: fingerprint (12-char short form), outcome (see Outcome* constants).
	// Semantics: GaugeVec — set once per terminal transition. The value persists
	// in the Prometheus time series until the watcher restarts, allowing
	// post-processing of per-job durations.
	AgentJobDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "mechanic",
			Name:      "agent_job_duration_seconds",
			Help:      "Wall-clock duration in seconds of each agent Job from dispatch to completion (one data point per job).",
		},
		[]string{"fingerprint", "outcome"},
	)

	// PRsOpenedTotal counts PRs opened by agent Jobs. Incremented when
	// SinkRef.URL transitions from empty to non-empty in
	// RemediationJobReconciler.
	PRsOpenedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "mechanic",
			Name:      "prs_opened_total",
			Help:      "Total number of GitHub PRs opened by agent Jobs.",
		},
	)

	// PRsClosedTotal counts PRs closed by the watcher's auto-close mechanism.
	// Incremented in GitHubSinkCloser.Close after a successful close call.
	PRsClosedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "mechanic",
			Name:      "prs_closed_total",
			Help:      "Total number of GitHub PRs closed by the watcher's auto-close mechanism.",
		},
	)

	// CircuitBreakerActivationsTotal counts how many times the self-remediation
	// circuit breaker blocked a cascade finding.
	CircuitBreakerActivationsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "mechanic",
			Name:      "circuit_breaker_activations_total",
			Help:      "Total number of times the self-remediation circuit breaker blocked a cascade finding.",
		},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		FindingsDispatchedTotal,
		FindingsSuppressedTotal,
		AgentJobsActive,
		AgentJobsPending,
		AgentJobDurationSeconds,
		PRsOpenedTotal,
		PRsClosedTotal,
		CircuitBreakerActivationsTotal,
	)
}

// --- Convenience wrappers used by call sites ---

// RecordDispatched increments FindingsDispatchedTotal for the given resource
// kind and severity. kind should be the finding's resource Kind (e.g.
// "Deployment", "Pod"). severity should be the string form of domain.Severity.
func RecordDispatched(kind, severity string) {
	FindingsDispatchedTotal.WithLabelValues(kind, severity).Inc()
}

// RecordSuppressed increments FindingsSuppressedTotal for the given reason.
// Use the Reason* constants defined in this package.
func RecordSuppressed(reason string) {
	FindingsSuppressedTotal.WithLabelValues(reason).Inc()
}

// SetActiveJobs sets AgentJobsActive to the given count.
func SetActiveJobs(count int) {
	AgentJobsActive.Set(float64(count))
}

// SetPendingJobs sets AgentJobsPending to the given count.
func SetPendingJobs(count int) {
	AgentJobsPending.Set(float64(count))
}

// ObserveJobDuration records the duration of a terminal agent Job.
// fingerprint should be the 12-char short fingerprint.
// outcome should be one of the Outcome* constants.
// dispatched and completed are the timestamps from RemediationJob.Status.
func ObserveJobDuration(fingerprint, outcome string, dispatched, completed time.Time) {
	if dispatched.IsZero() || completed.IsZero() {
		return
	}
	dur := completed.Sub(dispatched).Seconds()
	if dur < 0 {
		dur = 0
	}
	AgentJobDurationSeconds.WithLabelValues(fingerprint, outcome).Set(dur)
}

// RecordPROpened increments PRsOpenedTotal.
func RecordPROpened() {
	PRsOpenedTotal.Inc()
}

// RecordPRClosed increments PRsClosedTotal.
func RecordPRClosed() {
	PRsClosedTotal.Inc()
}

// RecordCircuitBreakerActivation increments CircuitBreakerActivationsTotal.
func RecordCircuitBreakerActivation() {
	CircuitBreakerActivationsTotal.Inc()
}
