# Story: Infrastructure Failure Cascade Prevention

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Medium
**Status:** Not Started
**Estimated Effort:** 6 hours

---

## User Story

As a **mendabot operator**, I want the system to detect and suppress cascading infrastructure failures (like node failures causing multiple pod failures), so that redundant investigations are avoided and root cause analysis focuses on the actual infrastructure issue.

---

## Acceptance Criteria

- [ ] Cascade checker interface with `ShouldSuppress(ctx, finding, client) (bool, reason string)` method
- [ ] Node failure detection: suppress pod findings when node is `NotReady`
- [ ] Node pressure correlation: suppress OOMKilled pods when node has `MemoryPressure=True`
- [ ] Namespace-wide failure detection: > 50% pods failing → single namespace finding
- [ ] Configurable via `DISABLE_CASCADE_CHECK` environment variable
- [ ] Performance optimization: cache node state to reduce API calls
- [ ] Integration with SourceProviderReconciler before RemediationJob creation
- [ ] Unit tests for each cascade detection scenario
- [ ] Integration tests simulating infrastructure failures
- [ ] Documentation of cascade detection rules and trade-offs

---

## Technical Implementation

### Location: `internal/cascade/` (new package)

**Proposed Interface:**
```go
package cascade

type Checker interface {
    ShouldSuppress(ctx context.Context, finding *domain.Finding, client client.Client) (bool, string, error)
}

type Config struct {
    Enabled bool
    // Additional configuration for thresholds, caching, etc.
}
```

**Cascade Detection Rules:**

1. **Node Failure Detection**:
   ```go
   func (c *checker) isNodeFailure(ctx context.Context, podNamespace, podName string) (bool, *corev1.Node, error) {
       // Get pod to find node name
       // Get node and check NodeReady condition
       // Return true if node is NotReady
   }
   ```

2. **Node Pressure Correlation**:
   ```go
   func (c *checker) isNodePressureRelated(ctx context.Context, pod *corev1.Pod, finding *domain.Finding) bool {
       // Check if pod was OOMKilled
       // Check if node has MemoryPressure=True
       // Return true if correlated
   }
   ```

3. **Namespace-Wide Failure**:
   ```go
   func (c *checker) isNamespaceWideFailure(ctx context.Context, namespace string) (bool, int, int, error) {
       // List all pods in namespace
       // Count failing vs total pods
       // Return true if > 50% failing
   }
   ```

### Integration with SourceProviderReconciler

**Modification to `internal/provider/provider.go`:**
```go
// After extracting finding, before fingerprint calculation
if r.cascadeChecker != nil && finding != nil {
    suppress, reason, err := r.cascadeChecker.ShouldSuppress(ctx, finding, r.Client)
    if err != nil {
        r.Log.Error("cascade check error", zap.Error(err))
        // Continue with investigation rather than fail
    } else if suppress {
        r.Log.Info("suppressing finding due to cascade",
            zap.String("reason", reason),
            zap.String("kind", finding.Kind),
            zap.String("namespace", finding.Namespace),
        )
        return ctrl.Result{}, nil
    }
}
```

### Configuration

**Environment Variables:**
```bash
# Enable/disable cascade checking
DISABLE_CASCADE_CHECK=false  # Default: enabled

# Namespace-wide failure threshold (percentage)
CASCADE_NAMESPACE_THRESHOLD=50  # Default: 50%

# Node state cache TTL (seconds)
CASCADE_NODE_CACHE_TTL=30  # Default: 30 seconds
```

### Integration Points

- **SourceProviderReconciler**: Calls cascade checker before creating RemediationJob
- **Native Providers**: Provide additional context for cascade detection
- **Config**: Cascade checker configuration
- **Kubernetes API**: Node and pod state queries

### Testing Requirements

**Unit Tests** (`internal/cascade/cascade_test.go`):
- Node failure detection logic
- Node pressure correlation
- Namespace-wide failure calculation
- Cache implementation and TTL
- Configuration validation

**Integration Tests** (`internal/cascade/integration_test.go`):
- Simulated node failure with multiple pod failures
- Node pressure scenario with OOMKilled pods
- Namespace-wide degradation scenario
- Performance with caching enabled
- Disabled cascade checker behavior

---

## Tasks

- [ ] Design cascade checker interface and implementation
- [ ] Implement node failure detection
- [ ] Implement node pressure correlation
- [ ] Implement namespace-wide failure detection
- [ ] Add caching for performance optimization
- [ ] Integrate with SourceProviderReconciler
- [ ] Add configuration options
- [ ] Write unit tests for cascade detection logic
- [ ] Write integration tests for infrastructure failure scenarios
- [ ] Document cascade detection rules and trade-offs

---

## Dependencies

**Depends on:** epic01-controller (SourceProviderReconciler)
**Blocks:** STORY_06_monitoring (cascade detection metrics)

---

## Definition of Done

- [ ] All tests pass with `-race`
- [ ] `go vet` clean
- [ ] Cascade detection reduces redundant investigations
- [ ] Performance impact acceptable (caching reduces API calls)
- [ ] Configuration provides escape hatch for operators
- [ ] Logging provides clear visibility into suppression decisions
- [ ] Documentation explains detection rules and trade-offs