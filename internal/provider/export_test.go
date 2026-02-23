// export_test.go exposes internal fields of SourceProviderReconciler for white-box testing.
// This file is compiled only during test runs (package provider, not provider_test).
package provider

import (
	"time"

	"github.com/lenaxia/k8s-mendabot/internal/cascade"
	"github.com/lenaxia/k8s-mendabot/internal/circuitbreaker"
)

// FirstSeen returns a snapshot copy of the firstSeen map for test inspection.
func (r *SourceProviderReconciler) FirstSeen() map[string]time.Time {
	r.initFirstSeen()
	return r.firstSeen.Copy()
}

// SetFirstSeenForTest sets a firstSeen entry for a test-controlled timestamp.
func (r *SourceProviderReconciler) SetFirstSeenForTest(key string, timestamp time.Time) {
	r.initFirstSeen()
	r.firstSeen.SetForTest(key, timestamp)
}

// SetCascadeCheckerForTest injects a cascade.Checker for testing.
// Must be called before the first Reconcile invocation.
func (r *SourceProviderReconciler) SetCascadeCheckerForTest(c cascade.Checker) {
	r.initCascadeOnce.Do(func() {
		r.cascadeChecker = c
	})
}

// SetCircuitBreakerForTest injects a CircuitBreaker for testing.
// Must be called before the first Reconcile invocation.
func (r *SourceProviderReconciler) SetCircuitBreakerForTest(cb *circuitbreaker.CircuitBreaker) {
	r.initCBOnce.Do(func() {
		r.circuitBreaker = cb
	})
}
