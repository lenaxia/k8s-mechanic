package provider

import "time"

// SetFirstSeenForTest sets a firstSeen entry with a specific timestamp for testing.
func (r *SourceProviderReconciler) SetFirstSeenForTest(fp string, t time.Time) {
	r.initFirstSeen()
	r.firstSeen.SetForTest(fp, t)
}

// FirstSeen returns a snapshot of the firstSeen map for assertions in tests.
func (r *SourceProviderReconciler) FirstSeen() map[string]time.Time {
	r.initFirstSeen()
	return r.firstSeen.Copy()
}

// NewBoundedMapForTest creates a BoundedMap for use in unit tests with explicit
// configuration (bypasses the default constructor's zero-value adjustments).
func NewBoundedMapForTest(maxSize int, ttl, cleanupInterval time.Duration) *BoundedMap {
	return NewBoundedMap(maxSize, ttl, cleanupInterval)
}

// SetLastCleanupForTest overwrites lastCleanup for testing TTL/cleanup behaviour.
func (m *BoundedMap) SetLastCleanupForTest(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastCleanup = t
}
