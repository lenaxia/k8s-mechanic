# Story: Chain Depth Tracking and Max Depth Enforcement

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want the system to track how deep a self-remediation cascade has gone and enforce a maximum depth limit, so that infinite recursion is prevented and resources are not wasted.

---

## Acceptance Criteria

- [x] `ChainDepth` field added to `domain.Finding` and `RemediationJobSpec`
- [x] Chain depth incremented on each self-remediation level (0 = not self-remediation)
- [x] Depth read atomically from owner RemediationJob to prevent races
- [x] Fallback to annotation for backward compatibility
- [x] Max depth enforced via `SELF_REMEDIATION_MAX_DEPTH` config
- [x] Findings exceeding max depth return `nil` (skip investigation)
- [x] Deep cascades (depth > 2) logged as warnings
- [x] Unit tests verify depth calculation and increment logic
- [x] Integration tests simulate cascade scenarios with concurrent updates
- [x] Thread-safe implementation for concurrent reconciliation

---

## Technical Implementation

### Location: `internal/provider/native/job.go`

**Key Implementation Details:**

1. **Chain Depth Fields**:
   - `domain.Finding.ChainDepth` (line 122 in `internal/domain/provider.go`)
   - `RemediationJobSpec.ChainDepth` (line 125 in `api/v1alpha1/remediationjob_types.go`)

2. **Atomic Depth Reading** (lines 82-88):
   ```go
   // Try to get chain depth from owner RemediationJob first (atomic source)
   parentDepth, err := p.getChainDepthFromOwner(context.Background(), job)
   if err != nil {
       // Fall back to annotation for backward compatibility
       chainDepth = p.getChainDepthFromAnnotation(job)
   } else {
       chainDepth = parentDepth
   }
   
   // Increment chain depth for self-remediation
   chainDepth++
   ```

3. **Max Depth Enforcement** (lines 94-96):
   ```go
   // Check if we've exceeded max depth
   if chainDepth > p.cfg.SelfRemediationMaxDepth {
       return nil, nil
   }
   ```

4. **Owner-Based Depth Reading** (lines 157-182):
   ```go
   func (p *jobProvider) getChainDepthFromOwner(ctx context.Context, job *batchv1.Job) (int, error) {
       // Find RemediationJob owner and read its ChainDepth field
   }
   ```

5. **Annotation Fallback** (lines 184-195):
   ```go
   func (p *jobProvider) getChainDepthFromAnnotation(job *batchv1.Job) int {
       // Read from annotation for backward compatibility
   }
   ```

### Configuration

**Environment Variables** (`internal/config/config.go` lines 105-118):
```go
depthStr := os.Getenv("SELF_REMEDIATION_MAX_DEPTH")
if depthStr == "" {
    cfg.SelfRemediationMaxDepth = 2  // Default: allow 2 levels of self-remediation
} else {
    n, err := strconv.Atoi(depthStr)
    // ... validation
    cfg.SelfRemediationMaxDepth = n
}
```

### Integration Points

- **SourceProviderReconciler**: Passes chain depth from Finding to RemediationJob
- **JobBuilder**: Injects chain depth into agent job environment
- **Config**: Provides `SelfRemediationMaxDepth` to JobProvider

### Testing Requirements

**Unit Tests** (`internal/provider/native/job_test.go`):
- Chain depth increment logic
- Max depth enforcement
- Owner reference vs annotation reading
- Edge cases (missing owner, invalid annotation)

**Integration Tests** (`internal/provider/native/chaindepth_integration_test.go`):
- Cascade simulation with multiple levels
- Concurrent updates to parent RemediationJob
- Race condition prevention
- Max depth enforcement at different limits

---

## Tasks

- [x] Add ChainDepth field to domain.Finding and RemediationJobSpec
- [x] Implement atomic depth reading from owner RemediationJob
- [x] Implement annotation fallback for backward compatibility
- [x] Add max depth enforcement in JobProvider
- [x] Add configuration for max depth (SELF_REMEDIATION_MAX_DEPTH)
- [x] Write unit tests for depth calculation
- [x] Write integration tests for cascade scenarios
- [x] Test concurrent reconciliation scenarios

---

## Dependencies

**Depends on:** STORY_01_self_remediation_detection
**Blocks:** STORY_03_circuit_breaker, STORY_04_upstream_routing

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] No data races in concurrent depth reading
- [x] Backward compatibility maintained with annotation fallback
- [x] Max depth enforcement works correctly at all levels
- [x] Configuration validated and documented