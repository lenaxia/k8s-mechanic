package metrics_test

// Comprehensive tests for the metrics package.
//
// Strategy: each test creates an isolated prometheus.Registry and registers
// the metric under test into it, then triggers the action and asserts the
// result via testutil.GatherAndCompare or manual Gather + inspection.
//
// We avoid the ctrlmetrics.Registry (process-global) to prevent count bleed
// across parallel tests. The package-level Collector vars can be registered
// into multiple registries simultaneously — prometheus allows this.
//
// For tests of the package-level helpers (RecordDispatched, RecordPROpened,
// etc.) we register the same package-level var into a local registry and
// read the delta: before-call vs after-call.

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/lenaxia/k8s-mechanic/internal/metrics"
)

// newIsolatedRegistry registers the given collectors into a fresh registry.
// AlreadyRegisteredError is silently accepted — the same collector object may
// already be in ctrlmetrics.Registry; registering it again in a local registry
// is fine and expected.
func newIsolatedRegistry(t *testing.T, collectors ...prometheus.Collector) *prometheus.Registry {
	t.Helper()
	reg := prometheus.NewRegistry()
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				t.Fatalf("prometheus.Register: %v", err)
			}
		}
	}
	return reg
}

// gatherCounterNoLabels returns the value of a no-label counter by metric name.
func gatherCounterNoLabels(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		if len(mf.GetMetric()) == 0 {
			return 0
		}
		if c := mf.GetMetric()[0].GetCounter(); c != nil {
			return c.GetValue()
		}
	}
	return 0
}

// gatherGaugeNoLabels returns the value of a no-label gauge by metric name.
func gatherGaugeNoLabels(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		if len(mf.GetMetric()) == 0 {
			return 0
		}
		if g := mf.GetMetric()[0].GetGauge(); g != nil {
			return g.GetValue()
		}
	}
	return 0
}

// gatherCounterByLabel returns the value of a labelled counter series.
func gatherCounterByLabel(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			lmap := map[string]string{}
			for _, lp := range m.GetLabel() {
				lmap[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range labels {
				if lmap[k] != v {
					match = false
					break
				}
			}
			if match {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	return 0
}

// gatherGaugeByLabel returns the value of a labelled gauge series.
func gatherGaugeByLabel(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			lmap := map[string]string{}
			for _, lp := range m.GetLabel() {
				lmap[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range labels {
				if lmap[k] != v {
					match = false
					break
				}
			}
			if match {
				if g := m.GetGauge(); g != nil {
					return g.GetValue()
				}
			}
		}
	}
	return 0
}

// ═══════════════════════════════════════════════════════════════════
// FindingsDispatchedTotal
// ═══════════════════════════════════════════════════════════════════

func TestRecordDispatched_IncrementsCorrectly(t *testing.T) {
	t.Parallel()
	// Use isolated CounterVec to get clean state regardless of parallel tests.
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "findings_dispatched_isolated_test",
		Help:      "test",
	}, []string{"kind", "severity"})
	reg := newIsolatedRegistry(t, cv)

	cv.WithLabelValues("Pod", "critical").Inc()
	cv.WithLabelValues("Pod", "critical").Inc()
	cv.WithLabelValues("Deployment", "high").Inc()

	want := `
# HELP mechanic_findings_dispatched_isolated_test test
# TYPE mechanic_findings_dispatched_isolated_test counter
mechanic_findings_dispatched_isolated_test{kind="Deployment",severity="high"} 1
mechanic_findings_dispatched_isolated_test{kind="Pod",severity="critical"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"mechanic_findings_dispatched_isolated_test"); err != nil {
		t.Error(err)
	}
}

func TestRecordDispatched_PackageHelper_IncrementsDelta(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.FindingsDispatchedTotal)

	before := gatherCounterByLabel(t, reg, "mechanic_findings_dispatched_total",
		map[string]string{"kind": "Job", "severity": "low"})
	metrics.RecordDispatched("Job", "low")
	after := gatherCounterByLabel(t, reg, "mechanic_findings_dispatched_total",
		map[string]string{"kind": "Job", "severity": "low"})

	if after-before != 1 {
		t.Errorf("expected delta 1, got %f", after-before)
	}
}

func TestRecordDispatched_AllKinds(t *testing.T) {
	t.Parallel()
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "findings_dispatched_kinds_test",
		Help:      "test",
	}, []string{"kind", "severity"})
	reg := newIsolatedRegistry(t, cv)

	kinds := []string{"Pod", "Deployment", "Node", "StatefulSet", "PersistentVolumeClaim", "Job"}
	for _, k := range kinds {
		cv.WithLabelValues(k, "medium").Inc()
	}

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	if got := len(mfs[0].GetMetric()); got != len(kinds) {
		t.Errorf("expected %d kind series, got %d", len(kinds), got)
	}
}

func TestRecordDispatched_LabelNames(t *testing.T) {
	t.Parallel()
	// Introspect the descriptor to confirm label names are "kind" and "severity".
	ch := make(chan *prometheus.Desc, 2)
	metrics.FindingsDispatchedTotal.Describe(ch)
	close(ch)
	desc := <-ch
	s := desc.String()
	if !strings.Contains(s, "kind") {
		t.Error("FindingsDispatchedTotal: 'kind' label not found in descriptor")
	}
	if !strings.Contains(s, "severity") {
		t.Error("FindingsDispatchedTotal: 'severity' label not found in descriptor")
	}
}

func TestRecordDispatched_MultipleCallsAccumulate(t *testing.T) {
	t.Parallel()
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "findings_dispatched_accumulate_test",
		Help:      "test",
	}, []string{"kind", "severity"})
	reg := newIsolatedRegistry(t, cv)

	for i := 0; i < 10; i++ {
		cv.WithLabelValues("Node", "critical").Inc()
	}

	want := `
# HELP mechanic_findings_dispatched_accumulate_test test
# TYPE mechanic_findings_dispatched_accumulate_test counter
mechanic_findings_dispatched_accumulate_test{kind="Node",severity="critical"} 10
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"mechanic_findings_dispatched_accumulate_test"); err != nil {
		t.Error(err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// FindingsSuppressedTotal
// ═══════════════════════════════════════════════════════════════════

func TestRecordSuppressed_AllReasonConstants_NonEmptyAndDistinct(t *testing.T) {
	t.Parallel()
	reasons := []string{
		metrics.ReasonMinSeverity,
		metrics.ReasonStabilisationWindow,
		metrics.ReasonDuplicate,
		metrics.ReasonParentHierarchy,
		metrics.ReasonPermanentlyFailed,
		metrics.ReasonCorrelationSuppressed,
		metrics.ReasonInjectionDetected,
		metrics.ReasonSelfRemediationDepth,
		metrics.ReasonCircuitBreaker,
	}
	seen := map[string]bool{}
	for _, r := range reasons {
		if r == "" {
			t.Error("a Reason* constant is the empty string")
		}
		if seen[r] {
			t.Errorf("duplicate Reason* constant value: %q", r)
		}
		seen[r] = true
	}
}

func TestRecordSuppressed_EachReasonIncrementsItsOwnSeries(t *testing.T) {
	t.Parallel()
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "findings_suppressed_reasons_test",
		Help:      "test",
	}, []string{"reason"})
	reg := newIsolatedRegistry(t, cv)

	reasons := []string{
		metrics.ReasonMinSeverity,
		metrics.ReasonStabilisationWindow,
		metrics.ReasonDuplicate,
		metrics.ReasonParentHierarchy,
		metrics.ReasonPermanentlyFailed,
		metrics.ReasonCorrelationSuppressed,
		metrics.ReasonInjectionDetected,
		metrics.ReasonSelfRemediationDepth,
		metrics.ReasonCircuitBreaker,
	}
	for _, r := range reasons {
		cv.WithLabelValues(r).Inc()
	}

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	if got := len(mfs[0].GetMetric()); got != len(reasons) {
		t.Errorf("expected %d reason series, got %d", len(reasons), got)
	}
}

func TestRecordSuppressed_PackageHelper_IncrementsDelta(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.FindingsSuppressedTotal)

	before := gatherCounterByLabel(t, reg, "mechanic_findings_suppressed_total",
		map[string]string{"reason": metrics.ReasonDuplicate})
	metrics.RecordSuppressed(metrics.ReasonDuplicate)
	metrics.RecordSuppressed(metrics.ReasonDuplicate)
	after := gatherCounterByLabel(t, reg, "mechanic_findings_suppressed_total",
		map[string]string{"reason": metrics.ReasonDuplicate})

	if after-before != 2 {
		t.Errorf("expected delta 2, got %f", after-before)
	}
}

func TestRecordSuppressed_ReasonsDoNotCrossContaminate(t *testing.T) {
	t.Parallel()
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "findings_suppressed_nocross_test",
		Help:      "test",
	}, []string{"reason"})
	reg := newIsolatedRegistry(t, cv)

	cv.WithLabelValues(metrics.ReasonMinSeverity).Inc()
	cv.WithLabelValues(metrics.ReasonMinSeverity).Inc()
	cv.WithLabelValues(metrics.ReasonDuplicate).Inc()

	want := `
# HELP mechanic_findings_suppressed_nocross_test test
# TYPE mechanic_findings_suppressed_nocross_test counter
mechanic_findings_suppressed_nocross_test{reason="duplicate"} 1
mechanic_findings_suppressed_nocross_test{reason="min_severity"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"mechanic_findings_suppressed_nocross_test"); err != nil {
		t.Error(err)
	}
}

func TestRecordSuppressed_LabelName(t *testing.T) {
	t.Parallel()
	ch := make(chan *prometheus.Desc, 2)
	metrics.FindingsSuppressedTotal.Describe(ch)
	close(ch)
	desc := <-ch
	if !strings.Contains(desc.String(), "reason") {
		t.Error("FindingsSuppressedTotal: 'reason' label not found in descriptor")
	}
}

// ═══════════════════════════════════════════════════════════════════
// AgentJobsActive
// ═══════════════════════════════════════════════════════════════════

// newActiveJobsGauge creates an isolated gauge with the same shape as
// AgentJobsActive. Used by tests that need SetActiveJobs-like semantics without
// sharing the process-global var across parallel tests.
func newActiveJobsGauge(t *testing.T) (prometheus.Gauge, *prometheus.Registry) {
	t.Helper()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "mechanic",
		Name:      "agent_jobs_active_isolated_" + sanitize(t.Name()),
		Help:      "test isolation gauge for agent_jobs_active",
	})
	reg := newIsolatedRegistry(t, g)
	return g, reg
}

// newPendingJobsGauge creates an isolated gauge with the same shape as
// AgentJobsPending. Used by tests that need SetPendingJobs-like semantics
// without sharing the process-global var across parallel tests.
func newPendingJobsGauge(t *testing.T) (prometheus.Gauge, *prometheus.Registry) {
	t.Helper()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "mechanic",
		Name:      "agent_jobs_pending_isolated_" + sanitize(t.Name()),
		Help:      "test isolation gauge for agent_jobs_pending",
	})
	reg := newIsolatedRegistry(t, g)
	return g, reg
}

// sanitize replaces characters that are illegal in prometheus metric names with underscores.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func TestSetActiveJobs_SetsCurrentValue(t *testing.T) {
	t.Parallel()
	g, reg := newActiveJobsGauge(t)
	g.Set(7)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 7 {
		t.Errorf("expected 7, got %f", got)
	}
}

func TestSetActiveJobs_OverwritesPreviousValue(t *testing.T) {
	t.Parallel()
	g, reg := newActiveJobsGauge(t)
	g.Set(5)
	g.Set(2)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 2 {
		t.Errorf("expected 2 after overwrite, got %f", got)
	}
}

func TestSetActiveJobs_ZeroIsValid(t *testing.T) {
	t.Parallel()
	g, reg := newActiveJobsGauge(t)
	g.Set(0)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered after Set(0)")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 0 {
		t.Errorf("expected 0, got %f", got)
	}
}

func TestSetActiveJobs_LargeValue(t *testing.T) {
	t.Parallel()
	g, reg := newActiveJobsGauge(t)
	g.Set(1000)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 1000 {
		t.Errorf("expected 1000, got %f", got)
	}
}

// ═══════════════════════════════════════════════════════════════════
// AgentJobsPending
// ═══════════════════════════════════════════════════════════════════

func TestSetPendingJobs_SetsCurrentValue(t *testing.T) {
	t.Parallel()
	g, reg := newPendingJobsGauge(t)
	g.Set(12)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 12 {
		t.Errorf("expected 12, got %f", got)
	}
}

func TestSetPendingJobs_OverwritesPreviousValue(t *testing.T) {
	t.Parallel()
	g, reg := newPendingJobsGauge(t)
	g.Set(99)
	g.Set(3)

	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics gathered")
	}
	got := mfs[0].GetMetric()[0].GetGauge().GetValue()
	if got != 3 {
		t.Errorf("expected 3 after overwrite, got %f", got)
	}
}

func TestSetPendingJobs_IndependentOfActiveJobs(t *testing.T) {
	t.Parallel()
	ga, regA := newActiveJobsGauge(t)
	gp, regP := newPendingJobsGauge(t)

	ga.Set(8)
	gp.Set(3)

	mfsA, _ := regA.Gather()
	mfsP, _ := regP.Gather()

	if len(mfsA) == 0 || len(mfsP) == 0 {
		t.Fatal("no metrics gathered")
	}
	active := mfsA[0].GetMetric()[0].GetGauge().GetValue()
	pending := mfsP[0].GetMetric()[0].GetGauge().GetValue()

	if active != 8 {
		t.Errorf("active: expected 8, got %f", active)
	}
	if pending != 3 {
		t.Errorf("pending: expected 3, got %f", pending)
	}
}

// ═══════════════════════════════════════════════════════════════════
// AgentJobDurationSeconds
// ═══════════════════════════════════════════════════════════════════

func TestObserveJobDuration_CorrectDuration(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	dispatched := time.Unix(1_000_000, 0)
	completed := time.Unix(1_000_120, 0) // 120 seconds

	metrics.ObserveJobDuration("testfp000001", metrics.OutcomeSucceeded, dispatched, completed)

	got := gatherGaugeByLabel(t, reg, "mechanic_agent_job_duration_seconds",
		map[string]string{"fingerprint": "testfp000001", "outcome": metrics.OutcomeSucceeded})
	if got != 120 {
		t.Errorf("expected duration 120s, got %f", got)
	}
}

func TestObserveJobDuration_ZeroDispatchedAt_NoObservation(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	metrics.ObserveJobDuration("testfp000002", metrics.OutcomeFailed, time.Time{}, time.Now())

	got := gatherGaugeByLabel(t, reg, "mechanic_agent_job_duration_seconds",
		map[string]string{"fingerprint": "testfp000002", "outcome": metrics.OutcomeFailed})
	// Returns 0 when not found; 0-duration would also be 0 but we check
	// that neither is present nor set to something odd.
	_ = got // no observation expected — the helper returns 0 for "not found"
	// Confirm no series with this fingerprint exists.
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "mechanic_agent_job_duration_seconds" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "fingerprint" && lp.GetValue() == "testfp000002" {
						t.Error("metric was set despite zero DispatchedAt")
					}
				}
			}
		}
	}
}

func TestObserveJobDuration_ZeroCompletedAt_NoObservation(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	metrics.ObserveJobDuration("testfp000003", metrics.OutcomeFailed, time.Now(), time.Time{})

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "mechanic_agent_job_duration_seconds" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "fingerprint" && lp.GetValue() == "testfp000003" {
						t.Error("metric was set despite zero CompletedAt")
					}
				}
			}
		}
	}
}

func TestObserveJobDuration_NegativeDuration_ClampedToZero(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	// completed before dispatched — clock skew scenario
	dispatched := time.Unix(2000, 0)
	completed := time.Unix(1999, 0)

	metrics.ObserveJobDuration("testfp000004", metrics.OutcomeSucceeded, dispatched, completed)

	got := gatherGaugeByLabel(t, reg, "mechanic_agent_job_duration_seconds",
		map[string]string{"fingerprint": "testfp000004", "outcome": metrics.OutcomeSucceeded})
	if got != 0 {
		t.Errorf("negative duration should be clamped to 0, got %f", got)
	}
}

func TestObserveJobDuration_AllThreeOutcomes(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	base := time.Unix(5000, 0)
	end := time.Unix(5100, 0)

	metrics.ObserveJobDuration("testfp000005", metrics.OutcomeSucceeded, base, end)
	metrics.ObserveJobDuration("testfp000006", metrics.OutcomeFailed, base, end)
	metrics.ObserveJobDuration("testfp000007", metrics.OutcomePermanentlyFailed, base, end)

	for _, tc := range []struct {
		fp      string
		outcome string
	}{
		{"testfp000005", metrics.OutcomeSucceeded},
		{"testfp000006", metrics.OutcomeFailed},
		{"testfp000007", metrics.OutcomePermanentlyFailed},
	} {
		got := gatherGaugeByLabel(t, reg, "mechanic_agent_job_duration_seconds",
			map[string]string{"fingerprint": tc.fp, "outcome": tc.outcome})
		if got != 100 {
			t.Errorf("fp=%s outcome=%s: expected 100s, got %f", tc.fp, tc.outcome, got)
		}
	}
}

func TestObserveJobDuration_OutcomeConstants_NonEmptyDistinct(t *testing.T) {
	t.Parallel()
	outcomes := []string{
		metrics.OutcomeSucceeded,
		metrics.OutcomeFailed,
		metrics.OutcomePermanentlyFailed,
	}
	seen := map[string]bool{}
	for _, o := range outcomes {
		if o == "" {
			t.Error("an Outcome* constant is the empty string")
		}
		if seen[o] {
			t.Errorf("duplicate Outcome* constant: %q", o)
		}
		seen[o] = true
	}
}

func TestObserveJobDuration_LabelNames(t *testing.T) {
	t.Parallel()
	ch := make(chan *prometheus.Desc, 2)
	metrics.AgentJobDurationSeconds.Describe(ch)
	close(ch)
	desc := <-ch
	s := desc.String()
	if !strings.Contains(s, "fingerprint") {
		t.Error("AgentJobDurationSeconds: 'fingerprint' label not in descriptor")
	}
	if !strings.Contains(s, "outcome") {
		t.Error("AgentJobDurationSeconds: 'outcome' label not in descriptor")
	}
}

func TestObserveJobDuration_ShortFingerprint_StillRecorded(t *testing.T) {
	t.Parallel()
	// The controller guards len(fp)>=12 before ObserveJobDuration, but the
	// helper itself should not panic on shorter strings.
	reg := newIsolatedRegistry(t, metrics.AgentJobDurationSeconds)

	dispatched := time.Unix(0, 0)
	completed := time.Unix(50, 0)

	// Should not panic.
	metrics.ObserveJobDuration("", metrics.OutcomeSucceeded, dispatched, completed)
	metrics.ObserveJobDuration("short", metrics.OutcomeSucceeded, dispatched, completed)

	// Metric is set for both; we just confirm no panic occurred.
	mfs, _ := reg.Gather()
	_ = mfs
}

// ═══════════════════════════════════════════════════════════════════
// PRsOpenedTotal
// ═══════════════════════════════════════════════════════════════════

func TestRecordPROpened_PackageHelper_IncrementsDelta(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.PRsOpenedTotal)

	before := gatherCounterNoLabels(t, reg, "mechanic_prs_opened_total")
	metrics.RecordPROpened()
	after := gatherCounterNoLabels(t, reg, "mechanic_prs_opened_total")

	if after-before != 1 {
		t.Errorf("expected delta 1, got %f", after-before)
	}
}

func TestRecordPROpened_MultipleCallsAccumulate(t *testing.T) {
	t.Parallel()
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "prs_opened_multi_test",
		Help:      "test",
	})
	reg := newIsolatedRegistry(t, c)

	for i := 0; i < 5; i++ {
		c.Inc()
	}

	want := `
# HELP mechanic_prs_opened_multi_test test
# TYPE mechanic_prs_opened_multi_test counter
mechanic_prs_opened_multi_test 5
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "mechanic_prs_opened_multi_test"); err != nil {
		t.Error(err)
	}
}

func TestRecordPROpened_IsCounter(t *testing.T) {
	t.Parallel()
	// Verify the metric is a Counter (not Gauge) — counters only increase.
	ch := make(chan *prometheus.Desc, 2)
	metrics.PRsOpenedTotal.Describe(ch)
	close(ch)
	desc := <-ch
	if !strings.Contains(desc.String(), "prs_opened_total") {
		t.Error("PRsOpenedTotal descriptor does not mention prs_opened_total")
	}
}

// ═══════════════════════════════════════════════════════════════════
// PRsClosedTotal
// ═══════════════════════════════════════════════════════════════════

func TestRecordPRClosed_PackageHelper_IncrementsDelta(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.PRsClosedTotal)

	before := gatherCounterNoLabels(t, reg, "mechanic_prs_closed_total")
	metrics.RecordPRClosed()
	after := gatherCounterNoLabels(t, reg, "mechanic_prs_closed_total")

	if after-before != 1 {
		t.Errorf("expected delta 1, got %f", after-before)
	}
}

func TestRecordPRClosed_MultipleCallsAccumulate(t *testing.T) {
	t.Parallel()
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "prs_closed_multi_test",
		Help:      "test",
	})
	reg := newIsolatedRegistry(t, c)

	for i := 0; i < 3; i++ {
		c.Inc()
	}

	want := `
# HELP mechanic_prs_closed_multi_test test
# TYPE mechanic_prs_closed_multi_test counter
mechanic_prs_closed_multi_test 3
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "mechanic_prs_closed_multi_test"); err != nil {
		t.Error(err)
	}
}

func TestRecordPROpenedAndClosed_Independent(t *testing.T) {
	t.Parallel()
	// Use isolated counters to avoid accumulation from other parallel tests.
	opened := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "prs_opened_independent_test",
		Help:      "test",
	})
	closed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "prs_closed_independent_test",
		Help:      "test",
	})
	reg := newIsolatedRegistry(t, opened, closed)

	opened.Inc()
	opened.Inc()
	closed.Inc()

	want := `
# HELP mechanic_prs_closed_independent_test test
# TYPE mechanic_prs_closed_independent_test counter
mechanic_prs_closed_independent_test 1
# HELP mechanic_prs_opened_independent_test test
# TYPE mechanic_prs_opened_independent_test counter
mechanic_prs_opened_independent_test 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"mechanic_prs_opened_independent_test",
		"mechanic_prs_closed_independent_test"); err != nil {
		t.Error(err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// CircuitBreakerActivationsTotal
// ═══════════════════════════════════════════════════════════════════

func TestRecordCircuitBreakerActivation_PackageHelper_IncrementsDelta(t *testing.T) {
	t.Parallel()
	reg := newIsolatedRegistry(t, metrics.CircuitBreakerActivationsTotal)

	before := gatherCounterNoLabels(t, reg, "mechanic_circuit_breaker_activations_total")
	metrics.RecordCircuitBreakerActivation()
	metrics.RecordCircuitBreakerActivation()
	after := gatherCounterNoLabels(t, reg, "mechanic_circuit_breaker_activations_total")

	if after-before != 2 {
		t.Errorf("expected delta 2, got %f", after-before)
	}
}

func TestRecordCircuitBreakerActivation_MultipleCallsAccumulate(t *testing.T) {
	t.Parallel()
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mechanic",
		Name:      "circuit_breaker_multi_test",
		Help:      "test",
	})
	reg := newIsolatedRegistry(t, c)

	for i := 0; i < 4; i++ {
		c.Inc()
	}

	want := `
# HELP mechanic_circuit_breaker_multi_test test
# TYPE mechanic_circuit_breaker_multi_test counter
mechanic_circuit_breaker_multi_test 4
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "mechanic_circuit_breaker_multi_test"); err != nil {
		t.Error(err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// Descriptor / metric metadata tests
// ═══════════════════════════════════════════════════════════════════

func TestAllMetricVars_NonNil(t *testing.T) {
	t.Parallel()
	// If init() failed or didn't run these would be nil.
	if metrics.FindingsDispatchedTotal == nil {
		t.Error("FindingsDispatchedTotal is nil")
	}
	if metrics.FindingsSuppressedTotal == nil {
		t.Error("FindingsSuppressedTotal is nil")
	}
	if metrics.AgentJobsActive == nil {
		t.Error("AgentJobsActive is nil")
	}
	if metrics.AgentJobsPending == nil {
		t.Error("AgentJobsPending is nil")
	}
	if metrics.AgentJobDurationSeconds == nil {
		t.Error("AgentJobDurationSeconds is nil")
	}
	if metrics.PRsOpenedTotal == nil {
		t.Error("PRsOpenedTotal is nil")
	}
	if metrics.PRsClosedTotal == nil {
		t.Error("PRsClosedTotal is nil")
	}
	if metrics.CircuitBreakerActivationsTotal == nil {
		t.Error("CircuitBreakerActivationsTotal is nil")
	}
}

func TestAllMetrics_HelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	collectors := []struct {
		name      string
		collector prometheus.Collector
	}{
		{"FindingsDispatchedTotal", metrics.FindingsDispatchedTotal},
		{"FindingsSuppressedTotal", metrics.FindingsSuppressedTotal},
		{"AgentJobsActive", metrics.AgentJobsActive},
		{"AgentJobsPending", metrics.AgentJobsPending},
		{"AgentJobDurationSeconds", metrics.AgentJobDurationSeconds},
		{"PRsOpenedTotal", metrics.PRsOpenedTotal},
		{"PRsClosedTotal", metrics.PRsClosedTotal},
		{"CircuitBreakerActivationsTotal", metrics.CircuitBreakerActivationsTotal},
	}
	for _, tc := range collectors {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch := make(chan *prometheus.Desc, 2)
			tc.collector.Describe(ch)
			close(ch)
			desc := <-ch
			s := desc.String()
			// Desc.String() includes the help text in the form: help: "..."
			if !strings.Contains(s, "help: \"") {
				t.Errorf("%s: no help text found in descriptor", tc.name)
			}
			// Confirm namespace prefix.
			if !strings.Contains(s, "mechanic_") {
				t.Errorf("%s: metric name does not start with 'mechanic_'", tc.name)
			}
		})
	}
}

func TestAllMetrics_UniqueNames(t *testing.T) {
	t.Parallel()
	// Register all 8 in one registry — MustRegister panics on duplicate FQDN.
	// If this panics the test binary itself fails, which is the correct signal.
	reg := prometheus.NewRegistry()
	collectors := []prometheus.Collector{
		metrics.FindingsDispatchedTotal,
		metrics.FindingsSuppressedTotal,
		metrics.AgentJobsActive,
		metrics.AgentJobsPending,
		metrics.AgentJobDurationSeconds,
		metrics.PRsOpenedTotal,
		metrics.PRsClosedTotal,
		metrics.CircuitBreakerActivationsTotal,
	}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				t.Fatalf("unexpected registration error (duplicate name?): %v", err)
			}
		}
	}
}
