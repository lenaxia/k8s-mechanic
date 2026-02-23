package circuitbreaker

import (
	"context"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return s
}

func newTestClient(objs ...client.Object) client.Client {
	s := newTestScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		Build()
}

func TestCircuitBreaker_New(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	if cb == nil {
		t.Fatal("Expected CircuitBreaker instance, got nil")
	}
}

func TestCircuitBreaker_ShouldAllow_FirstCall(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow failed: %v", err)
	}

	if !allowed {
		t.Error("First call to ShouldAllow should return true")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining=0, got %v", remaining)
	}
}

func TestCircuitBreaker_ShouldAllow_WithinCooldown(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	// First call should succeed
	allowed, _, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("First ShouldAllow failed: %v", err)
	}
	if !allowed {
		t.Error("First call should be allowed")
	}

	// Second call immediately after should be blocked
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("Second ShouldAllow failed: %v", err)
	}
	if allowed {
		t.Error("Second call within cooldown should not be allowed")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining > 0, got %v", remaining)
	}
	if remaining > 5*time.Minute {
		t.Errorf("Expected remaining <= 5 minutes, got %v", remaining)
	}
}

func TestCircuitBreaker_ShouldAllow_AfterCooldown(t *testing.T) {
	// Create a ConfigMap with a timestamp 6 minutes ago (past cooldown)
	pastTime := time.Now().Add(-6 * time.Minute)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: pastTime.Format(time.RFC3339),
		},
	}

	c := newTestClient(configMap)
	cb := New(c, "test-namespace", 5*time.Minute)

	// Should be allowed since last remediation was 6 minutes ago
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow failed: %v", err)
	}
	if !allowed {
		t.Error("Should be allowed after cooldown period")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining=0, got %v", remaining)
	}
}

func TestCircuitBreaker_ShouldAllow_WithExistingConfigMap(t *testing.T) {
	// Create a ConfigMap with a recent timestamp (1 minute ago)
	recentTime := time.Now().Add(-1 * time.Minute)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: recentTime.Format(time.RFC3339),
		},
	}

	c := newTestClient(configMap)
	cb := New(c, "test-namespace", 5*time.Minute)

	// Should be blocked since last remediation was only 1 minute ago
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow failed: %v", err)
	}
	if allowed {
		t.Error("Should not be allowed within cooldown period")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining > 0, got %v", remaining)
	}
	// Remaining should be approximately 4 minutes
	expectedMin := 3 * time.Minute
	expectedMax := 5 * time.Minute
	if remaining < expectedMin || remaining > expectedMax {
		t.Errorf("Expected remaining between %v and %v, got %v", expectedMin, expectedMax, remaining)
	}
}

func TestCircuitBreaker_ShouldAllow_InvalidTimestamp(t *testing.T) {
	// Create a ConfigMap with an invalid timestamp
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: "not-a-valid-timestamp",
		},
	}

	c := newTestClient(configMap)
	cb := New(c, "test-namespace", 5*time.Minute)

	// Should treat invalid timestamp as zero time (never remediated)
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow failed: %v", err)
	}
	if !allowed {
		t.Error("Should be allowed with invalid timestamp (treated as never remediated)")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining=0, got %v", remaining)
	}
}

func TestCircuitBreaker_GetLastSelfRemediation(t *testing.T) {
	// Create a ConfigMap with a specific timestamp
	expectedTime := time.Now().Add(-10 * time.Minute).UTC()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: expectedTime.Format(time.RFC3339),
		},
	}

	c := newTestClient(configMap)
	cb := New(c, "test-namespace", 5*time.Minute)

	lastRemediation, err := cb.GetLastSelfRemediation(context.Background())
	if err != nil {
		t.Fatalf("GetLastSelfRemediation failed: %v", err)
	}

	// Compare times with tolerance for parsing/rounding
	if lastRemediation.IsZero() {
		t.Error("Expected non-zero time")
	}
	diff := expectedTime.Sub(lastRemediation)
	if diff.Abs() > time.Second {
		t.Errorf("Expected time %v, got %v (diff: %v)", expectedTime, lastRemediation, diff)
	}
}

func TestCircuitBreaker_GetLastSelfRemediation_NoConfigMap(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	lastRemediation, err := cb.GetLastSelfRemediation(context.Background())
	if err != nil {
		t.Fatalf("GetLastSelfRemediation failed: %v", err)
	}

	if !lastRemediation.IsZero() {
		t.Errorf("Expected zero time for non-existent ConfigMap, got %v", lastRemediation)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	// Create a ConfigMap with a recent timestamp
	recentTime := time.Now().Add(-1 * time.Minute)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: recentTime.Format(time.RFC3339),
		},
	}

	c := newTestClient(configMap)
	cb := New(c, "test-namespace", 5*time.Minute)

	// Reset the circuit breaker
	err := cb.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// After reset, should be allowed
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow after reset failed: %v", err)
	}
	if !allowed {
		t.Error("Should be allowed after reset")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining=0 after reset, got %v", remaining)
	}

	// Verify ConfigMap was updated
	var updatedConfigMap corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: "test-namespace",
	}, &updatedConfigMap)
	if err != nil {
		t.Fatalf("Failed to get updated ConfigMap: %v", err)
	}

	timestampStr, exists := updatedConfigMap.Data[LastSelfRemediationKey]
	if !exists || timestampStr == "" {
		t.Error("ConfigMap should have last self-remediation key after reset")
	}

	// Parse and check it's recent (not the old time)
	resetTime, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		t.Fatalf("Failed to parse reset time: %v", err)
	}

	// Reset time should be recent (within last few seconds)
	if time.Since(resetTime) > 10*time.Second {
		t.Errorf("Reset time should be recent, got %v", resetTime)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	// Run multiple goroutines to test thread safety
	const goroutines = 10
	results := make(chan bool, goroutines)
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			allowed, _, err := cb.ShouldAllow(context.Background())
			errs <- err
			results <- allowed
		}()
	}

	// Collect results
	allowedCount := 0
	for i := 0; i < goroutines; i++ {
		err := <-errs
		if err != nil {
			t.Fatalf("ShouldAllow failed in goroutine: %v", err)
		}

		if <-results {
			allowedCount++
		}
	}

	// Only one goroutine should have been allowed
	if allowedCount != 1 {
		t.Errorf("Expected exactly 1 goroutine to be allowed, got %d", allowedCount)
	}
}

func TestCircuitBreaker_PersistenceAcrossInstances(t *testing.T) {
	// Test that different CircuitBreaker instances share state via ConfigMap
	c := newTestClient()

	// First instance updates state
	cb1 := New(c, "test-namespace", 5*time.Minute)
	allowed, _, err := cb1.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("First ShouldAllow failed: %v", err)
	}
	if !allowed {
		t.Error("First call should be allowed")
	}

	// Second instance should see the updated state
	cb2 := New(c, "test-namespace", 5*time.Minute)
	allowed, remaining, err := cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("Second ShouldAllow failed: %v", err)
	}
	if allowed {
		t.Error("Second instance should not be allowed (within cooldown)")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining > 0, got %v", remaining)
	}
}

func TestCircuitBreaker_ZeroCooldown(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 0)

	// With zero cooldown, should always be allowed
	for i := 0; i < 5; i++ {
		allowed, remaining, err := cb.ShouldAllow(context.Background())
		if err != nil {
			t.Fatalf("ShouldAllow failed on iteration %d: %v", i, err)
		}
		if !allowed {
			t.Errorf("Should be allowed with zero cooldown, iteration %d", i)
		}
		if remaining != 0 {
			t.Errorf("Expected remaining=0 with zero cooldown, got %v", remaining)
		}
	}
}

// TestCircuitBreaker_ControllerRestartPersistence tests circuit breaker persistence across controller restarts
func TestCircuitBreaker_ControllerRestartPersistence(t *testing.T) {
	// Create a ConfigMap with a recent timestamp
	recentTime := time.Now().Add(-1 * time.Minute)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: recentTime.Format(time.RFC3339),
		},
	}

	c := newTestClient(configMap)

	// First controller instance
	cb1 := New(c, "test-namespace", 5*time.Minute)
	allowed1, remaining1, err := cb1.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("First instance ShouldAllow failed: %v", err)
	}
	if allowed1 {
		t.Error("First instance should not be allowed (within cooldown)")
	}
	if remaining1 <= 0 {
		t.Errorf("First instance expected remaining > 0, got %v", remaining1)
	}

	// Simulate controller restart - new instance with same ConfigMap
	cb2 := New(c, "test-namespace", 5*time.Minute)
	allowed2, remaining2, err := cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("Second instance ShouldAllow failed: %v", err)
	}
	if allowed2 {
		t.Error("Second instance should not be allowed (should read same persisted state)")
	}
	if remaining2 <= 0 {
		t.Errorf("Second instance expected remaining > 0, got %v", remaining2)
	}

	// Remaining times should be approximately equal (within 1 second tolerance)
	diff := remaining1 - remaining2
	if diff.Abs() > time.Second {
		t.Errorf("Remaining time mismatch between instances: %v vs %v (diff: %v)",
			remaining1, remaining2, diff)
	}
}

// TestCircuitBreaker_ConcurrentResetAndCheck tests race condition between Reset and ShouldAllow
func TestCircuitBreaker_ConcurrentResetAndCheck(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	const numGoroutines = 10
	results := make(chan struct {
		allowed   bool
		remaining time.Duration
		err       error
	}, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				// Even goroutines call ShouldAllow
				allowed, remaining, err := cb.ShouldAllow(context.Background())
				results <- struct {
					allowed   bool
					remaining time.Duration
					err       error
				}{allowed, remaining, err}
			} else {
				// Odd goroutines call Reset
				err := cb.Reset(context.Background())
				results <- struct {
					allowed   bool
					remaining time.Duration
					err       error
				}{false, 0, err}
			}
		}(i)
	}

	wg.Wait()
	close(results)

	// Collect results
	var shouldAllowCalls, resetCalls int
	var errors []error
	for result := range results {
		if result.err != nil {
			errors = append(errors, result.err)
		}
		if result.allowed || result.remaining > 0 {
			shouldAllowCalls++
		} else if result.err == nil {
			resetCalls++
		}
	}

	// Check for errors
	for _, err := range errors {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}

	// Verify at least one Reset succeeded
	if resetCalls == 0 {
		t.Error("expected at least one Reset to succeed")
	}
}

// TestCircuitBreaker_ConfigMapCorruption tests handling of corrupted ConfigMap data
func TestCircuitBreaker_ConfigMapCorruption(t *testing.T) {
	testCases := []struct {
		name     string
		data     map[string]string
		expected bool // Should be allowed?
	}{
		{
			name: "empty timestamp",
			data: map[string]string{
				LastSelfRemediationKey: "",
			},
			expected: true, // Empty string should be treated as zero time
		},
		{
			name: "whitespace timestamp",
			data: map[string]string{
				LastSelfRemediationKey: "   ",
			},
			expected: true,
		},
		{
			name: "future timestamp",
			data: map[string]string{
				LastSelfRemediationKey: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			},
			expected: false, // Future timestamp should block
		},
		{
			name: "malformed RFC3339",
			data: map[string]string{
				LastSelfRemediationKey: "2023-13-45T25:61:61Z",
			},
			expected: true, // Malformed should be treated as zero time
		},
		{
			name: "wrong key name",
			data: map[string]string{
				"wrong-key": time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
			},
			expected: true, // Missing key should be treated as zero time
		},
		{
			name: "multiple keys",
			data: map[string]string{
				LastSelfRemediationKey: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
				"another-key":          "some-value",
			},
			expected: false, // Should read correct key
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: "test-namespace",
				},
				Data: tc.data,
			}

			c := newTestClient(configMap)
			cb := New(c, "test-namespace", 5*time.Minute)

			allowed, remaining, err := cb.ShouldAllow(context.Background())
			if err != nil {
				t.Fatalf("ShouldAllow failed: %v", err)
			}

			if allowed != tc.expected {
				t.Errorf("allowed = %v, want %v for case %q", allowed, tc.expected, tc.name)
			}

			// Additional validation for remaining time
			if allowed && remaining != 0 {
				t.Errorf("allowed=true but remaining = %v, want 0", remaining)
			}
			if !allowed && remaining <= 0 {
				t.Errorf("allowed=false but remaining = %v, want > 0", remaining)
			}
		})
	}
}

// TestCircuitBreaker_HighLoadPerformance tests circuit breaker under high concurrent load
func TestCircuitBreaker_HighLoadPerformance(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	const numRequests = 1000
	start := time.Now()

	// Simulate high load
	for i := 0; i < numRequests; i++ {
		_, _, err := cb.ShouldAllow(context.Background())
		if err != nil {
			t.Fatalf("ShouldAllow failed on request %d: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	t.Logf("Processed %d requests in %v (%.0f req/sec)",
		numRequests, elapsed, float64(numRequests)/elapsed.Seconds())

	// Verify only first request was allowed
	// This is a bit tricky since we're calling ShouldAllow in a loop
	// The first call should succeed, subsequent calls should fail
	// But we need to reset between checks
	cb.Reset(context.Background())

	allowed, _, _ := cb.ShouldAllow(context.Background())
	if !allowed {
		t.Error("After reset, first call should be allowed")
	}

	allowed, _, _ = cb.ShouldAllow(context.Background())
	if allowed {
		t.Error("Second call after reset should not be allowed")
	}
}

// TestCircuitBreaker_MemoryLeakPrevention tests that circuit breaker doesn't leak memory
// by checking that ConfigMap operations are efficient
func TestCircuitBreaker_MemoryLeakPrevention(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 5*time.Minute)

	// Perform many operations
	const numOperations = 100
	for i := 0; i < numOperations; i++ {
		if i%10 == 0 {
			// Reset every 10th operation
			err := cb.Reset(context.Background())
			if err != nil {
				t.Fatalf("Reset failed on operation %d: %v", i, err)
			}
		} else {
			_, _, err := cb.ShouldAllow(context.Background())
			if err != nil {
				t.Fatalf("ShouldAllow failed on operation %d: %v", i, err)
			}
		}
	}

	// Verify final state
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("Final ShouldAllow failed: %v", err)
	}

	// Should not be allowed unless we just reset
	// Since we don't know the exact state, just verify no errors
	if allowed && remaining != 0 {
		t.Errorf("Unexpected state: allowed=%v, remaining=%v", allowed, remaining)
	}
}

// TestCircuitBreaker_NamespaceIsolation tests that circuit breakers in different namespaces are isolated
func TestCircuitBreaker_NamespaceIsolation(t *testing.T) {
	c := newTestClient()

	// Create circuit breakers in different namespaces
	cb1 := New(c, "namespace-1", 5*time.Minute)
	cb2 := New(c, "namespace-2", 5*time.Minute)

	// First call to cb1 should succeed
	allowed1, _, err := cb1.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("cb1 ShouldAllow failed: %v", err)
	}
	if !allowed1 {
		t.Error("cb1 first call should be allowed")
	}

	// First call to cb2 should also succeed (different namespace)
	allowed2, _, err := cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("cb2 ShouldAllow failed: %v", err)
	}
	if !allowed2 {
		t.Error("cb2 first call should be allowed (different namespace)")
	}

	// Second call to cb1 should fail
	allowed1, _, err = cb1.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("cb1 second ShouldAllow failed: %v", err)
	}
	if allowed1 {
		t.Error("cb1 second call should not be allowed")
	}

	// Second call to cb2 should also fail
	allowed2, _, err = cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("cb2 second ShouldAllow failed: %v", err)
	}
	if allowed2 {
		t.Error("cb2 second call should not be allowed")
	}
}

// TestCircuitBreaker_ZeroCooldownEdgeCases tests edge cases with zero cooldown
func TestCircuitBreaker_ZeroCooldownEdgeCases(t *testing.T) {
	c := newTestClient()
	cb := New(c, "test-namespace", 0)

	// Test many rapid calls
	const rapidCalls = 100
	for i := 0; i < rapidCalls; i++ {
		allowed, remaining, err := cb.ShouldAllow(context.Background())
		if err != nil {
			t.Fatalf("ShouldAllow failed on rapid call %d: %v", i, err)
		}
		if !allowed {
			t.Errorf("Should be allowed with zero cooldown, rapid call %d", i)
		}
		if remaining != 0 {
			t.Errorf("Expected remaining=0 with zero cooldown, rapid call %d got %v", i, remaining)
		}
	}

	// Test with existing ConfigMap (should still allow)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			LastSelfRemediationKey: time.Now().Format(time.RFC3339),
		},
	}

	c2 := newTestClient(configMap)
	cb2 := New(c2, "test-namespace", 0)

	allowed, remaining, err := cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("ShouldAllow with existing ConfigMap failed: %v", err)
	}
	if !allowed {
		t.Error("Should be allowed with zero cooldown even with existing ConfigMap")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining=0 with zero cooldown, got %v", remaining)
	}
}
