package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCircuitBreakerMetrics(t *testing.T) {
	// Reset metrics for clean test
	ResetMetrics()

	t.Run("CircuitBreakerActivationsTotal", func(t *testing.T) {
		RecordCircuitBreakerActivation("test-provider", "test-namespace")
		RecordCircuitBreakerActivation("test-provider", "test-namespace")
		RecordCircuitBreakerActivation("another-provider", "another-namespace")

		count := testutil.ToFloat64(circuitBreakerActivationsTotal.WithLabelValues("test-provider", "test-namespace"))
		if count != 2 {
			t.Errorf("Expected 2 circuit breaker activations, got %v", count)
		}

		count = testutil.ToFloat64(circuitBreakerActivationsTotal.WithLabelValues("another-provider", "another-namespace"))
		if count != 1 {
			t.Errorf("Expected 1 circuit breaker activation, got %v", count)
		}
	})

	t.Run("CircuitBreakerCooldownSeconds", func(t *testing.T) {
		SetCircuitBreakerCooldown("test-provider", "test-namespace", 30.5)
		SetCircuitBreakerCooldown("another-provider", "another-namespace", 15.0)

		value := testutil.ToFloat64(circuitBreakerCooldownSeconds.WithLabelValues("test-provider", "test-namespace"))
		if value != 30.5 {
			t.Errorf("Expected cooldown 30.5, got %v", value)
		}

		value = testutil.ToFloat64(circuitBreakerCooldownSeconds.WithLabelValues("another-provider", "another-namespace"))
		if value != 15.0 {
			t.Errorf("Expected cooldown 15.0, got %v", value)
		}

		// Test clearing cooldown
		ClearCircuitBreakerCooldown("test-provider", "test-namespace")
		value = testutil.ToFloat64(circuitBreakerCooldownSeconds.WithLabelValues("test-provider", "test-namespace"))
		if value != 0 {
			t.Errorf("Expected cleared cooldown 0, got %v", value)
		}
	})
}

func TestChainDepthMetrics(t *testing.T) {
	ResetMetrics()

	t.Run("ChainDepthDistribution", func(t *testing.T) {
		RecordChainDepth("test-provider", "test-namespace", 1)
		RecordChainDepth("test-provider", "test-namespace", 2)
		RecordChainDepth("test-provider", "test-namespace", 5)
		RecordChainDepth("another-provider", "another-namespace", 3)

		// Count observations in histogram - CollectAndCount returns metric count, not observation count
		// We need to check the metric was created with observations
		// The histogram creates multiple time series (buckets + sum + count)
		count := testutil.CollectAndCount(chainDepthDistribution)
		if count == 0 {
			t.Errorf("Expected histogram metrics to be created, got %v", count)
		}
	})

	t.Run("MaxDepthExceededTotal", func(t *testing.T) {
		RecordMaxDepthExceeded("test-provider", "test-namespace", 3)
		RecordMaxDepthExceeded("test-provider", "test-namespace", 3)
		RecordMaxDepthExceeded("another-provider", "another-namespace", 5)

		count := testutil.ToFloat64(maxDepthExceededTotal.WithLabelValues("test-provider", "test-namespace", "3"))
		if count != 2 {
			t.Errorf("Expected 2 max depth exceeded events for depth 3, got %v", count)
		}

		count = testutil.ToFloat64(maxDepthExceededTotal.WithLabelValues("another-provider", "another-namespace", "5"))
		if count != 1 {
			t.Errorf("Expected 1 max depth exceeded event for depth 5, got %v", count)
		}
	})
}

func TestSelfRemediationMetrics(t *testing.T) {
	ResetMetrics()

	t.Run("SelfRemediationAttemptsTotal", func(t *testing.T) {
		RecordSelfRemediationAttempt("test-provider", "test-namespace", true)
		RecordSelfRemediationAttempt("test-provider", "test-namespace", false)
		RecordSelfRemediationAttempt("another-provider", "another-namespace", true)

		successCount := testutil.ToFloat64(selfRemediationAttemptsTotal.WithLabelValues("test-provider", "test-namespace", "true"))
		if successCount != 1 {
			t.Errorf("Expected 1 successful self-remediation attempt, got %v", successCount)
		}

		failureCount := testutil.ToFloat64(selfRemediationAttemptsTotal.WithLabelValues("test-provider", "test-namespace", "false"))
		if failureCount != 1 {
			t.Errorf("Expected 1 failed self-remediation attempt, got %v", failureCount)
		}

		successCount = testutil.ToFloat64(selfRemediationAttemptsTotal.WithLabelValues("another-provider", "another-namespace", "true"))
		if successCount != 1 {
			t.Errorf("Expected 1 successful self-remediation attempt for another provider, got %v", successCount)
		}
	})

	t.Run("SelfRemediationSuccessRate", func(t *testing.T) {
		// Use ResetMetrics to clear all state
		ResetMetrics()

		// Record 3 successes and 1 failure
		RecordSelfRemediationAttempt("test-provider", "test-namespace", true)
		RecordSelfRemediationAttempt("test-provider", "test-namespace", true)
		RecordSelfRemediationAttempt("test-provider", "test-namespace", true)
		RecordSelfRemediationAttempt("test-provider", "test-namespace", false)

		// Update success rate (75% success)
		UpdateSelfRemediationSuccessRate("test-provider", "test-namespace")

		rate := testutil.ToFloat64(selfRemediationSuccessRate.WithLabelValues("test-provider", "test-namespace"))
		if rate != 0.75 {
			t.Errorf("Expected success rate 0.75, got %v", rate)
		}
	})
}

func TestCascadeSuppressionMetrics(t *testing.T) {
	ResetMetrics()

	t.Run("CascadeSuppressionsTotal", func(t *testing.T) {
		RecordCascadeSuppression("test-provider", "test-namespace", "circuit_breaker")
		RecordCascadeSuppression("test-provider", "test-namespace", "circuit_breaker")
		RecordCascadeSuppression("test-provider", "test-namespace", "max_depth")
		RecordCascadeSuppression("another-provider", "another-namespace", "stabilisation_window")

		count := testutil.ToFloat64(cascadeSuppressionsTotal.WithLabelValues("test-provider", "test-namespace", "circuit_breaker"))
		if count != 2 {
			t.Errorf("Expected 2 circuit breaker suppressions, got %v", count)
		}

		count = testutil.ToFloat64(cascadeSuppressionsTotal.WithLabelValues("test-provider", "test-namespace", "max_depth"))
		if count != 1 {
			t.Errorf("Expected 1 max depth suppression, got %v", count)
		}

		count = testutil.ToFloat64(cascadeSuppressionsTotal.WithLabelValues("another-provider", "another-namespace", "stabilisation_window"))
		if count != 1 {
			t.Errorf("Expected 1 stabilisation window suppression, got %v", count)
		}
	})

	t.Run("CascadeSuppressionReasons", func(t *testing.T) {
		RecordCascadeSuppressionReason("test-provider", "test-namespace", "cooldown_active", "Circuit breaker cooldown active")
		RecordCascadeSuppressionReason("test-provider", "test-namespace", "chain_too_deep", "Chain depth 5 exceeds maximum 3")
		RecordCascadeSuppressionReason("another-provider", "another-namespace", "cooldown_active", "Circuit breaker cooldown active")

		count := testutil.ToFloat64(cascadeSuppressionReasons.WithLabelValues("test-provider", "test-namespace", "cooldown_active"))
		if count != 1 {
			t.Errorf("Expected 1 cooldown_active reason, got %v", count)
		}

		count = testutil.ToFloat64(cascadeSuppressionReasons.WithLabelValues("test-provider", "test-namespace", "chain_too_deep"))
		if count != 1 {
			t.Errorf("Expected 1 chain_too_deep reason, got %v", count)
		}
	})
}

func TestConcurrentMetricUpdates(t *testing.T) {
	ResetMetrics()

	// Test concurrent updates to ensure thread safety
	done := make(chan bool)
	numGoroutines := 10
	updatesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < updatesPerGoroutine; j++ {
				provider := "concurrent-provider"
				namespace := "test-namespace"

				RecordCircuitBreakerActivation(provider, namespace)
				RecordChainDepth(provider, namespace, id%5+1)
				RecordSelfRemediationAttempt(provider, namespace, j%2 == 0)
				RecordCascadeSuppression(provider, namespace, "circuit_breaker")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify total counts
	expectedActivations := float64(numGoroutines * updatesPerGoroutine)
	actualActivations := testutil.ToFloat64(circuitBreakerActivationsTotal.WithLabelValues("concurrent-provider", "test-namespace"))
	if actualActivations != expectedActivations {
		t.Errorf("Expected %v circuit breaker activations, got %v", expectedActivations, actualActivations)
	}

	expectedSuppressions := float64(numGoroutines * updatesPerGoroutine)
	actualSuppressions := testutil.ToFloat64(cascadeSuppressionsTotal.WithLabelValues("concurrent-provider", "test-namespace", "circuit_breaker"))
	if actualSuppressions != expectedSuppressions {
		t.Errorf("Expected %v cascade suppressions, got %v", expectedSuppressions, actualSuppressions)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Test that our metrics package registers correctly
	// This is a simple test to verify the package compiles and initializes
	registry := prometheus.NewRegistry()

	err := RegisterMetrics(registry)
	if err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	// The metrics should be registered now
	// We can't easily test which specific metrics are registered
	// because they're package-private variables
	t.Log("Metrics registration test passed")
}
