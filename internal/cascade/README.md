# Cascade Checker

The Cascade Checker implements infrastructure failure cascade prevention for mendabot. It detects and suppresses findings that are likely caused by broader infrastructure issues to avoid redundant investigations.

## Detection Rules

### 1. Node Failure Detection
- **Rule**: Suppress pod findings when the node is `NotReady`
- **Logic**: Checks if the pod's node has `NodeReady` condition with status `False` or `Unknown`
- **Reason**: Pod failures on unavailable nodes are symptoms, not root causes

### 2. Node Pressure Correlation
- **Rule**: Suppress OOMKilled pods when node has `MemoryPressure=True`
- **Logic**: Checks if pod errors contain "OOMKilled" and node has `NodeMemoryPressure` condition with status `True`
- **Reason**: OOMKilled pods on memory-constrained nodes are symptoms of node-level issues

### 3. Namespace-Wide Failure Detection
- **Rule**: If > threshold% of pods in a namespace are failing, suppress individual pod findings
- **Logic**: Counts failing pods vs total pods in namespace, suppresses if failure percentage exceeds threshold
- **Reason**: Widespread failures suggest namespace-level or cluster-level issues
- **Default Threshold**: 50% (configurable via `CASCADE_NAMESPACE_THRESHOLD`)

## Configuration

### Environment Variables
- `DISABLE_CASCADE_CHECK`: Disable infrastructure cascade detection (default: `false`)
- `CASCADE_NAMESPACE_THRESHOLD`: Percentage of failing pods to trigger namespace-wide suppression (default: `50`)
- `CASCADE_NODE_CACHE_TTL_SECONDS`: Node state cache TTL in seconds (default: `30`)

### Performance Optimization
- **Node State Caching**: Node conditions are cached to reduce Kubernetes API calls
- **Cache TTL**: Configurable via `CASCADE_NODE_CACHE_TTL_SECONDS`
- **Thread Safety**: Cache is protected by read-write mutex for concurrent access

## Integration

### SourceProviderReconciler Integration
The cascade checker is integrated into `SourceProviderReconciler.Reconcile()`:
1. After extracting a finding from a watched object
2. Before fingerprint calculation and deduplication
3. If cascade checker suppresses the finding, reconciliation stops early

### Metrics
When a finding is suppressed due to cascade detection:
- `mendabot_cascade_suppressions_total` is incremented with label `reason="infrastructure_cascade"`
- `mendabot_cascade_suppression_reasons` records the suppression reason

## Usage Example

```go
import "github.com/lenaxia/k8s-mendabot/internal/cascade"

// Create cascade checker configuration
cfg := cascade.Config{
    Enabled:                 true,
    NamespaceFailurePercent: 50,
    NodeCacheTTL:            30 * time.Second,
}

// Create checker
checker, err := cascade.NewChecker(cfg)
if err != nil {
    // Handle error
}

// Check if finding should be suppressed
suppress, reason, err := checker.ShouldSuppress(ctx, finding, k8sClient)
if err != nil {
    // Handle error
}
if suppress {
    // Finding should be suppressed due to infrastructure cascade
    log.Info("Suppressing finding", zap.String("reason", reason))
}
```

## Testing

Unit tests cover:
- Node failure detection scenarios
- Node pressure correlation with OOMKilled pods
- Namespace-wide failure calculation
- Cache implementation and TTL behavior
- Disabled cascade checker behavior

Run tests with:
```bash
go test ./internal/cascade/...
```