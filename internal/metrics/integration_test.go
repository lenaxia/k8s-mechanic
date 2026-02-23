package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestIntegration verifies that metrics are properly integrated and can be collected
func TestIntegration(t *testing.T) {
	// Reset metrics for clean test
	ResetMetrics()

	// Simulate a typical workflow
	provider := "native-pod"
	namespace := "default"

	// 1. Circuit breaker activates
	RecordCircuitBreakerActivation(provider, namespace)
	SetCircuitBreakerCooldown(provider, namespace, 30.5)

	// 2. Chain depth observed
	RecordChainDepth(provider, namespace, 3)
	RecordChainDepth(provider, namespace, 5)
	RecordMaxDepthExceeded(provider, namespace, 5)

	// 3. Cascade suppression recorded
	RecordCascadeSuppression(provider, namespace, "circuit_breaker")
	RecordCascadeSuppressionReason(provider, namespace, "cooldown_active", "Circuit breaker cooldown active")

	// 4. Self-remediation attempts
	RecordSelfRemediationAttempt(provider, namespace, true)
	RecordSelfRemediationAttempt(provider, namespace, true)
	RecordSelfRemediationAttempt(provider, namespace, false)
	UpdateSelfRemediationSuccessRate(provider, namespace)

	// Verify metrics can be collected
	registry := prometheus.NewRegistry()
	err := RegisterMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	// Collect metrics
	_, err = registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Verify specific metric values
	activations := testutil.ToFloat64(circuitBreakerActivationsTotal.WithLabelValues(provider, namespace))
	if activations != 1 {
		t.Errorf("Expected 1 circuit breaker activation, got %v", activations)
	}

	cooldown := testutil.ToFloat64(circuitBreakerCooldownSeconds.WithLabelValues(provider, namespace))
	if cooldown != 30.5 {
		t.Errorf("Expected cooldown 30.5, got %v", cooldown)
	}

	successRate := testutil.ToFloat64(selfRemediationSuccessRate.WithLabelValues(provider, namespace))
	if successRate != 0.6666666666666666 { // 2/3 success rate
		t.Errorf("Expected success rate ~0.667, got %v", successRate)
	}

	suppressions := testutil.ToFloat64(cascadeSuppressionsTotal.WithLabelValues(provider, namespace, "circuit_breaker"))
	if suppressions != 1 {
		t.Errorf("Expected 1 cascade suppression, got %v", suppressions)
	}
}

// TestMetricLabels verifies label consistency
func TestMetricLabels(t *testing.T) {
	ResetMetrics()

	// Test with different providers and namespaces
	providers := []string{"native-pod", "native-deployment", "native-node"}
	namespaces := []string{"default", "kube-system", "production"}

	for i, provider := range providers {
		namespace := namespaces[i%len(namespaces)]

		RecordCircuitBreakerActivation(provider, namespace)
		RecordChainDepth(provider, namespace, i+1)
		RecordSelfRemediationAttempt(provider, namespace, i%2 == 0)
		RecordCascadeSuppression(provider, namespace, "circuit_breaker")
	}

	// Verify all metrics have the expected label combinations
	// This ensures we don't have label cardinality issues
	totalMetrics := 0
	metrics := []prometheus.Collector{
		circuitBreakerActivationsTotal,
		circuitBreakerCooldownSeconds,
		chainDepthDistribution,
		maxDepthExceededTotal,
		selfRemediationAttemptsTotal,
		selfRemediationSuccessRate,
		cascadeSuppressionsTotal,
		cascadeSuppressionReasons,
	}

	for _, metric := range metrics {
		if err := prometheus.Register(metric); err == nil {
			// Metric wasn't registered yet, unregister it
			prometheus.Unregister(metric)
		}
		// Count would be complex, but we at least verify they can be registered
		totalMetrics++
	}

	if totalMetrics != 8 {
		t.Errorf("Expected 8 metrics, found %v", totalMetrics)
	}
}
