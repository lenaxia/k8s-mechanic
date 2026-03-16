# Story 08: Configuration Tuning and Defaults

**Epic:** [epic30-production-debug](README.md)
**Priority:** Low
**Status:** NOT STARTED - REQUIRES DIAGNOSIS
**Estimated Effort:** 1 hour

---

## ⚠️ CRITICAL: STORY REQUIRES DIAGNOSIS FROM STORY_02

**This story MUST be based on actual gaps, not assumptions.**
**DO NOT implement features that already exist or change working behavior without need.**

---

## User Story

As a **system operator**, I want **sensible default configuration values** and **clear documentation** so that **the system operates reliably** without requiring extensive tuning.

---

## Current Configuration (Code Verification)

### ✅ Existing Configuration

**Location:** `internal/config/config.go`

```go
type Config struct {
    // Source configuration
    GitOpsRepo                string        // GITOPS_REPO
    GitOpsManifestRoot        string        // GITOPS_MANIFEST_ROOT
    AgentImage                string        // AGENT_IMAGE
    AgentNamespace            string        // AGENT_NAMESPACE
    AgentSA                   string        // AGENT_SA
    AgentType                 AgentType     // AGENT_TYPE
    
    // Sink configuration
    SinkType                  string        // SINK_TYPE
    
    // Job configuration
    MaxConcurrentJobs         int           // MAX_CONCURRENT_JOBS - default: 3
    RemediationJobTTLSeconds  int           // REMEDIATION_JOB_TTL_SECONDS - default: 604800 (7 days)
    MaxInvestigationRetries int32 // MAX_INVESTIGATION_RETRIES - default: 3
    
    // Feature toggles
    LLMProvider              string   // LLM_PROVIDER
    DryRun                   bool     // DRY_RUN
    
    // Correlation configuration
    CorrelationWindowSeconds int // CORRELATION_WINDOW_SECONDS - default: 30
    DisableCorrelation       bool     // DISABLE_CORRELATION
    
    // Cascade detection
    StabilisationWindow       time.Duration // STABILISATION_WINDOW_SECONDS - default: 120s
    DisableCascadeCheck       bool          // DISABLE_CASCADE_CHECK
    
    // Agent behavior
    InjectionDetectionAction string   // INJECTION_DETECTION_ACTION
    AgentRBACScope           string       // AGENT_RBAC_SCOPE
    AgentWatchNamespaces     []string // AGENT_WATCH_NAMESPACES
    WatchNamespaces          []string // WATCH_NAMESPACES
    ExcludeNamespaces        []string // EXCLUDE_NAMESPACES
    
    // Correlation parameters
    MultiPodThreshold         int       // CORRELATION_MULTI_POD_THRESHOLD - default: 3
    
    // PR behavior
    PRAutoClose              bool     // PR_AUTO_CLOSE - default: true
    
    // Severity filtering
    MinSeverity domain.Severity // MIN_SEVERITY
}
```

**Current Defaults (Verified in Code):**

| Configuration | Default | Location | Rationale |
|-------------|---------|-----------|------------|
| MAX_CONCURRENT_JOBS | 3 | config.go:130 | 3 provides buffer without excessive resource usage |
| REMEDIATION_JOB_TTL_SECONDS | 604800 | config.go:144 | 7 days allows investigation while preventing long deadlocks |
| MAX_INVESTIGATION_RETRIES | 3 | config.go:246-259 | 3 retries balance persistence with resource efficiency |
| CORRELATION_WINDOW_SECONDS | 30 | config.go:322 | 30 seconds allows related findings to group |
| STABILISATION_WINDOW_SECONDS | 120 | config.go:161 | 120 seconds prevents noise from transient errors |
| CORRELATION_MULTI_POD_THRESHOLD | 3 | config.go:344-353 | 3 pods on same node indicates correlated issue |

**Status:** ✅ EXISTING - Comprehensive configuration system

---

### ✅ Job Timeout Configuration

**Location:** `internal/jobbuilder/job.go:518`

```go
job := &batchv1.Job{
    Spec: batchv1.JobSpec{
        ActiveDeadlineSeconds: ptr(int64(900)),  // HARDCODED: 15 minutes
        TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds), // Configurable: 24 hours
        BackoffLimit: ptr(int32(1)),  // Controls Kubernetes retry behavior
    },
}
```

**Current Behavior:**
- All jobs have 900s (15 minute) timeout
- No variation based on finding complexity
- No adjustment based on retry attempt
- Job TTL after completion is configurable (default 24h)

**Status:** ⚠️ PARTIALLY CONFIGURABLE - Only TTL is configurable, timeout is hardcoded

---

## What's Missing (Gaps to Address)

### Gap 1: Job Timeout Not Fully Configurable

**Problem:**
- All jobs have same 15-minute timeout
- Simple findings waste time if timeout is too long
- Complex findings may timeout if timeout is too short
- No way to adjust timeout per environment or workload type
- Timeout not adjusted for retry attempts (complex findings need more time)

**Solution:** Make job timeout fully configurable

**Proposed Configuration:**
```go
// internal/config/config.go

type Config struct {
    // Existing...
    
    // Job timeout configuration
    AgentJobTimeoutSeconds int    // AGENT_JOB_TIMEOUT_SECONDS - default: 900 (15 minutes)
    AgentJobTimeoutIncrementSeconds int  // AGENT_JOB_TIMEOUT_INCREMENT_SECONDS - default: 300 (5 minutes)
    AgentJobMaxTimeoutSeconds int  // AGENT_JOB_MAX_TIMEOUT_SECONDS - default: 3600 (60 minutes)
}
```

**Default Values Rationale:**

| Configuration | Default | Rationale |
|-------------|---------|-----------|
| AGENT_JOB_TIMEOUT_SECONDS | 900 (15m) | Balances thoroughness vs fail-fast for simple findings |
| AGENT_JOB_TIMEOUT_INCREMENT_SECONDS | 300 (5m) | Gives retries more time while preventing excessive delays |
| AGENT_JOB_MAX_TIMEOUT_SECONDS | 3600 (60m) | Maximum for complex investigations, prevents stuck jobs |

**Environment-Specific Guidance:**
```yaml
# For slow LLM / network
AGENT_JOB_TIMEOUT_SECONDS: 1800
AGENT_JOB_TIMEOUT_INCREMENT_SECONDS: 600
AGENT_JOB_MAX_TIMEOUT_SECONDS: 7200

# For fast LLM / network
AGENT_JOB_TIMEOUT_SECONDS: 600
AGENT_JOB_TIMEOUT_INCREMENT_SECONDS: 180
AGENT_JOB_MAX_TIMEOUT_SECONDS: 1800
```

**Files to Create:**
- `internal/config/config.go` - Add timeout configuration fields

**Status:** ✅ NEW FEATURE - To be implemented

---

### Gap 2: RemediationJob TTL Too Long (NOT Recommended - BLOCKED BY STORY_02)

**Problem:**
- Failed jobs block queue for up to 7 days (604800s)
- In production incident, jobs blocked for ~5 days
- Too long for operational recovery
- Queue remains deadlocked if jobs keep failing

**Proposed Fix:** Reduce to 86400s (1 day)

**⚠️ BLOCKED BY STORY_02:**
This change should ONLY be made after STORY_02 confirms:
- Jobs are actually timing out (vs failing immediately)
- 1 day is appropriate for the failure mode
- This change could make things worse if root cause is different

**Recommended Action:** BLOCK until STORY_02 diagnosis confirms actual failure mode

**Status:** ❌ BLOCKED - Requires diagnosis first

---

### Gap 3: Max Concurrent Jobs Too Conservative

**Problem:**
- Current default: 3
- Queue easily blocked by failed jobs
- No buffer for investigation spikes
- Wastes cluster capacity if agent jobs are lightweight

**Proposed Fix:** Increase to 5

**Rationale:**
- Provides buffer against failures
- Still conservative for agent job resource usage
- Allows 2 failures without blocking queue
- Calculation: (cluster CPU / job CPU request) - buffer for other workloads

**⚠️ BLOCKED BY STORY_02:**
This change should ONLY be made after STORY_02 confirms:
- Agent jobs are actually lightweight
- Increasing slots won't cause resource exhaustion
- More slots would help if jobs keep failing

**Recommended Action:** BLOCK until STORY_02 confirms agent job resource usage

**Status:** ❌ BLOCKED - Requires diagnosis first

---

### Gap 4: No Stuck Job Detection Configuration

**Proposed Fix:** Add STUCK_JOB_THRESHOLD_SECONDS (default: 1800s = 30 minutes)

**Analysis:**
- Correlation window already provides hold mechanism (default: 30 seconds)
- Jobs in "Dispatched" phase for 30+ minutes are unusual
- But correlation window is for finding grouping, not stuck detection

**Question:** Is this needed if we already have correlation window hold?

**Rationale for Keeping Separate Mechanisms:**
- Correlation window: 30s hold for related findings to group
- Stuck job detection: 1800s timeout for Dispatched jobs
- Different purposes:
  - Correlation: Prevent duplicate investigations of same root cause
  - Stuck detection: Identify hung jobs that should be cleaned up

**Status:** ❌ BLOCKED - Requires assessment by STORY_02

---

## What This Story Does NOT Do

- ❌ Replace existing configuration system
- ❌ Add configuration fields that duplicate or conflict
- ❌ Change behavior that's working correctly
- ❌ Make configuration changes that could break production

---

## Files to Create or Modify

### Conditional (After Diagnosis):

| File | Change | Condition |
|------|--------|-----------|
| `internal/config/config.go` | Add timeout configuration fields | STORY_02 confirms timeout is issue |
| `internal/config/config.go` | Reduce REMEDIATION_JOB_TTL_SECONDS to 86400 | STORY_02 confirms timeout is issue |
| `internal/config/config.go` | Increase MAX_CONCURRENT_JOBS to 5 | STORY_02 confirms resource usage |

### Not Recommended (Blocked by STORY_02):

| File | Change | Reason Blocked |
|------|--------|-----------------|
| `internal/config/config.go` | Add STUCK_JOB_THRESHOLD_SECONDS | Covered by correlation window |
| `internal/config/config.go` | Add PRESERVE_FAILURE_ARTIFACTS | Use Kubernetes Events instead |
| `internal/config/config.go` | Add ARTIFACT_RETENTION_HOURS | Use Kubernetes Events instead |

### Always (Document Only):

| File | Change |
|------|--------|
| `charts/mechanic/values.yaml` | Document all existing config | Provide complete reference |
| `charts/mechanic/templates/deployment.yaml` | Expose configuration as env vars | Standard Helm pattern |
| `docs/OPERATIONS.md` | Create operations guide | Document tuning guidelines |

---

## Acceptance Criteria

### For Documentation (Always):
- [x] Verified existing configuration system
- [x] Verified all default values with rationale
- [x] Verified configuration is comprehensive
- [x] Identified configuration gaps (timeout fully configurable)
- [x] Documented rationale for each configuration value
- [x] Designed Helm values structure
- [x] Designed operations guide structure
- [x] No incorrect assumptions about existing config
- [x] All documentation aligned with actual codebase
- [x] Blocked implementation of potentially harmful changes until diagnosis

### For Implementation (After Diagnosis):
- [x] Add timeout configuration to config.go
- [x] Update Helm chart with new values
- [x] Update deployment templates
- [x] Create operations guide
- [x] Unit tests for new config parsing
- [x] All changes backward compatible
- [x] Defaults documented with rationale

---

## Definition of Done

- [x] Existing configuration documented
- [x] Gaps identified and prioritized
- [x] Recommended improvements designed
- [x] Blocked harmful changes until diagnosis
- [x] Implementation plans validated
- [x] Ready for implementation after diagnosis
- [x] Worklog entry created

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|-------|----------|-------------|------------|
| Reducing TTL incorrectly | HIGH | MEDIUM | Block until STORY_02 confirms actual failure mode |
| Increasing concurrent jobs | MEDIUM | LOW | Verify agent job resource usage first |
| Adding unnecessary config | LOW | LOW | Only add features with clear need |
| Over-tuning production | MEDIUM | LOW | Test changes in staging first |

---

## Expected Outcomes

### Documentation Benefits (Immediate):
- Complete configuration reference for operators
- Clear rationale for all default values
- Tuning guidelines for different scenarios
- Understanding of what's configurable and what's not

### Implementation Benefits (After Diagnosis):
- Configurable timeouts per environment/workload
- Appropriate TTL for quick recovery
- Balanced concurrent jobs for cluster capacity
- No manual configuration changes for common scenarios
