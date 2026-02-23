# Story: Persistent Circuit Breaker with ConfigMap State

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 4 hours

---

## User Story

As a **mendabot operator**, I want a circuit breaker that prevents rapid cascades of self-remediations, with persistent state that survives controller restarts, so that LLM quota is not exhausted by infinite retry loops.

---

## Acceptance Criteria

- [x] Circuit breaker implementation in `internal/circuitbreaker/` package
- [x] Persistent state stored in ConfigMap `mendabot-circuit-breaker`
- [x] Thread-safe operations with mutex protection
- [x] Configurable cooldown via `SELF_REMEDIATION_COOLDOWN_SECONDS`
- [x] Only applies to self-remediations (`IsSelfRemediation: true`)
- [x] Non-self-remediations bypass circuit breaker entirely
- [x] State survives controller restarts (reads from ConfigMap on initialization)
- [x] Cooldown period respected with `RequeueAfter` for blocked reconciles
- [x] Zero cooldown disables circuit breaker (always allow)
- [x] Reset functionality for manual intervention
- [x] Unit tests for circuit breaker logic
- [x] Integration tests simulating controller restarts
- [x] Concurrent reconciliation handled correctly

---

## Technical Implementation

### Location: `internal/circuitbreaker/circuitbreaker.go`

**Key Implementation Details:**

1. **Circuit Breaker Struct** (lines 32-43):
   ```go
   type CircuitBreaker struct {
       client    client.Client
       namespace string
       cooldown  time.Duration
   
       mutex               sync.RWMutex
       lastSelfRemediation time.Time
       initialized         bool
   }
   ```

2. **ShouldAllow Logic** (lines 54-87):
   ```go
   func (cb *CircuitBreaker) ShouldAllow(ctx context.Context) (bool, time.Duration, error) {
       // If cooldown is zero, circuit breaker is disabled
       if cb.cooldown == 0 {
           return true, 0, nil
       }
   
       // Check cooldown
       timeSinceLast := time.Since(cb.lastSelfRemediation)
       if timeSinceLast < cb.cooldown {
           remaining := cb.cooldown - timeSinceLast
           return false, remaining, nil
       }
   
       // Update last self-remediation time
       cb.lastSelfRemediation = time.Now()
       if err := cb.saveState(ctx); err != nil {
           return false, 0, fmt.Errorf("failed to save circuit breaker state: %w", err)
       }
   
       return true, 0, nil
   }
   ```

3. **ConfigMap Persistence** (lines 103-139, 141-189):
   - `loadState()`: Reads from ConfigMap on initialization
   - `saveState()`: Updates ConfigMap with RFC3339 timestamp
   - Creates ConfigMap if it doesn't exist

4. **Integration with SourceProviderReconciler** (`internal/provider/provider.go` lines 118-152):
   ```go
   if finding.IsSelfRemediation {
       // Initialize circuit breaker if not already initialized
       if r.circuitBreaker == nil {
           r.circuitBreaker = circuitbreaker.New(r.Client, r.Cfg.AgentNamespace, r.Cfg.SelfRemediationCooldown)
       }
   
       allowed, remaining, err := r.circuitBreaker.ShouldAllow(ctx)
       if err != nil {
           r.Log.Error("circuit breaker error", zap.Error(err))
           return ctrl.Result{}, fmt.Errorf("circuit breaker error: %w", err)
       }
   
       if !allowed {
           if r.Log != nil {
               r.Log.Info("circuit breaker: skipping self-remediation due to cooldown",
                   zap.String("fingerprint", fp[:12]),
                   zap.Duration("remaining", remaining),
                   zap.Int("chainDepth", finding.ChainDepth),
               )
           }
           return ctrl.Result{RequeueAfter: remaining}, nil
       }
   }
   ```

### Configuration

**Environment Variables** (`internal/config/config.go` lines 120-133):
```go
cooldownStr := os.Getenv("SELF_REMEDIATION_COOLDOWN_SECONDS")
if cooldownStr == "" {
    cfg.SelfRemediationCooldown = 300 * time.Second // 5 minutes default
} else {
    n, err := strconv.Atoi(cooldownStr)
    // ... validation
    cfg.SelfRemediationCooldown = time.Duration(n) * time.Second
}
```

### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mendabot-circuit-breaker
  namespace: <agent-namespace>
  labels:
    app.kubernetes.io/managed-by: mendabot
    app.kubernetes.io/component: circuit-breaker
data:
  last-self-remediation: "2026-02-23T10:00:00Z"  # RFC3339 timestamp
  agent-namespace: "mendabot"
```

### Integration Points

- **SourceProviderReconciler**: Initializes and uses circuit breaker for self-remediations
- **Config**: Provides `SelfRemediationCooldown` duration
- **Kubernetes API**: ConfigMap for persistent state storage

### Testing Requirements

**Unit Tests** (`internal/circuitbreaker/circuitbreaker_test.go`):
- Cooldown logic with various durations
- ConfigMap persistence (creation, reading, updating)
- Thread safety with concurrent access
- Zero cooldown (disabled) behavior
- Reset functionality

**Integration Tests** (`internal/provider/circuitbreaker_integration_test.go`):
- Controller restart simulation
- Persistent state across reconciler instances
- Concurrent reconciliation scenarios
- Non-self-remediation bypass

---

## Tasks

- [x] Implement circuit breaker package with mutex protection
- [x] Add ConfigMap persistence for state survival
- [x] Integrate with SourceProviderReconciler for self-remediations
- [x] Add configuration for cooldown duration
- [x] Implement reset functionality for manual intervention
- [x] Write unit tests for circuit breaker logic
- [x] Write integration tests for controller restart scenarios
- [x] Test concurrent reconciliation handling

---

## Dependencies

**Depends on:** STORY_01_self_remediation_detection, STORY_02_chain_depth_tracking
**Blocks:** STORY_06_monitoring (circuit breaker metrics)

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] No data races in concurrent access
- [x] State persists across controller restarts
- [x] Configurable cooldown with safe defaults
- [x] Non-self-remediations bypass circuit breaker
- [x] Reset functionality works correctly
- [x] Logging provides clear operational visibility